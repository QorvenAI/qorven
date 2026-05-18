// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

// hard_pipeline_test.go — Tests for the agent message pipeline.
// These verify the middleware chain, output sanitization, truncation handling,
// loop guard, and silent reply detection — the code that runs on EVERY message.

// ── Middleware Pipeline: runs on every inbound/outbound message ──

func TestHard_Middleware_InboundChain(t *testing.T) {
	p := NewMiddlewarePipeline()
	p.AddInbound(StripDeliveryKeywords)
	p.AddInbound(StripSchedulingWords)

	ctx := &MiddlewareContext{Channel: "telegram"}
	result := p.RunInbound("dm me the daily report every morning", ctx)

	if strings.Contains(result, "dm me") { t.Error("delivery keyword not stripped") }
	if strings.Contains(result, "every morning") { t.Error("scheduling word not stripped") }
	if result == "" { t.Error("entire message stripped — should keep task text") }
	if ctx.Direction != "inbound" { t.Error("direction not set") }
	t.Logf("inbound: %q → %q ✓", "dm me the daily report every morning", result)
}

func TestHard_Middleware_OutboundTruncation(t *testing.T) {
	p := NewMiddlewarePipeline()
	p.AddOutbound(TruncateForChannel)

	// Telegram limit is 4096
	longMsg := strings.Repeat("x", 5000)
	ctx := &MiddlewareContext{Channel: "telegram"}
	result := p.RunOutbound(longMsg, ctx)
	if len(result) > 4096 { t.Errorf("telegram: %d chars (limit 4096)", len(result)) }
	if !strings.HasSuffix(result, "...") { t.Error("should end with ...") }

	// SMS limit is 160
	ctx2 := &MiddlewareContext{Channel: "sms"}
	result2 := p.RunOutbound(longMsg, ctx2)
	if len(result2) > 160 { t.Errorf("sms: %d chars (limit 160)", len(result2)) }

	// Chat has no limit
	ctx3 := &MiddlewareContext{Channel: "chat"}
	result3 := p.RunOutbound(longMsg, ctx3)
	if len(result3) != 5000 { t.Error("chat should not truncate") }
}

func TestHard_Middleware_SanitizeOutput_ToolCallXML(t *testing.T) {
	input := "Here's the result.\n<tool_call>{\"name\":\"search\",\"args\":{\"q\":\"test\"}}</tool_call>\nThe answer is 42."
	result := SanitizeOutput(input, nil)
	if strings.Contains(result, "<tool_call>") { t.Error("tool_call XML not stripped") }
	if !strings.Contains(result, "42") { t.Error("answer lost") }
}

func TestHard_Middleware_SanitizeOutput_FunctionCallXML(t *testing.T) {
	input := "Let me search.\n<function_call>web_search(\"test\")</function_call>\nFound results."
	result := SanitizeOutput(input, nil)
	if strings.Contains(result, "<function_call>") { t.Error("function_call XML not stripped") }
	if !strings.Contains(result, "Found results") { t.Error("content lost") }
}

func TestHard_Middleware_SanitizeOutput_NestedTags(t *testing.T) {
	input := "<tool_call><tool_call>nested</tool_call></tool_call>clean text"
	result := SanitizeOutput(input, nil)
	if strings.Contains(result, "<tool_call>") { t.Error("nested tags not fully stripped") }
}

func TestHard_Middleware_ChainOrder(t *testing.T) {
	// Middleware should run in order — first added, first executed
	var order []string
	p := NewMiddlewarePipeline()
	p.AddOutbound(func(msg string, _ *MiddlewareContext) string {
		order = append(order, "first")
		return msg + " [1]"
	})
	p.AddOutbound(func(msg string, _ *MiddlewareContext) string {
		order = append(order, "second")
		return msg + " [2]"
	})

	result := p.RunOutbound("start", &MiddlewareContext{})
	if result != "start [1] [2]" { t.Errorf("chain order wrong: %q", result) }
	if len(order) != 2 || order[0] != "first" { t.Error("execution order wrong") }
}

// ── Silent Reply Detection ──

func TestHard_IsSilentReply_ShortPrefill(t *testing.T) {
	// Short messages starting with "I'll" or "Let me" are silent (agent is about to use tools)
	silent := []string{
		"I'll search for that.",
		"Let me check.",
		"Searching now...",
		"Looking into it.",
	}
	for _, msg := range silent {
		if !IsSilentReply(msg) { t.Errorf("should be silent: %q", msg) }
	}
}

func TestHard_IsSilentReply_RealContent(t *testing.T) {
	// Long messages or actual answers should NOT be silent
	notSilent := []string{
		"Here's what I found about Go programming. Go is a statically typed language...",
		"The answer is 42. Let me explain why.",
		"I'll explain the concept in detail. First, you need to understand that Go uses goroutines for concurrency.",
	}
	for _, msg := range notSilent {
		if IsSilentReply(msg) { t.Errorf("should NOT be silent: %q", msg[:50]) }
	}
}

// ── Truncation Handler: detects when LLM output was cut off ──

func TestHard_TruncationHandler_NormalResponse(t *testing.T) {
	h := NewTruncationHandler()
	resp := &providers.ChatResponse{Content: "Hello!", FinishReason: "stop"}
	retry, _ := h.Check(resp)
	if retry { t.Error("normal response should not trigger retry") }
}

