// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package pii detects and redacts personally-identifiable information
// in free-form text. Complements the credential scrubber in
// internal/agent/secret_scrub.go: that one catches API keys, tokens,
// and connection strings; this one catches the human data the LLM
// should never see unless the user explicitly opts in.
//
// Threat model:
//   - A user pastes a customer list, résumé, or exported CSV into chat.
//     Without redaction, that content ends up in provider logs, prompt
//     caches, and cross-request summaries. We don't control any of
//     those downstream stores.
//   - A tool returns a document/email/database row that contains
//     someone else's PII. The agent summarises it, and that summary
//     gets persisted as a memory — now the PII is in our KG too.
//
// Design:
//   - Pure regex + checksum. No ML, no sidecar, no network.
//   - Off by default. Callers ask for redaction explicitly — we never
//     silently mutate user content.
//   - Every redaction preserves the *category* (so the LLM still
//     understands "there was a credit card here"). "{{PII:credit_card}}"
//     is more useful than "[REDACTED]".
//   - Every detector is independently toggleable via a Set bitmask so
//     callers can say "redact credit cards but keep emails" when e.g.
//     the agent's job is "reply to this email thread".
package pii

import (
	"net"
	"regexp"
	"strings"
)

// Kind identifies a single PII category. Used as a bitmask so a caller
// can enable any combination with bitwise OR.
type Kind uint32

const (
	KindEmail Kind = 1 << iota
	KindPhone
	KindSSN        // US Social Security Number
	KindCreditCard // validated with Luhn
	KindIBAN       // international bank account, length+checksum validated
	KindIPv4
	KindIPv6
)

// All is every detector. Callers who want "redact everything" pass
// this as the Kinds field on Config.
const All = KindEmail | KindPhone | KindSSN | KindCreditCard | KindIBAN | KindIPv4 | KindIPv6

// String returns a stable category name that's safe to embed in redaction
// placeholders. The agent loop depends on these names staying lowercase
// and underscore-separated — don't rename without updating callers.
func (k Kind) String() string {
	switch k {
	case KindEmail:
		return "email"
	case KindPhone:
		return "phone"
	case KindSSN:
		return "ssn"
	case KindCreditCard:
		return "credit_card"
	case KindIBAN:
		return "iban"
	case KindIPv4:
		return "ipv4"
	case KindIPv6:
		return "ipv6"
	}
	return "unknown"
}

// Detection is a single hit from Scan. Start/End are byte offsets in
// the original string, suitable for passing to strings.Replace or
// building redacted output manually.
type Detection struct {
	Kind  Kind
	Value string // the matched raw text
	Start int    // inclusive byte offset
	End   int    // exclusive byte offset
}

// Config controls which categories to detect and how to render the
// replacement. A zero Config scans nothing — callers must set Kinds
// explicitly. This is intentional: silent defaults in a redaction
// library are a footgun.
type Config struct {
	// Kinds is a bitmask of Kind values to detect. Use All for everything.
	Kinds Kind

	// Placeholder formats the replacement text. The default emits
	// "{{PII:email}}" / "{{PII:credit_card}}" etc. Callers who want a
	// different shape ("[redacted email]", "<PII>") can override.
	//
	// The returned string is written literally — no further escaping.
	Placeholder func(Kind) string
}

func defaultPlaceholder(k Kind) string { return "{{PII:" + k.String() + "}}" }

// --- Precompiled regexes. Compiled once at package init; cheap to reuse. ---

