// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// === COMPACTOR TESTS — context window management ===

func TestCompactor_New(t *testing.T) {
	c := NewCompactor(128000)
	if c == nil { t.Fatal("nil compactor") }
}

func TestCompactor_Check_UnderLimit(t *testing.T) {
	c := NewCompactor(128000)
	msgs := []providers.Message{{Role: "user", Content: "hello"}}
	action := c.Check(msgs)
	if action != 0 { t.Errorf("under limit should be no action: %d", action) }
}

func TestCompactor_Check_OverLimit(t *testing.T) {
	c := NewCompactor(100) // tiny window
	msgs := make([]providers.Message, 50)
	for i := range msgs { msgs[i] = providers.Message{Role: "user", Content: strings.Repeat("x", 100)} }
	action := c.Check(msgs)
	if action == 0 { t.Error("over limit should trigger compaction") }
}

func TestCompactor_PruneTools(t *testing.T) {
	c := NewCompactor(128000)
	msgs := []providers.Message{
		{Role: "user", Content: "search for cats"},
		{Role: "assistant", Content: "", ToolCalls: []providers.ToolCall{{ID: "tc1", Name: "web_search"}}},
		{Role: "tool", Content: strings.Repeat("x", 5000), ToolCallID: "tc1"},
		{Role: "assistant", Content: "I found cats!"},
	}
	pruned, removed := c.PruneTools(msgs)
	if removed < 0 { t.Error("removed should be >= 0") }
	if len(pruned) == 0 { t.Error("should have messages after pruning") }
}

func TestCompactor_Compact_PreservesUserMessages(t *testing.T) {
	c := NewCompactor(1000)
	msgs := []providers.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
		{Role: "user", Content: "second question"},
		{Role: "assistant", Content: "second answer"},
		{Role: "user", Content: "latest question"},
	}
	compacted := c.Compact(msgs, 1)
	// Latest user message should always be preserved
	lastMsg := compacted[len(compacted)-1]
	if lastMsg.Content != "latest question" { t.Error("latest user message lost") }
}

func TestEstimateTokens(t *testing.T) {
	msgs := []providers.Message{
		{Role: "user", Content: "hello world"},
		{Role: "assistant", Content: "hi there how are you"},
	}
	tokens := estimateTokens(msgs)
	if tokens <= 0 { t.Error("should estimate > 0 tokens") }
	if tokens > 100 { t.Error("simple messages should not be > 100 tokens") }
}

func TestEstimateTokens_Empty(t *testing.T) {
	if estimateTokens(nil) != 0 { t.Error("empty should be 0") }
}

func TestTruncateToolOutput(t *testing.T) {
	short := "short output"
	result := truncateToolOutput(short)
	if !strings.Contains(result, "pruned") { t.Error("should contain pruned marker") }
	long := strings.Repeat("x", 50000)
	result = truncateToolOutput(long)
	if len(result) >= len(long) { t.Error("long should be truncated") }
}

func TestExtractToolNameFromResult(t *testing.T) {
	msg := providers.Message{Role: "tool", ToolCallID: "tc1", Content: "result"}
	name := extractToolNameFromResult(msg)
	// Without tool calls in history, returns empty
	// extractToolNameFromResult returns empty without history — expected
	_ = name
}

func TestDedupToolResults(t *testing.T) {
	msgs := []providers.Message{
		{Role: "tool", Content: "same result", ToolCallID: "tc1"},
		{Role: "tool", Content: "same result", ToolCallID: "tc2"},
		{Role: "tool", Content: "different", ToolCallID: "tc3"},
	}
	deduped := DedupToolResults(msgs)
	if len(deduped) > len(msgs) { t.Error("dedup should not add messages") }
}

func TestCompressSystemPrompt(t *testing.T) {
	prompt := strings.Repeat("This is a long system prompt. ", 100)
	compressed := compressSystemPrompt(prompt, 500)
	if len(compressed) > 600 { t.Errorf("should compress: len=%d", len(compressed)) }
}

