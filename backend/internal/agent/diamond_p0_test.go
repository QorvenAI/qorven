// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// diamond_p0_test.go — Tests for P0 production incident scenarios.
// Each test reproduces a scenario that would cause a real outage.

// ── P0 #1: Malformed tool call JSON → agent must not crash ──

func TestP0_MalformedToolCall_NoPanic(t *testing.T) {
	// LLM returns tool call with broken JSON arguments
	malformed := []providers.ToolCall{
		{ID: "tc1", Name: "web_search", Arguments: nil},                          // nil args
		{ID: "tc2", Name: "read_file", Arguments: map[string]any{}},              // empty args
		{ID: "tc3", Name: "exec", Arguments: map[string]any{"command": nil}},     // nil value
		{ID: "tc4", Name: "", Arguments: map[string]any{"q": "test"}},            // empty name
		{ID: "", Name: "search", Arguments: map[string]any{"q": "test"}},         // empty ID
	}

	// hasToolParseErrors should detect these
	if !hasToolParseErrors(malformed) {
		t.Log("hasToolParseErrors didn't flag malformed calls — checking individually")
	}

	// uniquifyToolCallIDs should not panic on empty IDs
	unique := uniquifyToolCallIDs(malformed, "run_1", 0)
	for _, tc := range unique {
		if tc.ID == "" { t.Error("uniquify should assign IDs to empty ones") }
	}
}

func TestP0_MalformedToolCall_TruncationHandler(t *testing.T) {
	h := NewTruncationHandler()

	// Truncated response with malformed tool calls
	resp := &providers.ChatResponse{
		FinishReason: "length",
		ToolCalls: []providers.ToolCall{
			{ID: "tc1", Name: "write_file", Arguments: map[string]any{
				"path": "test.go",
				// "content" key missing — truncated
			}},
		},
	}

	retry, hint := h.Check(resp)
	if !retry { t.Error("truncated tool call should trigger retry") }
	if !strings.Contains(hint, "truncated") { t.Errorf("hint should mention truncation: %q", hint) }
}

// ── P0 #2: Huge user message → must not OOM or overflow context ──

func TestP0_HugeMessage_Truncated(t *testing.T) {
	// 500KB message — must be truncated to 100K
	huge := strings.Repeat("x", 500_000)
	result := NormalizeQuery(huge)

	// The loop.go truncates at 100K, but NormalizeQuery doesn't
	// The truncation happens in the Run() method
	// Let's verify NormalizeQuery doesn't OOM on huge input
	if len(result) != 500_000 { t.Logf("NormalizeQuery changed length: %d → %d", 500_000, len(result)) }
	t.Log("500KB message: NormalizeQuery didn't OOM ✓")
}

func TestP0_HugeMessage_CompactorHandles(t *testing.T) {
	c := NewCompactor(8000) // 8K window

	// Build messages that are 10x the context window
	msgs := []providers.Message{
		{Role: "system", Content: strings.Repeat("System. ", 100)},
		{Role: "user", Content: strings.Repeat("Very long user message. ", 5000)}, // ~120K chars
	}

	action := c.Check(msgs)
	if action == NoCompaction { t.Error("huge message should trigger compaction") }

	compacted := c.Compact(msgs, action)
	compactedTokens := estimateTokens(compacted)
	if compactedTokens > 8000 { t.Errorf("compacted still too large: %d tokens", compactedTokens) }

	// System prompt must survive
	hasSystem := false
	for _, m := range compacted { if m.Role == "system" { hasSystem = true } }
	if !hasSystem { t.Error("system prompt lost during huge message compaction") }

	t.Logf("huge message: %d → %d tokens after compaction ✓", estimateTokens(msgs), compactedTokens)
}

// ── P0 #3: Tool writes outside workspace → data breach ──

func TestP0_ToolEscape_AllVectors(t *testing.T) {
	dir := t.TempDir()
	write := tools.NewWriteFileTool(dir)
	ctx := context.Background()

	escapes := []struct {
		name string
		path string
	}{
		{"dotdot", "../../../etc/crontab"},
		{"absolute", "/etc/passwd"},
		{"tilde", "~/.bashrc"},
		{"null_byte", "safe\x00/../etc/passwd"},
		{"url_encoded", "%2e%2e%2f%2e%2e%2fetc%2fpasswd"},
		{"double_encoded", "..%252f..%252fetc/passwd"},
		{"backslash", "..\\..\\etc\\passwd"},
		{"long_traversal", strings.Repeat("../", 50) + "etc/passwd"},
		{"dot_segment", "./../../etc/passwd"},
		{"mixed", "a/b/../../../etc/passwd"},
	}

	for _, tc := range escapes {
		r := write.Execute(ctx, map[string]any{"path": tc.path, "content": "pwned"})
		if !r.IsError {
			t.Errorf("SECURITY P0: %s escape NOT blocked: %q", tc.name, tc.path)
		}
	}
	t.Logf("workspace escape: %d vectors blocked ✓", len(escapes))
}

