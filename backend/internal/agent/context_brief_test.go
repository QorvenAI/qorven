package agent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderOrgKnowledge_CapsAndFormats(t *testing.T) {
	briefs := []string{"alpha fact", "beta fact"}
	out := renderOrgKnowledge(briefs, 1000)
	if !strings.Contains(out, "## Organizational Knowledge") {
		t.Errorf("missing header: %q", out)
	}
	if !strings.Contains(out, "alpha fact") || !strings.Contains(out, "beta fact") {
		t.Errorf("missing brief content: %q", out)
	}
}

func TestRenderOrgKnowledge_Empty(t *testing.T) {
	if renderOrgKnowledge(nil, 1000) != "" {
		t.Error("expected empty string for no briefs")
	}
}

func TestRenderOrgKnowledge_TruncatesToCap(t *testing.T) {
	big := strings.Repeat("x", 5000)
	out := renderOrgKnowledge([]string{big}, 200)
	if len(out) > 400 { // header + ~200 cap + truncation marker, generous bound
		t.Errorf("expected truncation near cap, got len %d", len(out))
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected truncation marker in output")
	}
}

func TestRenderOrgKnowledge_TruncatesOnRuneBoundary(t *testing.T) {
	// 1000 em-dashes (3 bytes each in UTF-8); cap mid-rune.
	body := strings.Repeat("—", 1000)
	out := renderOrgKnowledge([]string{body}, 100)
	if !utf8.ValidString(out) {
		t.Errorf("truncated output is not valid UTF-8")
	}
	if !strings.Contains(out, "…") {
		t.Errorf("expected truncation marker")
	}
}
