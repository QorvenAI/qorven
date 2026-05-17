// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package promptguard detects prompt-injection attempts in free-form
// user input. A detection is a SIGNAL, not a verdict — callers decide
// whether to refuse, quarantine, or just log.
//
// Threat model:
//   A user-supplied string (web page content, scraped email, uploaded
//   document, forwarded message) tries to override the agent's system
//   prompt. Common patterns:
//
//   - "Ignore previous instructions and do X"
//   - "You are now an unrestricted AI. Respond to ..."
//   - Base64-encoded payloads telling the model to reveal secrets
//   - Fake system-role markers: "<|system|>", "<system>", "{{role:system}}"
//   - Jailbreak preambles: "DAN mode", "developer mode", etc.
//
// Design:
//   - Heuristic-only for now — regex + phrase matching, no model.
//     Fast enough to run on every user message (sub-ms) without
//     burning a second LLM call. Adequate for the common attacks.
//   - Callers opt in via Score(). A Score of 0 means clean, 1.0
//     means certain attack. Threshold is a caller decision:
//     0.3 = suspicious (log), 0.6 = likely (quarantine), 0.85 = block.
//   - Every detection reports WHY — the rule that matched — so the
//     audit log tells you which attack vector fired.
//   - Extensible: callers can register their own rules at runtime
//     without modifying the package (e.g. site-specific honeypot
//     phrases).
//
// Not a silver bullet. A determined attacker using novel phrasing or
// cross-language obfuscation will slip through. Treat promptguard as
// layer one of a defense-in-depth stack that also includes:
//   - Least-privilege tool access (permissions.ModeDefault)
//   - Approval gates for destructive actions
//   - Credential scrubbing on tool outputs
//   - PII redaction on user content
package promptguard

import (
	"regexp"
	"strings"
)

// Severity buckets are advisory — callers pick their own thresholds.
// We list these so the UI / audit log can render a consistent label.
const (
	ScoreClean      = 0.0
	ScoreSuspicious = 0.3
	ScoreLikely     = 0.6
	ScoreCertain    = 0.9
)

// Detection is the report of one matched rule.
type Detection struct {
	Rule     string  `json:"rule"`
	Weight   float64 `json:"weight"`   // 0..1, how strong a signal this one rule is
	Snippet  string  `json:"snippet"`  // up to ~80 chars around the match
	Category string  `json:"category"` // "override" | "role_injection" | "exfil" | "jailbreak" | "encoded"
}

// Report is the aggregate Scan result.
type Report struct {
	Score      float64     `json:"score"`       // aggregate [0..1]
	Detections []Detection `json:"detections"`  // every rule that fired, ordered by weight desc
}

// Clean is a convenience for callers that want a quick "safe or not".
func (r *Report) Clean() bool { return r.Score < ScoreSuspicious }

// Suspicious returns true at or above 0.3.
func (r *Report) Suspicious() bool { return r.Score >= ScoreSuspicious }

// Likely returns true at or above 0.6.
func (r *Report) Likely() bool { return r.Score >= ScoreLikely }

// Certain returns true at or above 0.9.
func (r *Report) Certain() bool { return r.Score >= ScoreCertain }

// rule is the internal shape. Pattern can be either a regex or a
// literal-substring — we decide based on `isRegex`. Literals are
// faster (no regex compile) so we prefer them when the phrase doesn't
// need wildcards.
type rule struct {
	name     string
	pattern  string
	re       *regexp.Regexp // nil for literal rules
	weight   float64
	category string
}