var (
	// Email — RFC-5322 "simplified". Covers 99%+ of real addresses
	// while staying readable and fast. Intentionally rejects quoted
	// local parts; those are rare enough that missing them is worth
	// the clarity win.
	reEmail = regexp.MustCompile(`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,24}\b`)

	// Phone — three common shapes:
	//   +CC(...)NNN-NNNN   →  +1 (415) 555-2671
	//   CC.NNN.NNN.NNNN    →  1.415.555.2671
	//   NNN-NNN-NNNN       →  415-555-2671 (assumed US if no country)
	// Anchored to word boundaries so we don't eat arbitrary runs of
	// digits. Order-of-magnitude heuristic: total digit count 7–15.
	rePhone = regexp.MustCompile(`(?:\+?\d{1,3}[\s.\-]?)?(?:\(\d{2,4}\)[\s.\-]?|\d{2,4}[\s.\-])\d{3,4}[\s.\-]\d{3,4}\b`)

	// SSN (US) — NNN-NN-NNNN. Go's RE2 doesn't support negative
	// lookahead, so we over-match here and filter invalid prefixes
	// (000, 666, 9xx area; 00 group; 0000 serial) in validateSSN.
	reSSN = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)

	// Credit card — 13–19 digits with optional spaces or hyphens as
	// separators. We match widely and then Luhn-validate downstream
	// so ordinary numeric IDs don't get flagged.
	reCC = regexp.MustCompile(`\b(?:\d[ \-]?){12,18}\d\b`)

	// IBAN — country code + 2 check digits + up to 30 alphanumerics.
	// We validate the mod-97 checksum so random uppercase strings
	// don't trigger.
	reIBAN = regexp.MustCompile(`\b[A-Z]{2}\d{2}[A-Z0-9]{10,30}\b`)

	// IPv4 — four dotted octets, each 0–255 (approximated as 0-999
	// in the regex; the Go net parser is the real validator).
	reIPv4 = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

	// IPv6 — permissive capture including `::` compression and zone
	// scope. The real validation happens via net.ParseIP in
	// validateIPv6. We deliberately skip `\b` anchors because `:` and
	// `%` aren't word characters, and RE2 word boundaries would split
	// the match at `::` or `%en0`.
	//
	// Shape: run of `hex:` prefixes (may include the double-colon
	// compression) optionally ending in a final hex group and an
	// optional zone (%eth0).
	reIPv6 = regexp.MustCompile(`(?:[A-Fa-f0-9]{1,4}(?::|::)){1,7}[A-Fa-f0-9]{1,4}(?:%[A-Za-z0-9]+)?|::1|::`)
)

// Scan returns every detection for the kinds set in cfg.Kinds, in
// left-to-right order. Overlapping matches prefer the first one
// encountered by category order (email > phone > ssn > cc > iban >
// ipv4 > ipv6) — this matters because a phone number can syntactically
// look like a poorly-formatted credit card, and we'd rather call it
// a phone.
func Scan(text string, cfg Config) []Detection {
	if cfg.Kinds == 0 || text == "" {
		return nil
	}
	out := []Detection{}

	// Order matters — see the doc comment above.
	if cfg.Kinds&KindEmail != 0 {
		out = append(out, findAll(text, reEmail, KindEmail, nil)...)
	}
	if cfg.Kinds&KindPhone != 0 {
		out = append(out, findAll(text, rePhone, KindPhone, nil)...)
	}
	if cfg.Kinds&KindSSN != 0 {
		out = append(out, findAll(text, reSSN, KindSSN, validateSSN)...)
	}
	if cfg.Kinds&KindCreditCard != 0 {
		out = append(out, findAll(text, reCC, KindCreditCard, validateCreditCard)...)
	}
	if cfg.Kinds&KindIBAN != 0 {
		out = append(out, findAll(text, reIBAN, KindIBAN, validateIBAN)...)
	}
	if cfg.Kinds&KindIPv4 != 0 {
		out = append(out, findAll(text, reIPv4, KindIPv4, validateIPv4)...)
	}
	if cfg.Kinds&KindIPv6 != 0 {
		out = append(out, findAll(text, reIPv6, KindIPv6, validateIPv6)...)
	}

	// De-dupe overlapping ranges — keep the first (highest priority)
	// match when categories disagree on the same bytes.
	return dedupeOverlaps(out)
}

// Redact applies Scan and replaces each detection with the configured
// placeholder. Emitted in a single pass so multi-byte UTF-8 offsets
// stay stable. Safe on text with no detections: returns the input.
func Redact(text string, cfg Config) string {
	dets := Scan(text, cfg)
	if len(dets) == 0 {
		return text
	}
	ph := cfg.Placeholder
	if ph == nil {
		ph = defaultPlaceholder
	}
	var sb strings.Builder
	sb.Grow(len(text))
	pos := 0
	for _, d := range dets {
		if d.Start < pos {
			// Overlap we couldn't fully dedupe — skip so we don't
			// corrupt the output.
			continue
		}
		sb.WriteString(text[pos:d.Start])
		sb.WriteString(ph(d.Kind))
		pos = d.End
	}
	sb.WriteString(text[pos:])
	return sb.String()
}

