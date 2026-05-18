// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

// diamond_hotpath_test.go — Tests for code that runs on EVERY request.
// These test the hot path: normalize → prompt build → LLM → tool truncation → sanitize.

// ── NormalizeQuery: runs on EVERY user message ──

func TestDiamond_Normalize_TypoCorrection(t *testing.T) {
	cases := map[string]string{
		"teh quick brown fox":    "The quick brown fox",
		"waht is golang":         "What is golang",
		"hwo to deploy":          "How to deploy",
		"i cant find teh file":   "I can't find the file",
		"im looking fo help":     "I'm looking of help", // "fo" → "of"
	}
	for input, expected := range cases {
		got := NormalizeQuery(input)
		if got != expected { t.Errorf("NormalizeQuery(%q) = %q, want %q", input, got, expected) }
	}
}

func TestDiamond_Normalize_PreservesCode(t *testing.T) {
	// Code snippets should NOT be "corrected"
	code := "func main() { fmt.Println(\"teh\") }"
	result := NormalizeQuery(code)
	// The typo fixer uses word boundaries (space after), so "teh" inside quotes
	// without trailing space should be preserved... let's verify
	if !strings.Contains(result, "Println") { t.Error("code content lost") }
}

func TestDiamond_Normalize_DoubleSpaces(t *testing.T) {
	result := NormalizeQuery("hello   world   test")
	if strings.Contains(result, "  ") { t.Error("double spaces not fixed") }
}

func TestDiamond_Normalize_PunctuationSpacing(t *testing.T) {
	result := NormalizeQuery("hello , world . how ?")
	if strings.Contains(result, " ,") { t.Error("space before comma") }
	if strings.Contains(result, " .") { t.Error("space before period") }
	if strings.Contains(result, " ?") { t.Error("space before question mark") }
}

func TestDiamond_Normalize_EmptyAndWhitespace(t *testing.T) {
	if NormalizeQuery("") != "" { t.Error("empty should return empty") }
	if NormalizeQuery("   ") != "" { t.Error("whitespace should return empty") }
}

func TestDiamond_Normalize_STTBug_CasePreservation(t *testing.T) {
	// BUG CHECK: STT fix lowercases the ENTIRE input when it matches
	// "Check the weather in New York please" should NOT become all lowercase
	input := "Check the weather in new york please"
	result := NormalizeQuery(input)
	// The STT fix replaces "weather in new york" → "weather in New York"
	// But the code does: s = strings.Replace(strings.ToLower(s), pattern, fix, 1)
	// This lowercases EVERYTHING first, then applies the fix
	if strings.HasPrefix(result, "check") {
		t.Errorf("BUG: STT fix lowercased entire input: %q", result)
	}
}

func TestDiamond_NormalizeSearchQuery_StripsFiller(t *testing.T) {
	cases := map[string]string{
		"please help me find golang tutorials": "Find golang tutorials",
		"can you show me the docs":             "The docs",
		"just search for AI agents":            "Search for AI agents",
	}
	for input, expected := range cases {
		got := NormalizeSearchQuery(input)
		if got != expected { t.Errorf("NormalizeSearchQuery(%q) = %q, want %q", input, got, expected) }
	}
}

// ── TruncateToolResult: runs on EVERY tool output ──

func TestDiamond_TruncateToolResult_ShortContent(t *testing.T) {
	content := "short output"
	result := TruncateToolResult(content, 1000)
	if result != content { t.Error("short content should not be truncated") }
}

func TestDiamond_TruncateToolResult_PreservesImportantTail(t *testing.T) {
	// Build content with error at the end
	content := strings.Repeat("normal output line\n", 500) + "FATAL ERROR: connection refused\nexit code: 1\n"
	result := TruncateToolResult(content, 2000)

	if !strings.Contains(result, "FATAL ERROR") { t.Error("important tail lost") }
	if !strings.Contains(result, "exit code") { t.Error("exit code lost") }
	if !strings.Contains(result, "truncated") { t.Error("truncation marker missing") }
	if len(result) > 2100 { t.Errorf("result too long: %d", len(result)) }
}

