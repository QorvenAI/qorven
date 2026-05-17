// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package agent

import (
	"strings"
	"testing"
)

// TestParseVisionAction_Clean: a minimal valid JSON action parses.
func TestParseVisionAction_Clean(t *testing.T) {
	in := `{"thoughts":"I see the search box","action_type":"click","x":440,"y":312}`
	a, err := parseVisionAction(in)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if a.ActionType != "click" || a.X != 440 || a.Y != 312 {
		t.Errorf("parsed wrong: %+v", a)
	}
}

// TestParseVisionAction_Fenced: LLMs often ignore "no fences" — the
// parser must strip ```json ... ``` blocks so we don't drop valid
// output.
func TestParseVisionAction_Fenced(t *testing.T) {
	cases := []string{
		"```json\n{\"action_type\":\"press\",\"key\":\"Enter\"}\n```",
		"```\n{\"action_type\":\"press\",\"key\":\"Enter\"}\n```",
		"Here's the action: {\"action_type\":\"press\",\"key\":\"Enter\"}\nThat should do it.",
	}
	for _, c := range cases {
		a, err := parseVisionAction(c)
		if err != nil {
			t.Errorf("should parse %q: %v", c, err)
			continue
		}
		if a.ActionType != "press" || a.Key != "Enter" {
			t.Errorf("wrong parse for %q: %+v", c, a)
		}
	}
}

// TestParseVisionAction_EmptyActionType: action_type is mandatory —
// an object without it is a parse error, not a silent fallback.
func TestParseVisionAction_EmptyActionType(t *testing.T) {
	_, err := parseVisionAction(`{"thoughts":"hmm"}`)
	if err == nil {
		t.Fatal("empty action_type should error")
	}
	if !strings.Contains(err.Error(), "action_type") {
		t.Errorf("error should mention action_type; got %q", err)
	}
}

// TestParseVisionAction_Malformed: junk input yields an error.
func TestParseVisionAction_Malformed(t *testing.T) {
	for _, raw := range []string{
		"",
		"not json",
		"{",
		"{\"action_type\"",
	} {
		if _, err := parseVisionAction(raw); err == nil {
			t.Errorf("malformed %q should error", raw)
		}
	}
}

// TestCompatAction_PreservesFields: converting to AgentAction for
// the summary output must keep thoughts/memory/action_type intact so
// the user-facing step log isn't corrupted.
func TestCompatAction_PreservesFields(t *testing.T) {
	v := &VisionAction{
		Thoughts:   "see the button",
		Memory:     "signup flow",
		ActionType: "click",
		X:          100, Y: 200,
		Text: "ignore", URL: "http://x",
	}
	c := compatAction(v)
	if c.Thoughts != v.Thoughts || c.Memory != v.Memory || c.ActionType != v.ActionType {
		t.Errorf("compat lost fields: %+v", c)
	}
	// URL should also ride along — we use it for the "navigated to X"
	// summary line.
	if c.URL != v.URL {
		t.Errorf("URL not preserved: got %q want %q", c.URL, v.URL)
	}
}

// TestModelSupportsVision_KnownPositives: every model in our catalog
// marked SupportsVision=true should be detected by the heuristic.
// Regression guard against future model-naming changes.
func TestModelSupportsVision_KnownPositives(t *testing.T) {
	positives := []string{
		"claude-3-5-sonnet-20241022",
		"claude-sonnet-4-6",
		"claude-opus-4-7",
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-5-preview-2025-01-15",
		"gemini-1.5-pro",
		"gemini-2.0-flash",
		"amazon-nova/nova-pro-v1",
		"mistralai/pixtral-large",
		"o1-preview",
		"o3-mini",
	}
	for _, m := range positives {
		if !modelSupportsVision(m) {
			t.Errorf("model %q should be detected as vision-capable", m)
		}
	}
}

// TestModelSupportsVision_KnownNegatives: text-only models must NOT
// be misidentified — triggering vision mode on them returns API
// errors like "content_parts not allowed" and wastes a whole turn.
func TestModelSupportsVision_KnownNegatives(t *testing.T) {
	negatives := []string{
		"gpt-3.5-turbo",
		"mistral-7b-instruct",
		"llama-3.1-70b",
		"deepseek-v3",
		"qwen-2.5-72b",
		"text-davinci-003",
		"",
	}
	for _, m := range negatives {
		if modelSupportsVision(m) {
			t.Errorf("model %q wrongly detected as vision-capable", m)
		}
	}
}

// TestModelSupportsVision_CaseInsensitive: users sometimes pass model
// names in weird casings. The detector must not care.
func TestModelSupportsVision_CaseInsensitive(t *testing.T) {
	variants := []string{
		"Claude-Opus-4-7",
		"GPT-4O",
		"Gemini-2.0-Flash",
		"gemini-2.0-flash",
	}
	for _, m := range variants {
		if !modelSupportsVision(m) {
			t.Errorf("case variant %q should still match", m)
		}
	}
}