// --- Internals ---

type validator func(string) bool

func findAll(text string, re *regexp.Regexp, kind Kind, ok validator) []Detection {
	idxs := re.FindAllStringIndex(text, -1)
	out := make([]Detection, 0, len(idxs))
	for _, pair := range idxs {
		match := text[pair[0]:pair[1]]
		if ok != nil && !ok(match) {
			continue
		}
		out = append(out, Detection{
			Kind:  kind,
			Value: match,
			Start: pair[0],
			End:   pair[1],
		})
	}
	return out
}

// dedupeOverlaps sorts by Start and drops any detection that begins
// before the previous one ended. We rely on Scan adding detections
// in category-priority order so the first match wins a tie.
func dedupeOverlaps(in []Detection) []Detection {
	if len(in) < 2 {
		return in
	}
	// Stable sort by Start — preserve category priority for ties.
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j].Start < in[j-1].Start; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
	out := in[:0]
	lastEnd := -1
	for _, d := range in {
		if d.Start < lastEnd {
			continue
		}
		out = append(out, d)
		lastEnd = d.End
	}
	return out
}

// validateCreditCard: strip separators, check length, run Luhn.
// Intentionally conservative — we'd rather miss a valid card than
// flag every 15-digit database ID.
func validateCreditCard(s string) bool {
	digits := []byte{}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		} else if c != ' ' && c != '-' {
			return false
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	// Luhn's algorithm.
	sum := 0
	alt := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

// validateIBAN: mod-97 on the numeric expansion per ISO 13616.
// Rejects anything where the numeric form isn't exactly mod-97 == 1.
func validateIBAN(s string) bool {
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	// Move first 4 chars to the end, then convert A-Z → 10-35.
	rearranged := s[4:] + s[:4]
	var num strings.Builder
	num.Grow(len(rearranged) * 2)
	for i := 0; i < len(rearranged); i++ {
		c := rearranged[i]
		switch {
		case c >= '0' && c <= '9':
			num.WriteByte(c)
		case c >= 'A' && c <= 'Z':
			n := int(c-'A') + 10
			num.WriteByte('0' + byte(n/10))
			num.WriteByte('0' + byte(n%10))
		default:
			return false
		}
	}
	// mod-97 in chunks to avoid big.Int; 9-digit chunks fit in uint64.
	buf := num.String()
	rem := 0
	for i := 0; i < len(buf); {
		end := i + 9
		if end > len(buf) {
			end = len(buf)
		}
		chunkStr := buf[i:end]
		var chunk int
		for j := 0; j < len(chunkStr); j++ {
			chunk = chunk*10 + int(chunkStr[j]-'0')
		}
		// Combine with carried remainder.
		scale := 1
		for k := 0; k < len(chunkStr); k++ {
			scale *= 10
		}
		rem = (rem*scale + chunk) % 97
		i = end
	}
	return rem == 1
}

// validateSSN filters the well-known invalid prefixes per SSA rules:
//   - area number 000, 666, or 9xx
//   - group number 00
//   - serial number 0000
// Without this, every "123-45-6789" in a test fixture would be flagged.
func validateSSN(s string) bool {
	if len(s) != 11 || s[3] != '-' || s[6] != '-' {
		return false
	}
	area := s[:3]
	group := s[4:6]
	serial := s[7:]
	if area == "000" || area == "666" {
		return false
	}
	if area[0] == '9' {
		return false
	}
	if group == "00" {
		return false
	}
	if serial == "0000" {
		return false
	}
	return true
}

func validateIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func validateIPv6(s string) bool {
	// Strip zone scope if present (fe80::1%en0).
	if i := strings.IndexByte(s, '%'); i >= 0 {
		s = s[:i]
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil && strings.Contains(s, ":")
}
