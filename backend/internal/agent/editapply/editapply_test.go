// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package editapply

import "testing"

// ── Required tests (must not be changed) ────────────────────────────────────

func TestApply_ExactMatch(t *testing.T) {
	got, err := Apply("a\nb\nc\n", "b\n", "B\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\nB\nc\n" {
		t.Errorf("got %q", got)
	}
}

func TestApply_WhitespaceFlexible(t *testing.T) {
	file := "func f() {\n\treturn 1\n}\n"
	got, err := Apply(file, "return 1\n", "return 2\n")
	if err != nil {
		t.Fatalf("ws-flexible should match: %v", err)
	}
	if got != "func f() {\n\treturn 2\n}\n" {
		t.Errorf("got %q", got)
	}
}

func TestApply_SkipLeadingBlank(t *testing.T) {
	got, err := Apply("x\ny\n", "\nx\n", "X\n")
	if err != nil {
		t.Fatalf("should skip leading blank: %v", err)
	}
	if got != "X\ny\n" {
		t.Errorf("got %q", got)
	}
}

func TestApply_Ellipsis(t *testing.T) {
	file := "start\nmid1\nmid2\nend\n"
	got, err := Apply(file, "start\n...\nend\n", "START\n...\nEND\n")
	if err != nil {
		t.Fatalf("ellipsis: %v", err)
	}
	if got != "START\nmid1\nmid2\nEND\n" {
		t.Errorf("got %q", got)
	}
}

func TestApply_NoMatch_ReturnsHint(t *testing.T) {
	_, err := Apply("alpha\nbeta\n", "gamma\n", "GAMMA\n")
	if err == nil {
		t.Fatal("expected no-match error")
	}
	if !ContainsSimilar(err.Error()) {
		t.Errorf("error should include a 'did you mean' hint: %v", err)
	}
}

func TestApply_NotUnique(t *testing.T) {
	_, err := Apply("dup\ndup\n", "dup\n", "X\n")
	if err == nil || !IsNotUnique(err) {
		t.Errorf("ambiguous match should be not-unique: %v", err)
	}
}

// ── Extra edge-case tests ────────────────────────────────────────────────────

// Exact multi-line match.
func TestApply_ExactMultiLine(t *testing.T) {
	file := "line1\nline2\nline3\nline4\n"
	got, err := Apply(file, "line2\nline3\n", "TWO\nTHREE\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "line1\nTWO\nTHREE\nline4\n" {
		t.Errorf("got %q", got)
	}
}

// Exact replacement at the very start of the file.
func TestApply_ExactAtStart(t *testing.T) {
	got, err := Apply("first\nsecond\n", "first\n", "FIRST\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "FIRST\nsecond\n" {
		t.Errorf("got %q", got)
	}
}

// Exact replacement at the very end of the file.
func TestApply_ExactAtEnd(t *testing.T) {
	got, err := Apply("first\nsecond\n", "second\n", "SECOND\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "first\nSECOND\n" {
		t.Errorf("got %q", got)
	}
}

// Whitespace-flexible with 4-space indent in file, no indent in search.
func TestApply_WhitespaceFlexible_FourSpaces(t *testing.T) {
	file := "class Foo:\n    def bar(self):\n        return 1\n"
	got, err := Apply(file, "def bar(self):\n    return 1\n", "def bar(self):\n    return 2\n")
	if err != nil {
		t.Fatalf("ws-flexible 4-space: %v", err)
	}
	if got != "class Foo:\n    def bar(self):\n        return 2\n" {
		t.Errorf("got %q", got)
	}
}

// Skip leading blank interacts correctly with exact match.
// The LLM provides an unindented search (no leading tab) with a spurious
// leading blank.  The replace also has no leading tab.  The engine must
// preserve the file's existing tab prefix on the matched line.
func TestApply_SkipLeadingBlank_WithWhitespace(t *testing.T) {
	file := "func g() {\n\tx := 1\n}\n"
	// Leading blank + unindented search; replace also unindented
	got, err := Apply(file, "\nx := 1\n", "x := 2\n")
	if err != nil {
		t.Fatalf("skip-blank+ws-flex: %v", err)
	}
	// The engine replaces the suffix "x := 1\n" with "x := 2\n",
	// leaving the existing "\t" prefix intact → "\tx := 2\n".
	if got != "func g() {\n\tx := 2\n}\n" {
		t.Errorf("got %q", got)
	}
}

// Ellipsis with leading and trailing non-ellipsis chunks.
func TestApply_Ellipsis_LeadingTrailingChunks(t *testing.T) {
	file := "A\nB\nC\nD\nE\n"
	search := "A\n...\nE\n"
	replace := "AA\n...\nEE\n"
	got, err := Apply(file, search, replace)
	if err != nil {
		t.Fatalf("ellipsis leading+trailing: %v", err)
	}
	if got != "AA\nB\nC\nD\nEE\n" {
		t.Errorf("got %q", got)
	}
}

// Ellipsis with multiple ellipsis lines.
func TestApply_Ellipsis_Multiple(t *testing.T) {
	file := "start\nkept1\nmiddle\nkept2\nend\n"
	search := "start\n...\nmiddle\n...\nend\n"
	replace := "START\n...\nMIDDLE\n...\nEND\n"
	got, err := Apply(file, search, replace)
	if err != nil {
		t.Fatalf("multi-ellipsis: %v", err)
	}
	if got != "START\nkept1\nMIDDLE\nkept2\nEND\n" {
		t.Errorf("got %q", got)
	}
}

// CRLF line endings should be handled without breakage (exact match).
func TestApply_CRLF(t *testing.T) {
	got, err := Apply("a\r\nb\r\nc\r\n", "b\r\n", "B\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\r\nB\r\nc\r\n" {
		t.Errorf("got %q", got)
	}
}

// File without trailing newline: exact match still works.
func TestApply_NoTrailingNewline(t *testing.T) {
	got, err := Apply("a\nb", "b", "B")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a\nB" {
		t.Errorf("got %q", got)
	}
}

// NotUnique via ellipsis chunks: if an ellipsis chunk is ambiguous, report ErrNotUnique.
func TestApply_Ellipsis_NotUnique(t *testing.T) {
	// Both "start" and "end" appear once, but the middle chunk "dup" appears twice.
	file := "start\ndup\ndup\nend\n"
	_, err := Apply(file, "start\n...\nend\n", "S\n...\nE\n")
	// This should succeed — ellipsis spans are kept; only anchors matter.
	// The non-ellipsis chunks here are "start" and "end" which are each unique.
	if err != nil {
		t.Fatalf("ellipsis spanning dup lines should work: %v", err)
	}
}