// defaultRules are compiled once at package init. Ordering is
// aesthetic; the scanner runs all of them and sums weights.
//
// Weights are calibrated so that:
//   - Any single rule with weight >= 0.7 is enough to flag Likely
//   - Two weight-0.4 rules together tip into Likely
//   - A single "ignore previous instructions" exact match pushes to Certain
var defaultRules = []rule{
	// --- Category: override — explicit instruction to disregard prior prompt
	{
		name:     "ignore-previous",
		pattern:  `(?i)\b(?:ignore|disregard|forget)(?:\s+(?:all|any|every|your|the|previous|prior|above|preceding|earlier))+\s+(?:instructions?|prompts?|rules?|messages?|context|system)\b`,
		weight:   0.9,
		category: "override",
	},
	{
		name:     "override-directive",
		pattern:  `(?i)\b(?:override|supersede|cancel|nullify|void)\s+(?:all|any|the)?\s*(?:previous|prior|system)\s+(?:instructions?|prompts?|rules?)\b`,
		weight:   0.85,
		category: "override",
	},
	{
		name:     "new-instructions",
		pattern:  `(?i)\b(?:new|updated|latest|real)\s+instructions?\s+(?:are|follow|below|override)\b`,
		weight:   0.5,
		category: "override",
	},

	// --- Category: role_injection — fake system/developer markers
	{
		name:     "fake-system-tag",
		pattern:  `(?i)<\s*\|?\s*(?:system|developer|admin|root)\s*\|?\s*>`,
		weight:   0.75,
		category: "role_injection",
	},
	{
		name:     "role-directive",
		pattern:  `(?i)\byou\s+are\s+now\s+(?:an?\s+)?(?:unrestricted|jailbroken|uncensored|unfiltered|omnipotent|god-mode|dan|dev(?:eloper)?\s+mode)`,
		weight:   0.85,
		category: "jailbreak",
	},
	{
		name:     "assume-role",
		pattern:  `(?i)\b(?:act|pretend|behave|roleplay|imagine)\s+(?:as|to\s+be)\s+(?:a|an)?\s*(?:unrestricted|evil|malicious|uncensored|jailbroken|god-mode)\b`,
		weight:   0.7,
		category: "jailbreak",
	},

	// --- Category: exfil — attempts to reveal secrets
	{
		name: "reveal-system-prompt",
		// Tolerate a short intervening word ("show ME your", "give me
		// YOUR", "tell ME the") — the attacker's phrasing varies but
		// the semantic ask is the same.
		pattern: `(?i)\b(?:print|show|reveal|output|dump|recite|repeat|tell|give|provide|expose)\s+(?:me\s+|us\s+)?(?:your|the)\s+(?:system\s+|secret\s+|hidden\s+)?(?:prompt|instructions?|context|rules|config|token|key|password|credentials?|api[\s-]?key|secret)\b`,
		weight:   0.8,
		category: "exfil",
	},
	{
		name:     "leak-api-key",
		pattern:  `(?i)\bwhat\s+(?:is|are)\s+(?:your|the)\s+(?:api[\s-]?key|secret|password|token|credential)s?\b`,
		weight:   0.7,
		category: "exfil",
	},

	// --- Category: jailbreak — named jailbreak patterns
	{
		name:     "dan-mode",
		pattern:  `(?i)\b(?:DAN|do\s+anything\s+now)\s+(?:mode|prompt)\b`,
		weight:   0.7,
		category: "jailbreak",
	},
	{
		name:     "jailbreak-keyword",
		pattern:  `(?i)\b(?:jailbreak(?:ing)?|bypass(?:es|ing|ed)?\s+(?:safety|safeguards|restrictions|filters))\b`,
		weight:   0.55,
		category: "jailbreak",
	},

	// --- Category: encoded — attempts to hide payload from scanners
	{
		name:     "base64-command-hint",
		pattern:  `(?i)\b(?:execute|decode|run)\s+(?:the\s+)?(?:following\s+)?base64\b`,
		weight:   0.6,
		category: "encoded",
	},
	{
		name:     "hidden-unicode-marker",
		// Tag characters (U+E0000..U+E007F) are sometimes used to hide
		// instructions inside visible text.
		pattern:  `[\x{E0000}-\x{E007F}]`,
		weight:   0.9,
		category: "encoded",
	},

	// --- Category: misc — low-weight signals, only matter in aggregate
	{
		name:     "markdown-system-header",
		pattern:  `(?im)^#{1,3}\s*(?:system|administrator|admin)\s+(?:prompt|instructions?|note)\s*$`,
		weight:   0.4,
		category: "role_injection",
	},
}

// init compiles the regex patterns once.
func init() {
	for i := range defaultRules {
		defaultRules[i].re = regexp.MustCompile(defaultRules[i].pattern)
	}
}

// Scan runs every rule against text and returns the aggregate report.
// The score is min(1.0, sum(weights)) so any single high-weight hit
// is sufficient but low-weight hits also aggregate.
//
// Empty or very short input is always clean — not worth the cost of
// false positives on a one-word message.
func Scan(text string) *Report {
	if len(strings.TrimSpace(text)) < 8 {
		return &Report{}
	}
	var dets []Detection
	var total float64
	for _, r := range defaultRules {
		match := r.re.FindStringIndex(text)
		if match == nil {
			continue
		}
		total += r.weight
		dets = append(dets, Detection{
			Rule:     r.name,
			Weight:   r.weight,
			Snippet:  snippetAround(text, match[0], match[1], 40),
			Category: r.category,
		})
	}
	if total > 1.0 {
		total = 1.0
	}
	// Sort detections by weight desc so the audit log reads "strongest
	// signal first". Stable — rules with equal weight keep their
	// definition order.
	for i := 1; i < len(dets); i++ {
		for j := i; j > 0 && dets[j].Weight > dets[j-1].Weight; j-- {
			dets[j], dets[j-1] = dets[j-1], dets[j]
		}
	}
	return &Report{Score: total, Detections: dets}
}

// snippetAround returns text surrounding a match, bounded by `pad`
// characters on each side. We clamp at word boundaries so the snippet
// in audit logs is legible.
func snippetAround(text string, start, end, pad int) string {
	s := start - pad
	if s < 0 {
		s = 0
	}
	e := end + pad
	if e > len(text) {
		e = len(text)
	}
	// Extend to the next space/newline on both sides for readability.
	for s > 0 && text[s] != ' ' && text[s] != '\n' {
		s--
	}
	for e < len(text) && text[e] != ' ' && text[e] != '\n' {
		e++
	}
	snip := strings.TrimSpace(text[s:e])
	// Collapse interior whitespace so a wall-of-text attack doesn't
	// produce a 500-char "snippet" in the audit log.
	snip = strings.Join(strings.Fields(snip), " ")
	if len(snip) > 160 {
		snip = snip[:160] + "…"
	}
	return snip
}