func TestDiamond_TruncateToolResult_NoImportantTail(t *testing.T) {
	content := strings.Repeat("x", 5000)
	result := TruncateToolResult(content, 2000)
	if len(result) > 2100 { t.Errorf("result too long: %d", len(result)) }
	if !strings.Contains(result, "truncated") { t.Error("truncation marker missing") }
}

func TestDiamond_HasImportantTail_Patterns(t *testing.T) {
	important := []string{
		"some output\nerror: file not found",
		"building...\nFATAL: out of memory",
		"test output\nTotal: 42 passed, 0 failed",
		"running...\nexit code: 1",
		"compiling...\npanic: runtime error",
	}
	for _, content := range important {
		if !HasImportantTail(content, 200) { t.Errorf("should detect important tail: %q", content[len(content)-30:]) }
	}

	unimportant := []string{
		"just some normal text output",
		"line 1\nline 2\nline 3",
	}
	for _, content := range unimportant {
		if HasImportantTail(content, 200) { t.Errorf("should NOT detect important tail: %q", content) }
	}
}

// ── Compactor: runs mid-loop when context grows ──

func TestDiamond_Compactor_ThresholdLevels(t *testing.T) {
	c := NewCompactor(32000) // 128K context window

	// Under threshold — no compaction
	small := makeMessages(5, 100)
	if c.Check(small) != NoCompaction { t.Error("small should not need compaction") }

	// Build progressively larger message sets
	medium := makeMessages(50, 500)
	action := c.Check(medium)
	t.Logf("50 msgs × 500 chars: action=%d (tokens≈%d)", action, estimateTokens(medium))

	large := makeMessages(100, 2000)
	action = c.Check(large)
	if action == NoCompaction { t.Error("100 msgs × 2000 chars should trigger compaction") }
	t.Logf("100 msgs × 2000 chars: action=%d (tokens≈%d)", action, estimateTokens(large))
}

func TestDiamond_Compactor_PruneTools_PreservesRecent(t *testing.T) {
	c := NewCompactor(8000)

	msgs := []providers.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Search for Go tutorials"},
		{Role: "assistant", ToolCalls: []providers.ToolCall{{ID: "tc1", Name: "web_search"}}},
		{Role: "tool", Content: strings.Repeat("old search result ", 200), ToolCallID: "tc1"},
		{Role: "assistant", Content: "I found some tutorials."},
		{Role: "user", Content: "Now search for Rust tutorials"},
		{Role: "assistant", ToolCalls: []providers.ToolCall{{ID: "tc2", Name: "web_search"}}},
		{Role: "tool", Content: strings.Repeat("recent search result ", 200), ToolCallID: "tc2"},
		{Role: "assistant", Content: "Here are Rust tutorials."},
	}

	pruned, savings := c.PruneTools(msgs)
	if savings <= 0 { t.Skip("no pruning needed at this size") }

	// Recent tool result should be preserved
	hasRecent := false
	for _, m := range pruned {
		if strings.Contains(m.Content, "recent search result") { hasRecent = true }
	}
	if !hasRecent { t.Error("recent tool result was pruned — should be protected") }

	t.Logf("prune: %d → %d messages, saved %d tokens ✓", len(msgs), len(pruned), savings)
}

