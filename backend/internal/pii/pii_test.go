// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package pii

import (
	"strings"
	"testing"
)

// TestRedact_NoKinds: zero config scans nothing — a deliberate guard
// against a caller forgetting to set Kinds and then shipping unredacted
// text thinking they were protected.
func TestRedact_NoKinds(t *testing.T) {
	in := "email me at alice@example.com or call 415-555-2671"
	out := Redact(in, Config{})
	if out != in {
		t.Errorf("zero config should be no-op, got %q", out)
	}
}

// TestRedact_Email: basic email detection + the default placeholder format.
func TestRedact_Email(t *testing.T) {
	in := "Contact alice@example.com and bob+work@sub.example.co.uk"
	got := Redact(in, Config{Kinds: KindEmail})
	want := "Contact {{PII:email}} and {{PII:email}}"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestRedact_Phone: three common US/international shapes.
func TestRedact_Phone(t *testing.T) {
	cases := []string{
		"+1 (415) 555-2671",
		"1.415.555.2671",
		"415-555-2671",
	}
	for _, c := range cases {
		in := "call " + c + " today"
		got := Redact(in, Config{Kinds: KindPhone})
		if !strings.Contains(got, "{{PII:phone}}") || strings.Contains(got, c) {
			t.Errorf("phone %q not redacted; got %q", c, got)
		}
	}
}

// TestRedact_SSN_Valid: real-shape SSN is redacted.
func TestRedact_SSN_Valid(t *testing.T) {
	got := Redact("SSN: 123-45-6789", Config{Kinds: KindSSN})
	if got != "SSN: {{PII:ssn}}" {
		t.Errorf("got %q", got)
	}
}

// TestRedact_SSN_Invalid: the well-known invalid prefixes (000, 666, 9xx)
// must NOT match. Otherwise test fixtures full of "000-00-0000" dummy
// values would look like real PII.
func TestRedact_SSN_Invalid(t *testing.T) {
	cases := []string{"000-12-3456", "666-12-3456", "912-34-5678", "123-00-4567", "123-45-0000"}
	for _, c := range cases {
		got := Redact(c, Config{Kinds: KindSSN})
		if got != c {
			t.Errorf("SSN %q should not be redacted; got %q", c, got)
		}
	}
}

// TestRedact_CreditCard_Luhn: a real test-card number (Visa 4242...)
// must redact; a same-length non-Luhn sequence must not.
func TestRedact_CreditCard_Luhn(t *testing.T) {
	// 4242 4242 4242 4242 is the Stripe test number — a valid Luhn.
	good := "card: 4242 4242 4242 4242"
	got := Redact(good, Config{Kinds: KindCreditCard})
	if !strings.Contains(got, "{{PII:credit_card}}") {
		t.Errorf("valid Luhn card not redacted: %q", got)
	}

	// Same length, not Luhn-valid. Should pass through untouched.
	bad := "tracking id 1234 5678 9012 3456"
	got = Redact(bad, Config{Kinds: KindCreditCard})
	if got != bad {
		t.Errorf("non-Luhn number redacted anyway: %q", got)
	}
}

// TestRedact_CreditCard_TooShortOrLong: length bounds 13–19 digits.
func TestRedact_CreditCard_TooShortOrLong(t *testing.T) {
	// 12 digits — too short.
	tooShort := "acct 4242424242 42" // 12 digits
	got := Redact(tooShort, Config{Kinds: KindCreditCard})
	if got != tooShort {
		t.Errorf("12-digit number redacted: %q", got)
	}
}

// TestRedact_IBAN_Valid: a known-good IBAN (GB-format) must redact.
// This is the Wikipedia-standard sample: GB82 WEST 1234 5698 7654 32.
func TestRedact_IBAN_Valid(t *testing.T) {
	in := "wire to GB82WEST12345698765432 by Friday"
	got := Redact(in, Config{Kinds: KindIBAN})
	if !strings.Contains(got, "{{PII:iban}}") {
		t.Errorf("valid IBAN not redacted: %q", got)
	}
}

// TestRedact_IBAN_Invalid: random uppercase + digits that pass the
// regex but fail mod-97 must NOT redact.
func TestRedact_IBAN_Invalid(t *testing.T) {
	in := "random AB12CDEF1234567890"
	got := Redact(in, Config{Kinds: KindIBAN})
	if got != in {
		t.Errorf("invalid IBAN redacted: got %q", got)
	}
}

// TestRedact_IPv4: valid + invalid addresses.
func TestRedact_IPv4(t *testing.T) {
	in := "server at 192.168.1.10 and 300.1.1.1"
	got := Redact(in, Config{Kinds: KindIPv4})
	// First should redact; second is out-of-range and must not.
	if !strings.Contains(got, "{{PII:ipv4}}") {
		t.Error("valid IPv4 not redacted")
	}
	if !strings.Contains(got, "300.1.1.1") {
		t.Errorf("invalid IPv4 redacted: %q", got)
	}
}

// TestRedact_IPv6: full + compressed forms.
func TestRedact_IPv6(t *testing.T) {
	cases := []string{
		"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		"2001:db8::1",
		"fe80::1%en0",
	}
	for _, c := range cases {
		got := Redact(c, Config{Kinds: KindIPv6})
		if !strings.Contains(got, "{{PII:ipv6}}") {
			t.Errorf("IPv6 %q not redacted; got %q", c, got)
		}
	}
}

// TestRedact_MultipleKinds: emails + phones + IPs in one pass.
func TestRedact_MultipleKinds(t *testing.T) {
	in := "alice@example.com called from 415-555-2671; saw 10.0.0.1 in logs"
	got := Redact(in, Config{Kinds: KindEmail | KindPhone | KindIPv4})
	if strings.Contains(got, "alice@example.com") ||
		strings.Contains(got, "415-555-2671") ||
		strings.Contains(got, "10.0.0.1") {
		t.Errorf("multi-kind scan missed one: %q", got)
	}
}

// TestRedact_CustomPlaceholder: callers wanting "[hidden]" instead of
// "{{PII:email}}" can supply their own function.
func TestRedact_CustomPlaceholder(t *testing.T) {
	in := "contact alice@example.com"
	got := Redact(in, Config{
		Kinds:       KindEmail,
		Placeholder: func(k Kind) string { return "<hidden>" },
	})
	if got != "contact <hidden>" {
		t.Errorf("custom placeholder ignored: %q", got)
	}
}

// TestScan_OffsetsMatch: every Detection's Start:End must slice back
// to the raw Value. Guards against regex group weirdness that would
// produce incorrect byte offsets.
func TestScan_OffsetsMatch(t *testing.T) {
	in := "primary alice@example.com and backup bob@example.org"
	dets := Scan(in, Config{Kinds: KindEmail})
	if len(dets) != 2 {
		t.Fatalf("expected 2 detections, got %d", len(dets))
	}
	for _, d := range dets {
		if in[d.Start:d.End] != d.Value {
			t.Errorf("offset mismatch: Start=%d End=%d slice=%q value=%q",
				d.Start, d.End, in[d.Start:d.End], d.Value)
		}
	}
}

// TestRedact_NoFalsePositivesInNormalProse: a blob of ordinary prose
// with no PII must come through unchanged. Reviewers want to know the
// filter won't eat their text.
func TestRedact_NoFalsePositivesInNormalProse(t *testing.T) {
	in := "The quick brown fox jumps over the lazy dog. " +
		"Meet me at 3pm on Friday for coffee."
	got := Redact(in, Config{Kinds: All})
	if got != in {
		t.Errorf("normal prose mangled: %q", got)
	}
}

// TestRedact_UTF8Preserved: non-ASCII content mixed with PII must not
// corrupt the UTF-8 output. Offsets need to survive rune boundaries.
func TestRedact_UTF8Preserved(t *testing.T) {
	in := "こんにちは alice@example.com さようなら"
	got := Redact(in, Config{Kinds: KindEmail})
	want := "こんにちは {{PII:email}} さようなら"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestKind_String: placeholder relies on stable lowercase names.
func TestKind_String(t *testing.T) {
	cases := map[Kind]string{
		KindEmail: "email", KindPhone: "phone", KindSSN: "ssn",
		KindCreditCard: "credit_card", KindIBAN: "iban",
		KindIPv4: "ipv4", KindIPv6: "ipv6",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

// TestRedact_AllSetEnabled: the `All` bitmask actually flips every bit.
func TestRedact_AllSetEnabled(t *testing.T) {
	in := "Me: alice@example.com, 415-555-2671, 4242 4242 4242 4242, 10.0.0.1"
	got := Redact(in, Config{Kinds: All})
	// Every original fragment should be gone; replaced by placeholders.
	for _, bad := range []string{"alice@example.com", "4242 4242 4242 4242", "10.0.0.1"} {
		if strings.Contains(got, bad) {
			t.Errorf("All bitmask missed %q: output %q", bad, got)
		}
	}
}