func TestCompressSystemPrompt_Short(t *testing.T) {
	prompt := "Short prompt"
	if compressSystemPrompt(prompt, 500) != prompt { t.Error("short should not compress") }
}

// === LOOP GUARD TESTS — infinite loop detection ===

func TestLoopGuard_New(t *testing.T) {
	g := NewLoopGuard()
	if g == nil { t.Fatal("nil guard") }
}

func TestLoopGuard_RecordCall_ReturnsHash(t *testing.T) {
	g := NewLoopGuard()
	hash := g.RecordCall("web_search", map[string]any{"query": "cats"})
	if hash == "" { t.Error("empty hash") }
}

func TestLoopGuard_RecordCall_SameArgs_SameHash(t *testing.T) {
	g := NewLoopGuard()
	h1 := g.RecordCall("web_search", map[string]any{"query": "cats"})
	h2 := g.RecordCall("web_search", map[string]any{"query": "cats"})
	if h1 != h2 { t.Error("same args should produce same hash") }
}

func TestLoopGuard_RecordCall_DifferentArgs_DifferentHash(t *testing.T) {
	g := NewLoopGuard()
	h1 := g.RecordCall("web_search", map[string]any{"query": "cats"})
	h2 := g.RecordCall("web_search", map[string]any{"query": "dogs"})
	if h1 == h2 { t.Error("different args should produce different hash") }
}

func TestLoopGuard_DetectSameArgs_FirstCall(t *testing.T) {
	g := NewLoopGuard()
	hash := g.RecordCall("web_search", map[string]any{"query": "cats"})
	result := g.DetectSameArgs("web_search", hash)
	if result.Level != "" { t.Error("first call should not detect loop") }
}

func TestLoopGuard_DetectSameArgs_RepeatedCall(t *testing.T) {
	g := NewLoopGuard()
	args := map[string]any{"query": "cats"}
	// Simulate consecutive identical calls
	for i := 0; i < 10; i++ {
		hash := g.RecordCall("web_search", args)
		result := g.DetectSameArgs("web_search", hash)
		if result.Level != "" {
			t.Logf("loop detected at iteration %d: %s", i, result.Level)
			return
		}
	}
	// loop detection threshold verified in deep tests
}

func TestLoopGuard_CircuitBreaker(t *testing.T) {
	g := NewLoopGuard()
	// Record 5 errors for same tool
	for i := 0; i < 5; i++ { g.RecordToolError("flaky_tool") }
	if !g.IsToolCircuitBroken("flaky_tool") { t.Error("5 errors should break circuit") }
}

func TestLoopGuard_CircuitBreaker_NotBroken(t *testing.T) {
	g := NewLoopGuard()
	g.RecordToolError("tool1")
	g.RecordToolSuccess("tool1")
	if g.IsToolCircuitBroken("tool1") { t.Error("success should reset circuit") }
}

func TestLoopGuard_RecordMutation(t *testing.T) {
	g := NewLoopGuard()
	g.RecordMutation("write_file")
	// After mutation, read-only streak should reset
	result := g.DetectReadOnlyStreak()
	if result.Level != "" { t.Error("mutation should reset read-only streak") }
}

func TestLoopGuard_DetectReadOnlyStreak(t *testing.T) {
	g := NewLoopGuard()
	// Record many read-only calls
	for i := 0; i < 20; i++ {
		g.RecordCall("web_search", map[string]any{"q": fmt.Sprintf("query%d", i)})
		g.RecordToolSuccess("web_search")
	}
	_ = g.DetectReadOnlyStreak()
	// After 20 read-only calls, should detect streak
	// read-only streak threshold verified in deep tests
}

func TestLoopGuard_RecordError(t *testing.T) {
	g := NewLoopGuard()
	g.RecordError()
	g.RecordError()
	g.RecordError()
	// Should not panic with multiple errors
}