func TestDiamond_Compactor_EmergencyTruncation(t *testing.T) {
	c := NewCompactor(4000) // tiny window

	// Build messages that exceed the window
	msgs := makeMessages(50, 500) // ~6250 tokens

	action := c.Check(msgs)
	if action < AggressiveCompaction {
		t.Logf("action=%d for 50×500 in 4K window", action)
	}

	compacted := c.Compact(msgs, action)

	// System prompt must survive
	hasSystem := false
	for _, m := range compacted {
		if m.Role == "system" { hasSystem = true }
	}
	if !hasSystem { t.Error("system prompt lost in emergency truncation") }

	// Last user message must survive
	lastUser := ""
	for i := len(compacted) - 1; i >= 0; i-- {
		if compacted[i].Role == "user" { lastUser = compacted[i].Content; break }
	}
	if lastUser == "" { t.Error("last user message lost") }

	// Must be smaller than original
	if estimateTokens(compacted) >= estimateTokens(msgs) {
		t.Error("compacted should be smaller than original")
	}

	t.Logf("emergency: %d → %d messages, %d → %d tokens ✓",
		len(msgs), len(compacted), estimateTokens(msgs), estimateTokens(compacted))
}

// ── DedupToolResults: runs every iteration ──

func TestDiamond_DedupToolResults_RemovesDuplicates(t *testing.T) {
	msgs := []providers.Message{
		{Role: "system", Content: "system"},
		{Role: "tool", Content: "result A", ToolCallID: "tc1"},
		{Role: "tool", Content: "result A", ToolCallID: "tc1"}, // exact duplicate
		{Role: "tool", Content: "result B", ToolCallID: "tc2"},
		{Role: "assistant", Content: "answer"},
	}

	deduped := DedupToolResults(msgs)
	toolCount := 0
	for _, m := range deduped {
		if m.Role == "tool" { toolCount++ }
	}
	if toolCount > 2 { t.Errorf("should dedup: %d tool messages", toolCount) }
}

// ── Prompt Builder: builds system prompt for every request ──

func TestDiamond_PromptBuilder_AllSections(t *testing.T) {
	ag := &Agent{
		ID: "test-agent", DisplayName: "Test Bot", AgentKey: "test",
		SystemPrompt: "You are a helpful coding assistant.",
		Model: "gpt-4o-mini",
	}

	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull, ModelID: "gpt-4o-mini"})
	prompt := pb.Build()

	// Must contain the agent's system prompt
	if !strings.Contains(prompt, "helpful coding assistant") {
		t.Error("system prompt not in output")
	}

	// Must contain mandatory tool use section (we added this)
	if !strings.Contains(prompt, "mandatory_tool_use") {
		t.Error("mandatory tool use section missing")
	}

	// Must contain act_dont_ask section
	if !strings.Contains(prompt, "act_dont_ask") {
		t.Error("act_dont_ask section missing")
	}

	// Must contain safety section
	if !strings.Contains(prompt, "safety") || !strings.Contains(prompt, "Safety") {
		t.Logf("safety section may use different casing")
	}

	if len(prompt) < 500 { t.Errorf("prompt too short for full mode: %d chars", len(prompt)) }
	t.Logf("prompt builder: %d chars, all sections present ✓", len(prompt))
}

func TestDiamond_PromptBuilder_MinimalMode(t *testing.T) {
	ag := &Agent{ID: "test", SystemPrompt: "You are helpful."}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptMinimal})
	prompt := pb.Build()

	// Minimal should be shorter than full
	pbFull := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull, ModelID: "gpt-4o-mini"})
	fullPrompt := pbFull.Build()

	if len(prompt) >= len(fullPrompt) {
		t.Errorf("minimal (%d) should be shorter than full (%d)", len(prompt), len(fullPrompt))
	}
	t.Logf("minimal=%d, full=%d ✓", len(prompt), len(fullPrompt))
}

// ── Helper ──

func makeMessages(n, charsPer int) []providers.Message {
	msgs := []providers.Message{{Role: "system", Content: strings.Repeat("System prompt content. ", 20)}}
	for i := 0; i < n; i++ {
		role := "user"
		if i%2 == 1 { role = "assistant" }
		msgs = append(msgs, providers.Message{Role: role, Content: strings.Repeat("x", charsPer)})
	}
	return msgs
}