func TestHard_TruncationHandler_TruncatedToolCall(t *testing.T) {
	h := NewTruncationHandler()
	resp := &providers.ChatResponse{
		FinishReason: "length",
		ToolCalls:    []providers.ToolCall{{ID: "tc1", Name: "search", Arguments: map[string]any{"q": "test"}}},
	}
	retry, hint := h.Check(resp)
	if !retry { t.Error("truncated tool call should trigger retry") }
	if hint == "" { t.Error("should provide hint about truncation") }
	if !strings.Contains(hint, "truncated") { t.Errorf("hint should mention truncation: %q", hint) }
}

func TestHard_TruncationHandler_MaxRetries(t *testing.T) {
	h := NewTruncationHandler()
	resp := &providers.ChatResponse{FinishReason: "length", ToolCalls: []providers.ToolCall{{ID: "tc1", Name: "x"}}}

	// Should retry up to max, then give up
	retried := 0
	for i := 0; i < 10; i++ {
		retry, _ := h.Check(resp)
		if retry { retried++ } else { break }
	}
	if retried > 3 { t.Errorf("should give up after max retries, retried %d times", retried) }
	if retried == 0 { t.Error("should retry at least once") }
}

func TestHard_TruncationHandler_NilResponse(t *testing.T) {
	h := NewTruncationHandler()
	retry, _ := h.Check(nil)
	if retry { t.Error("nil response should not trigger retry") }
}

// ── Loop Guard: prevents infinite tool loops ──

func TestHard_LoopGuard_DetectsRepeatedCalls(t *testing.T) {
	g := NewLoopGuard()

	// Same tool, same args, 10 times
	for i := 0; i < 10; i++ {
		hash := g.RecordCall("web_search", map[string]any{"query": "same query"})
		g.RecordResult("web_search", hash, "same result")
		g.RecordToolSuccess("web_search")

		result := g.DetectSameArgs("web_search", hash)
		if result.Level == DetectionCritical {
			t.Logf("loop detected at iteration %d ✓", i)
			return
		}
	}
	t.Error("should detect loop within 10 iterations")
}

func TestHard_LoopGuard_MutationResetsStreak(t *testing.T) {
	g := NewLoopGuard()

	// Build up a streak
	for i := 0; i < 3; i++ {
		hash := g.RecordCall("web_search", map[string]any{"query": "test"})
		g.RecordToolSuccess("web_search")
		g.DetectSameArgs("web_search", hash)
	}

	// Mutation resets
	g.RecordMutation("write_file")

	// New search should not be detected as loop
	hash := g.RecordCall("web_search", map[string]any{"query": "different"})
	result := g.DetectSameArgs("web_search", hash)
	if result.Level == DetectionCritical { t.Error("mutation should reset loop detection") }
}

func TestHard_LoopGuard_CircuitBreaker(t *testing.T) {
	g := NewLoopGuard()

	// 2 errors should break the circuit
	g.RecordToolError("flaky_api")
	g.RecordToolError("flaky_api")
	if !g.IsToolCircuitBroken("flaky_api") { t.Error("2 errors should break circuit") }

	// Different tool should not be affected
	if g.IsToolCircuitBroken("other_tool") { t.Error("other tool should not be broken") }

	// Success resets
	g.RecordToolSuccess("flaky_api")
	if g.IsToolCircuitBroken("flaky_api") { t.Error("success should reset circuit") }
}

func TestHard_LoopGuard_DifferentArgsNotLoop(t *testing.T) {
	g := NewLoopGuard()

	// Different args each time — should NOT detect loop
	for i := 0; i < 10; i++ {
		hash := g.RecordCall("web_search", map[string]any{"query": "query " + string(rune('A'+i))})
		g.RecordToolSuccess("web_search")
		result := g.DetectSameArgs("web_search", hash)
		if result.Level == DetectionCritical { t.Errorf("different args should not trigger loop at iteration %d", i) }
	}
}

// ── Unique Tool Call IDs ──

func TestHard_UniqueToolCallIDs_NoDuplicates(t *testing.T) {
	calls := []providers.ToolCall{
		{ID: "call_1", Name: "search"},
		{ID: "call_1", Name: "read"},  // duplicate ID!
		{ID: "call_2", Name: "write"},
	}

	unique := uniquifyToolCallIDs(calls, "run_1", 0)
	seen := map[string]bool{}
	for _, c := range unique {
		if seen[c.ID] { t.Errorf("duplicate ID after uniquify: %q", c.ID) }
		seen[c.ID] = true
	}
}

// ── StripDeliveryKeywords edge cases ──

func TestHard_StripDeliveryKeywords_CaseInsensitive(t *testing.T) {
	result := StripDeliveryKeywords("DM ME the report", nil)
	if strings.Contains(strings.ToLower(result), "dm me") { t.Error("should be case-insensitive") }
}

func TestHard_StripDeliveryKeywords_PreservesContent(t *testing.T) {
	result := StripDeliveryKeywords("dm me the quarterly report with charts", nil)
	if !strings.Contains(result, "report") { t.Error("should preserve task content") }
}