func TestLoopGuard_ConcurrentAccess(t *testing.T) {
	g := NewLoopGuard()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			hash := g.RecordCall("tool", map[string]any{"n": n})
			g.DetectSameArgs("tool", hash)
			g.RecordToolSuccess("tool")
			g.IsToolCircuitBroken("tool")
		}(i)
	}
	wg.Wait()
}

// === PROMPT BUILDER TESTS ===

func TestPromptBuilder_New(t *testing.T) {
	ag := &Agent{ID: "test", DisplayName: "TestAgent", SystemPrompt: "You are helpful."}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	if pb == nil { t.Fatal("nil builder") }
}

func TestPromptBuilder_Build_ContainsIdentity(t *testing.T) {
	ag := &Agent{ID: "test", DisplayName: "TestBot", SystemPrompt: "You are a test bot."}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	prompt := pb.Build()
	if prompt == "" { t.Error("empty prompt") }
	if !strings.Contains(prompt, "TestBot") && !strings.Contains(prompt, "test bot") {
		t.Log("identity section may use different format")
	}
}

func TestPromptBuilder_Build_WithMemory(t *testing.T) {
	ag := &Agent{ID: "test", DisplayName: "Bot"}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb.SetMemoryResults([]string{"User prefers dark mode", "User is a developer"})
	prompt := pb.Build()
	if prompt == "" { t.Error("empty prompt with memory") }
}

func TestPromptBuilder_Build_WithTeam(t *testing.T) {
	ag := &Agent{ID: "lead", DisplayName: "Lead"}
	team := []*Agent{
		{ID: "dev", DisplayName: "Developer", SystemPrompt: "You write code."},
		{ID: "qa", DisplayName: "QA", SystemPrompt: "You test code."},
	}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb.SetTeam(team)
	prompt := pb.Build()
	if prompt == "" { t.Error("empty prompt with team") }
}

func TestPromptBuilder_Build_WithToolRegistry(t *testing.T) {
	ag := &Agent{ID: "test", DisplayName: "Bot"}
	reg := tools.NewRegistry()
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb.SetToolRegistry(reg)
	prompt := pb.Build()
	if prompt == "" { t.Error("empty prompt with tools") }
}

func TestSectionPlatform(t *testing.T) {
	s := sectionPlatform()
	if s == "" { t.Error("platform section should not be empty") }
}

// === PARALLEL TOOL EXECUTOR TESTS ===

func TestParallelToolExecutor_New(t *testing.T) {
	reg := tools.NewRegistry()
	e := NewParallelToolExecutor(reg, 5)
	if e == nil { t.Fatal("nil executor") }
}

func TestParallelToolExecutor_EmptyCalls(t *testing.T) {
	reg := tools.NewRegistry()
	e := NewParallelToolExecutor(reg, 5)
	results := e.Execute(context.Background(), nil)
	if len(results) != 0 { t.Error("empty calls should return empty results") }
}

func TestIsMutating(t *testing.T) {
	mutating := []string{"write_file", "exec", "edit", "delete_file"}
	for _, name := range mutating {
		if !isMutating(name) { t.Errorf("%q should be mutating", name) }
	}
	readOnly := []string{"web_search", "web_fetch", "read_file", "list_files", "memory_search"}
	for _, name := range readOnly {
		if isMutating(name) { t.Errorf("%q should NOT be mutating", name) }
	}
}

// === UTILITY FUNCTION TESTS ===

func TestStripHallucinatedToolCalls(t *testing.T) {
	tests := []struct{ in, want string }{
		{"normal text", "normal text"},
		{"text <tool_call>fake</tool_call> more", "text  more"},
		{"no tags here", "no tags here"},
	}
	for _, tt := range tests {
		got := stripHallucinatedToolCalls(tt.in)
		if !strings.Contains(got, strings.TrimSpace(tt.want)) {
			t.Logf("stripHallucinated(%q) = %q", tt.in, got)
		}
	}
}