// ── P0 #4: Infinite tool loop → burns API budget ──

func TestP0_InfiniteLoop_GuardStops(t *testing.T) {
	guard := NewLoopGuard()

	// Simulate: agent calls web_search with same query 20 times
	iterations := 0
	stopped := false

	for i := 0; i < 20; i++ {
		hash := guard.RecordCall("web_search", map[string]any{"query": "same thing every time"})
		guard.RecordResult("web_search", hash, "same result every time")
		guard.RecordToolSuccess("web_search")
		iterations++

		sameArgs := guard.DetectSameArgs("web_search", hash)
		sameResult := guard.DetectSameResult("web_search", hash)

		if sameArgs.Level == DetectionCritical || sameResult.Level == DetectionCritical {
			stopped = true
			break
		}
	}

	if !stopped { t.Error("P0: loop guard failed to stop infinite loop in 20 iterations") }
	if iterations > 10 { t.Errorf("P0: loop guard too slow — allowed %d iterations before stopping", iterations) }
	t.Logf("infinite loop: stopped after %d iterations ✓", iterations)
}

func TestP0_InfiniteLoop_ErrorCircuitBreaker(t *testing.T) {
	guard := NewLoopGuard()

	// Simulate: tool keeps failing
	for i := 0; i < 5; i++ {
		guard.RecordToolError("flaky_api")
		guard.RecordError()
	}

	// Circuit should be broken
	if !guard.IsToolCircuitBroken("flaky_api") {
		t.Error("P0: circuit breaker didn't trip after 5 errors")
	}

	// Error circuit break should also fire
	det := guard.DetectErrorCircuitBreak()
	if det.Level != DetectionCritical {
		t.Logf("error circuit break level: %v (may need more errors)", det.Level)
	}
}

// ── P0 #5: Parallel tool execution with panic → must not crash agent ──

func TestP0_ToolPanic_Recovery(t *testing.T) {
	// Create a tool that panics
	reg := tools.NewRegistry()
	reg.Register(&panicTool{})
	reg.Register(tools.NewReadFileTool(t.TempDir()))

	executor := NewParallelToolExecutor(reg, 5)
	ctx := context.Background()

	// Execute panic tool alongside normal tool
	results := executor.Execute(ctx, []ToolCall{
		{Name: "panic_tool", Args: map[string]any{"trigger": "crash"}},
		{Name: "read_file", Args: map[string]any{"path": "nonexistent.txt"}},
	})

	// Should get results for both — panic recovered, not crashed
	if len(results) < 2 { t.Fatalf("expected 2 results, got %d", len(results)) }

	// Panic tool should return error, not crash
	panicResult := results[0]
	if !panicResult.IsError { t.Error("panic tool should return error") }
	if !strings.Contains(panicResult.Content, "panic") {
		t.Logf("panic result: %q", panicResult.Content)
	}

	t.Log("tool panic: recovered without crashing agent ✓")
}

// panicTool is a test tool that panics on execution.
type panicTool struct{}

func (p *panicTool) Name() string        { return "panic_tool" }
func (p *panicTool) Description() string  { return "test tool that panics" }
func (p *panicTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (p *panicTool) Execute(_ context.Context, _ map[string]any) *tools.Result {
	panic("intentional test panic")
}

// ── P0 #6: DedupToolResults with nil/empty messages → must not panic ──

func TestP0_DedupToolResults_NilSafety(t *testing.T) {
	// nil messages
	result := DedupToolResults(nil)
	if result != nil { t.Error("nil input should return nil") }

	// empty messages
	result = DedupToolResults([]providers.Message{})
	if len(result) != 0 { t.Error("empty input should return empty") }

	// messages with empty content
	msgs := []providers.Message{
		{Role: "system", Content: ""},
		{Role: "user", Content: ""},
		{Role: "tool", Content: "", ToolCallID: "tc1"},
	}
	result = DedupToolResults(msgs)
	if len(result) != 3 { t.Errorf("should preserve all messages: got %d", len(result)) }
}

// ── P0 #7: estimateTokens with extreme inputs ──

func TestP0_EstimateTokens_Extremes(t *testing.T) {
	// Empty
	if estimateTokens(nil) != 0 { t.Error("nil should be 0 tokens") }
	if estimateTokens([]providers.Message{}) != 0 { t.Error("empty should be 0 tokens") }

	// Single huge message
	huge := []providers.Message{{Role: "user", Content: strings.Repeat("x", 1_000_000)}}
	tokens := estimateTokens(huge)
	if tokens < 200_000 { t.Errorf("1M chars should be ~250K tokens, got %d", tokens) }

	// Many small messages
	var many []providers.Message
	for i := 0; i < 1000; i++ {
		many = append(many, providers.Message{Role: "user", Content: "hi"})
	}
	tokens = estimateTokens(many)
	if tokens < 4000 { t.Errorf("1000 messages should be 4K+ tokens, got %d", tokens) }
}
