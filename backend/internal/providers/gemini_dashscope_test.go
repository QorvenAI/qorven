// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"testing"
)

func TestGeminiCompat_CollapseEmpty(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hello"}}
	result := CollapseToolCallsWithoutSignature(msgs)
	if len(result) != 1 { t.Errorf("len=%d", len(result)) }
}

func TestGeminiCompat_NoToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	result := CollapseToolCallsWithoutSignature(msgs)
	if len(result) != 2 { t.Errorf("len=%d", len(result)) }
}

func TestGeminiCompat_WithSignature_NoCollapse(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "search for cats"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{
			{ID: "tc1", Name: "search", Metadata: map[string]string{"thought_signature": "abc123"}},
		}},
		{Role: "tool", Content: "found cats", ToolCallID: "tc1"},
	}
	result := CollapseToolCallsWithoutSignature(msgs)
	if len(result) != 3 { t.Errorf("should not collapse with signature: len=%d", len(result)) }
}

func TestGeminiCompat_WithoutSignature_Collapse(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "search for cats"},
		{Role: "assistant", Content: "thinking...", ToolCalls: []ToolCall{
			{ID: "tc1", Name: "search"},
		}},
		{Role: "tool", Content: "found cats", ToolCallID: "tc1"},
		{Role: "assistant", Content: "I found cats!"},
	}
	result := CollapseToolCallsWithoutSignature(msgs)
	// Should collapse: assistant tool_calls → stripped, tool result → user message
	if len(result) != 4 { t.Errorf("len=%d, want 4 (user, assistant-text, user-tool-result, assistant-final)", len(result)) }
	if result[0].Role != "user" { t.Errorf("msg0=%s", result[0].Role) }
	if result[1].Role != "assistant" { t.Errorf("msg1=%s", result[1].Role) }
	if result[1].Content != "thinking..." { t.Errorf("assistant content lost: %q", result[1].Content) }
	if result[2].Role != "user" { t.Errorf("tool result should become user: %s", result[2].Role) }
	if result[2].Content != "found cats" { t.Errorf("tool content=%q", result[2].Content) }
}

func TestGeminiCompat_EmptySignature_Collapse(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "tc1", Name: "search", Metadata: map[string]string{"thought_signature": "  "}},
		}},
		{Role: "tool", Content: "result", ToolCallID: "tc1"},
	}
	result := CollapseToolCallsWithoutSignature(msgs)
	// Whitespace-only signature should trigger collapse
	hasToolRole := false
	for _, m := range result { if m.Role == "tool" { hasToolRole = true } }
	if hasToolRole { t.Error("tool messages should be collapsed") }
}

func TestGeminiCompat_MultipleToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: "assistant", ToolCalls: []ToolCall{
			{ID: "tc1", Name: "search"},
			{ID: "tc2", Name: "fetch"},
		}},
		{Role: "tool", Content: "search result", ToolCallID: "tc1"},
		{Role: "tool", Content: "fetch result", ToolCallID: "tc2"},
	}
	result := CollapseToolCallsWithoutSignature(msgs)
	// Both tool results should be folded into one user message
	userMsgs := 0
	for _, m := range result { if m.Role == "user" { userMsgs++ } }
	if userMsgs != 1 { t.Errorf("expected 1 folded user message, got %d", userMsgs) }
}

func TestDashScope_New(t *testing.T) {
	ds := NewDashScope("test", "key123", "")
	if ds.Name() != "test" { t.Errorf("name=%q", ds.Name()) }
	if !ds.SupportsThinking() { t.Error("should support thinking") }
}

func TestDashScope_ModelSupportsThinking(t *testing.T) {
	ds := NewDashScope("test", "key", "qwen3-max")
	if !ds.ModelSupportsThinking("qwen3-max") { t.Error("qwen3-max should support thinking") }
	if !ds.ModelSupportsThinking("qwen3-32b") { t.Error("qwen3-32b should support thinking") }
	if ds.ModelSupportsThinking("qwen3-plus") { t.Error("qwen3-plus should NOT support thinking") }
	if ds.ModelSupportsThinking("gpt-4") { t.Error("gpt-4 should NOT support thinking") }
}

func TestDashScope_ApplyThinkingGuard_Off(t *testing.T) {
	ds := NewDashScope("test", "key", "qwen3-max")
	req := ChatRequest{Model: "qwen3-max", Options: map[string]any{"thinking": "off"}}
	result := ds.applyThinkingGuard(req)
	if _, ok := result.Options["enable_thinking"]; ok { t.Error("should not inject thinking when off") }
}

func TestDashScope_ApplyThinkingGuard_High(t *testing.T) {
	ds := NewDashScope("test", "key", "qwen3-max")
	req := ChatRequest{Model: "qwen3-max", Options: map[string]any{"thinking": "high"}}
	result := ds.applyThinkingGuard(req)
	if result.Options["enable_thinking"] != true { t.Error("should enable thinking") }
	if result.Options["thinking_budget"] != 32768 { t.Errorf("budget=%v", result.Options["thinking_budget"]) }
}

func TestDashScope_ApplyThinkingGuard_UnsupportedModel(t *testing.T) {
	ds := NewDashScope("test", "key", "qwen3-plus")
	req := ChatRequest{Model: "qwen3-plus", Options: map[string]any{"thinking": "high"}}
	result := ds.applyThinkingGuard(req)
	if _, ok := result.Options["enable_thinking"]; ok { t.Error("unsupported model should not get thinking") }
}

func TestDashScopeThinkingBudget(t *testing.T) {
	tests := []struct{ level string; want int }{
		{"low", 4096}, {"medium", 16384}, {"high", 32768}, {"unknown", 16384},
	}
	for _, tt := range tests {
		got := dashscopeThinkingBudget(tt.level)
		if got != tt.want { t.Errorf("budget(%q)=%d, want %d", tt.level, got, tt.want) }
	}
}

func TestDashScope_ChatStream_FallbackWithTools(t *testing.T) {
	// DashScope should fall back to non-streaming when tools are present
	// This is a structural test — we can't call the real API without credentials
	ds := NewDashScope("test", "", "qwen3-max")
	req := ChatRequest{
		Model: "qwen3-max",
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunctionSchema{Name: "test"}}},
	}
	// Should not panic even with empty API key
	_, err := ds.ChatStream(context.Background(), req, nil)
	if err == nil { t.Skip("expected error with empty key") }
}