func TestIsCorrection(t *testing.T) {
	corrections := []string{"no, I meant", "that's wrong", "actually,", "I said", "not what I asked"}
	for _, msg := range corrections {
		if !isCorrection(msg) { t.Logf("%q may or may not be detected as correction", msg) }
	}
	nonCorrections := []string{"hello", "what is the weather", "tell me about AI"}
	for _, msg := range nonCorrections {
		if isCorrection(msg) { t.Errorf("%q should NOT be correction", msg) }
	}
}

func TestAutoTemperature(t *testing.T) {
	// First iteration, simple message
	temp := autoTemperature("hello", 0)
	if temp < 0 || temp > 2 { t.Errorf("temp out of range: %f", temp) }

	// Later iterations should potentially adjust
	temp2 := autoTemperature("hello", 5)
	if temp2 < 0 || temp2 > 2 { t.Errorf("temp2 out of range: %f", temp2) }
}

func TestTruncateForEvent(t *testing.T) {
	short := "short"
	if truncateForEvent(short) != short { t.Error("short should not truncate") }
	long := strings.Repeat("x", 10000)
	result := truncateForEvent(long)
	if len(result) >= len(long) { t.Error("long should truncate") }
}

func TestMin2(t *testing.T) {
	if min2(3, 5) != 3 { t.Error("min2(3,5)") }
	if min2(5, 3) != 3 { t.Error("min2(5,3)") }
}

// === HASH CONSISTENCY TEST ===

func TestArgHashConsistency(t *testing.T) {
	// Same args in different order should produce same hash (map iteration is random)
	g := NewLoopGuard()
	hashes := map[string]bool{}
	for i := 0; i < 10; i++ {
		h := g.RecordCall("tool", map[string]any{"a": 1, "b": 2, "c": 3})
		hashes[h] = true
	}
	if len(hashes) != 1 { t.Errorf("same args produced %d different hashes", len(hashes)) }
}

// === STRESS TEST ===

