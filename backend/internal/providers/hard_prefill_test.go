// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"testing"
)

// hard_prefill_test.go — Tests for thinking-only prefill continuation and signature stripping.

func TestHard_NeedsPrefillContinuation_ThinkingOnly(t *testing.T) {
	// Model returned reasoning but no text — needs continuation
	resp := &ChatResponse{Thinking: "Let me analyze this step by step...", Content: ""}
	if !NeedsPrefillContinuation(resp) {
		t.Error("thinking-only response should need prefill continuation")
	}
}

func TestHard_NeedsPrefillContinuation_HasContent(t *testing.T) {
	// Model returned both thinking and content — no continuation needed
	resp := &ChatResponse{Thinking: "reasoning...", Content: "Here's the answer"}
	if NeedsPrefillContinuation(resp) {
		t.Error("response with content should NOT need continuation")
	}
}

func TestHard_NeedsPrefillContinuation_HasToolCalls(t *testing.T) {
	// Model returned thinking + tool calls — no continuation needed
	resp := &ChatResponse{
		Thinking:  "I should search for this...",
		ToolCalls: []ToolCall{{ID: "tc1", Name: "web_search"}},
	}
	if NeedsPrefillContinuation(resp) {
		t.Error("response with tool calls should NOT need continuation")
	}
}

func TestHard_NeedsPrefillContinuation_NilResponse(t *testing.T) {
	if NeedsPrefillContinuation(nil) {
		t.Error("nil response should NOT need continuation")
	}
}

func TestHard_NeedsPrefillContinuation_EmptyResponse(t *testing.T) {
	resp := &ChatResponse{}
	if NeedsPrefillContinuation(resp) {
		t.Error("empty response (no thinking) should NOT need continuation")
	}
}

func TestHard_NeedsPrefillContinuation_WhitespaceContent(t *testing.T) {
	// Content is just whitespace — should still need continuation
	resp := &ChatResponse{Thinking: "reasoning...", Content: "   \n  "}
	if !NeedsPrefillContinuation(resp) {
		t.Error("whitespace-only content should need continuation")
	}
}

func TestHard_BuildPrefillMessage_Structure(t *testing.T) {
	resp := &ChatResponse{Thinking: "Step 1: analyze. Step 2: respond."}
	msg := BuildPrefillMessage(resp)

	if msg.Role != "assistant" { t.Errorf("role: %q", msg.Role) }
	if msg.Thinking != resp.Thinking { t.Error("thinking not preserved") }
	if msg.Content != "" { t.Error("content should be empty for prefill") }
}

func TestHard_StripThinkingSignatures_PreservesLast(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi", Thinking: "old thinking 1"},
		{Role: "user", Content: "What's 2+2?"},
		{Role: "assistant", Content: "4", Thinking: "current thinking"},
	}

	stripped := StripThinkingSignatures(messages)

	// First assistant's thinking should be stripped
	if stripped[1].Thinking != "" {
		t.Errorf("old thinking should be stripped, got %q", stripped[1].Thinking)
	}

	// Last assistant's thinking should be preserved
	if stripped[3].Thinking != "current thinking" {
		t.Errorf("last thinking should be preserved, got %q", stripped[3].Thinking)
	}

	// Content should be untouched
	if stripped[1].Content != "Hi" { t.Error("content should be preserved") }
	if stripped[3].Content != "4" { t.Error("content should be preserved") }
}

func TestHard_StripThinkingSignatures_SingleAssistant(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi", Thinking: "only thinking"},
	}

	stripped := StripThinkingSignatures(messages)
	// Only one assistant — should preserve its thinking
	if stripped[1].Thinking != "only thinking" {
		t.Error("single assistant's thinking should be preserved")
	}
}

func TestHard_StripThinkingSignatures_NoAssistant(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "system", Content: "You are helpful"},
	}
	stripped := StripThinkingSignatures(messages)
	if len(stripped) != 2 { t.Error("should return same number of messages") }
}

func TestHard_StripThinkingSignatures_ManyAssistants(t *testing.T) {
	messages := []Message{
		{Role: "assistant", Content: "a1", Thinking: "t1"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a2", Thinking: "t2"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a3", Thinking: "t3"},
		{Role: "user", Content: "u3"},
		{Role: "assistant", Content: "a4", Thinking: "t4"},
	}

	stripped := StripThinkingSignatures(messages)

	// Only the LAST assistant (a4) should keep thinking
	for i, m := range stripped {
		if m.Role == "assistant" {
			if i == 6 { // last assistant
				if m.Thinking == "" { t.Error("last assistant should keep thinking") }
			} else {
				if m.Thinking != "" { t.Errorf("assistant at %d should have thinking stripped", i) }
			}
		}
	}
}