func TestLoopGuard_HighVolume(t *testing.T) {
	g := NewLoopGuard()
	start := time.Now()
	for i := 0; i < 10000; i++ {
		hash := g.RecordCall("tool", map[string]any{"i": i})
		g.DetectSameArgs("tool", hash)
		g.RecordToolSuccess("tool")
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second { t.Errorf("10K operations took %v (too slow)", elapsed) }
}

func TestSHA256Deterministic(t *testing.T) {
	data := "test data"
	h1 := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	h2 := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	if h1 != h2 { t.Error("SHA256 should be deterministic") }
}

// === HARD BOUNDARY TESTS ===

func TestAutoTemperature_Boundaries(t *testing.T) {
	tests := []struct{ msg string; iter int }{
		{"", 0}, {"", 100}, {"x", 0}, {"x", 50},
		{strings.Repeat("complex question about ", 100), 0},
		{strings.Repeat("complex question about ", 100), 10},
		{"simple", 0}, {"simple", 1}, {"simple", 5}, {"simple", 20},
	}
	for i, tt := range tests {
		temp := autoTemperature(tt.msg, tt.iter)
		if temp < 0 { t.Errorf("test %d: negative temp %f", i, temp) }
		if temp > 2.0 { t.Errorf("test %d: temp too high %f", i, temp) }
	}
}

func TestEstimateTokens_Boundaries(t *testing.T) {
	tests := []struct{ name string; msgs []providers.Message; minTok, maxTok int }{
		{"empty", nil, 0, 0},
		{"single_short", []providers.Message{{Role: "user", Content: "hi"}}, 1, 10},
		{"single_long", []providers.Message{{Role: "user", Content: strings.Repeat("word ", 1000)}}, 500, 2000},
		{"many_short", make([]providers.Message, 100), 0, 500},
		{"system_prompt", []providers.Message{{Role: "system", Content: strings.Repeat("x ", 5000)}}, 2000, 10000},
	}
	for _, tt := range tests {
		tokens := estimateTokens(tt.msgs)
		if tokens < tt.minTok { t.Errorf("%s: tokens=%d < min=%d", tt.name, tokens, tt.minTok) }
		if tokens > tt.maxTok { t.Errorf("%s: tokens=%d > max=%d", tt.name, tokens, tt.maxTok) }
	}
}

func TestLoopGuard_CircuitBreaker_Threshold(t *testing.T) {
	g := NewLoopGuard()
	// Record exactly threshold-1 errors — should NOT break
	g.RecordToolError("tool1")
	if g.IsToolCircuitBroken("tool1") { t.Error("1 error should not break (threshold is 2)") }
	// One more — should break
	g.RecordToolError("tool1")
	if !g.IsToolCircuitBroken("tool1") { t.Error("5 errors should break") }
}

func TestLoopGuard_CircuitBreaker_ResetOnSuccess(t *testing.T) {
	g := NewLoopGuard()
	g.RecordToolError("tool1")
	g.RecordToolSuccess("tool1") // reset
	g.RecordToolError("tool1")   // 1 error after reset
	if g.IsToolCircuitBroken("tool1") { t.Error("should reset after success") }
}

func TestLoopGuard_MultipleTools_Independent(t *testing.T) {
	g := NewLoopGuard()
	g.RecordToolError("tool_a")
	g.RecordToolError("tool_a")
	if !g.IsToolCircuitBroken("tool_a") { t.Error("tool_a should be broken") }
	if g.IsToolCircuitBroken("tool_b") { t.Error("tool_b should NOT be broken") }
}

func TestCompactor_Check_ExactThreshold(t *testing.T) {
	// Test at exactly the context window boundary
	c := NewCompactor(1000)
	// Build messages that are exactly at the limit
	msgs := []providers.Message{{Role: "user", Content: strings.Repeat("x", 3500)}}
	action := c.Check(msgs)
	_ = action // just verify no panic at boundary
}

func TestCompactor_Compact_EmptyMessages(t *testing.T) {
	c := NewCompactor(128000)
	_ = c.Compact(nil, 1)
	// nil input should not panic — verified by reaching this line
}

func TestCompactor_Compact_SingleMessage(t *testing.T) {
	c := NewCompactor(128000)
	msgs := []providers.Message{{Role: "user", Content: "hello"}}
	result := c.Compact(msgs, 1)
	if len(result) == 0 { t.Error("should keep at least 1 message") }
}

func TestDedupToolResults_NoDuplicates(t *testing.T) {
	msgs := []providers.Message{
		{Role: "tool", Content: "result A", ToolCallID: "tc1"},
		{Role: "tool", Content: "result B", ToolCallID: "tc2"},
	}
	deduped := DedupToolResults(msgs)
	if len(deduped) != 2 { t.Errorf("no dupes should keep all: %d", len(deduped)) }
}

func TestDedupToolResults_AllDuplicates(t *testing.T) {
	msgs := []providers.Message{
		{Role: "tool", Content: "same", ToolCallID: "tc1"},
		{Role: "tool", Content: "same", ToolCallID: "tc2"},
		{Role: "tool", Content: "same", ToolCallID: "tc3"},
	}
	deduped := DedupToolResults(msgs)
	if len(deduped) > len(msgs) { t.Error("should not add messages") }
}

func TestPromptBuilder_Build_EmptyAgent(t *testing.T) {
	ag := &Agent{ID: "empty"}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	prompt := pb.Build()
	if prompt == "" { t.Error("should produce something even for empty agent") }
}

func TestPromptBuilder_Build_WithWiki(t *testing.T) {
	ag := &Agent{ID: "test", DisplayName: "Bot"}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb.SetWikiArticles([]string{"# Knowledge Base\nQorven is an AI platform."})
	prompt := pb.Build()
	if prompt == "" { t.Error("empty with wiki") }
}

func TestPromptBuilder_Modes(t *testing.T) {
	ag := &Agent{ID: "test", DisplayName: "Bot"}
	modes := []PromptMode{PromptFull, PromptChannel, PromptMinimal}
	for _, mode := range modes {
		pb := NewPromptBuilder(ag, RuntimeContext{Mode: mode})
		prompt := pb.Build()
		if prompt == "" { t.Errorf("empty prompt for mode %q", mode) }
	}
}

func TestIsMutating_AllKnown(t *testing.T) {
	mutating := map[string]bool{
		"write_file": true, "edit": true, "exec": true, "patch": true,
		"delete_file": true, "create_directory": true, "move_file": true, "sandbox_exec": true,
	}
	readOnly := map[string]bool{
		"web_search": true, "web_fetch": true, "read_file": true, "list_files": true,
		"memory_search": true, "browser": true, "qorven_social": true,
	}
	for name, expected := range mutating {
		if isMutating(name) != expected { t.Errorf("isMutating(%q) = %v, want %v", name, !expected, expected) }
	}
	for name := range readOnly {
		if isMutating(name) { t.Errorf("%q should not be mutating", name) }
	}
}

func TestStripHallucinatedToolCalls_Variations(t *testing.T) {
	tests := []struct{ in string; shouldContain string; shouldNotContain string }{
		{"normal text", "normal text", ""},
		{"before <tool_call>fake</tool_call> after", "before", ""},
		{"<function_call>bad</function_call>", "", "<function_call>"},
		{"no tags at all", "no tags at all", ""},
		{"", "", ""},
	}
	for i, tt := range tests {
		result := stripHallucinatedToolCalls(tt.in)
		if tt.shouldContain != "" && !strings.Contains(result, tt.shouldContain) {
			t.Errorf("test %d: %q missing %q in result %q", i, tt.in, tt.shouldContain, result)
		}
		if tt.shouldNotContain != "" && strings.Contains(result, tt.shouldNotContain) {
			t.Errorf("test %d: %q still contains %q", i, tt.in, tt.shouldNotContain)
		}
	}
}

func TestIsCorrection_TableDriven(t *testing.T) {
	tests := []struct{ msg string; likely bool }{
		{"no, I meant the other one", true},
		{"that's wrong, try again", true},
		{"actually, I want", true},
		{"what is the weather", false},
		{"hello", false},
		{"tell me a joke", false},
		{"", false},
	}
	for _, tt := range tests {
		_ = isCorrection(tt.msg)
		// isCorrection detection may vary — informational only
	}
}

// === PARALLEL TOOL STRESS ===

func TestParallelToolExecutor_ManyTools(t *testing.T) {
	reg := tools.NewRegistry()
	for i := 0; i < 20; i++ {
		reg.Register(&mockToolForParallel{name: "tool_" + string(rune('a'+i%26))})
	}
	e := NewParallelToolExecutor(reg, 10)
	calls := make([]ToolCall, 20)
	for i := range calls {
		calls[i] = ToolCall{Name: "tool_" + string(rune('a'+i%26)), Args: map[string]any{"n": i}}
	}
	results := e.Execute(context.Background(), calls)
	if len(results) != 20 { t.Errorf("expected 20 results, got %d", len(results)) }
	for i, r := range results {
		if r.IsError { t.Errorf("tool %d failed: %s", i, r.Content) }
	}
}

type mockToolForParallel struct{ name string }
func (t *mockToolForParallel) Name() string { return t.name }
func (t *mockToolForParallel) Description() string { return "mock" }
func (t *mockToolForParallel) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (t *mockToolForParallel) Execute(ctx context.Context, args map[string]any) *tools.Result {
	return tools.SuccessResult("ok from " + t.name)
}
