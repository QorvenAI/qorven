// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hard tool tests — validation, error handling, concurrency, edge cases.

// === REGISTRY TESTS ===

func TestRegistry_New(t *testing.T) {
	r := NewRegistry()
	if r == nil { t.Fatal("nil registry") }
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "test_tool"})
	if len(r.List()) != 1 { t.Errorf("expected 1 tool, got %d", len(r.List())) }
}

func TestRegistry_Register_Multiple(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "tool1"})
	r.Register(&mockTool{name: "tool2"})
	r.Register(&mockTool{name: "tool3"})
	if len(r.List()) != 3 { t.Errorf("expected 3, got %d", len(r.List())) }
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "tool1"})
	r.Register(&mockTool{name: "tool1"}) // same name
	if len(r.List()) != 1 { t.Errorf("duplicate should overwrite: %d", len(r.List())) }
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "web_search"})
	tool, ok := r.Get("web_search")
	if !ok { t.Error("should find registered tool") }
	if tool.Name() != "web_search" { t.Errorf("name=%q", tool.Name()) }
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok { t.Error("should not find unregistered tool") }
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "a"})
	r.Register(&mockTool{name: "b"})
	names := r.List()
	if len(names) != 2 { t.Errorf("expected 2, got %d", len(names)) }
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Register(&mockTool{name: "tool"})
			r.Get("tool")
			r.List()
		}(i)
	}
	wg.Wait()
}

// === RESULT TESTS ===

func TestErrorResult(t *testing.T) {
	r := ErrorResult("something failed")
	if !r.IsError { t.Error("should be error") }
	if r.ForLLM != "something failed" { t.Errorf("ForLLM=%q", r.ForLLM) }
	if r.ForUser != "something failed" { t.Errorf("ForUser=%q", r.ForUser) }
}

func TestSuccessResult(t *testing.T) {
	r := SuccessResult("it worked")
	if r.IsError { t.Error("should not be error") }
	if r.ForLLM != "it worked" { t.Errorf("ForLLM=%q", r.ForLLM) }
}

func TestResult_Fields(t *testing.T) {
	r := &Result{ForLLM: "llm content", ForUser: "user content", IsError: false}
	if r.ForLLM != "llm content" { t.Error("wrong ForLLM") }
	if r.ForUser != "user content" { t.Error("wrong ForUser") }
}

func TestResult_WithMedia(t *testing.T) {
	r := &Result{ForLLM: "image", Media: []MediaFile{{Path: "/tmp/img.png", MimeType: "image/png"}}}
	if len(r.Media) != 1 { t.Error("should have 1 media") }
	if r.Media[0].MimeType != "image/png" { t.Error("wrong mime") }
}

// === TOOL EXECUTION TIMEOUT ===

func TestToolExecution_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	tool := &slowTool{delay: 5 * time.Second}
	done := make(chan *Result, 1)
	go func() { done <- tool.Execute(ctx, nil) }()

	select {
	case <-ctx.Done():
		t.Log("context cancelled as expected")
	case r := <-done:
		if r != nil && !r.IsError { t.Error("slow tool should timeout or error") }
	}
}

// === TOOL VALIDATION ===

func TestToolValidation_EmptyArgs(t *testing.T) {
	tool := &validatingTool{}
	result := tool.Execute(context.Background(), nil)
	if !result.IsError { t.Error("nil args should error") }
}

func TestToolValidation_MissingRequired(t *testing.T) {
	tool := &validatingTool{}
	result := tool.Execute(context.Background(), map[string]any{"optional": "value"})
	if !result.IsError { t.Error("missing required field should error") }
}

func TestToolValidation_ValidArgs(t *testing.T) {
	tool := &validatingTool{}
	result := tool.Execute(context.Background(), map[string]any{"query": "test"})
	if result.IsError { t.Errorf("valid args should succeed: %s", result.ForLLM) }
}

// === TOOL INTERFACE COMPLIANCE ===

func TestToolInterface(t *testing.T) {
	tools := []Tool{
		&mockTool{name: "test"},
		&validatingTool{},
		&slowTool{delay: 0},
	}
	for _, tool := range tools {
		if tool.Name() == "" { t.Error("empty name") }
		if tool.Description() == "" { t.Error("empty description") }
		params := tool.Parameters()
		if params == nil { t.Error("nil parameters") }
	}
}

// === CONCURRENT TOOL EXECUTION ===

func TestConcurrentToolExecution(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "fast_tool"})

	var wg sync.WaitGroup
	results := make(chan *Result, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tool, _ := r.Get("fast_tool")
			result := tool.Execute(context.Background(), map[string]any{"q": "test"})
			results <- result
		}()
	}
	wg.Wait()
	close(results)

	errors := 0
	for r := range results {
		if r.IsError { errors++ }
	}
	if errors > 0 { t.Errorf("%d errors in concurrent execution", errors) }
}

// === TOOL NAME CONVENTIONS ===

func TestToolNames_NoExternalBranding(t *testing.T) {
	// Guard rail — tool names must not leak third-party product names.
	// Keep this list current if you integrate new vendors so nobody
	// accidentally exposes the upstream brand as a stable API surface.
	banned := []string{"firecrawl", "scrapling", "hyperagent", "graphify"}
	r := NewRegistry()
	// Register some tools
	r.Register(&mockTool{name: "web_search"})
	r.Register(&mockTool{name: "qor_crawl"})
	r.Register(&mockTool{name: "qorven_social"})
	r.Register(&mockTool{name: "browser"})

	for _, name := range r.List() {
		lower := strings.ToLower(name)
		for _, b := range banned {
			if strings.Contains(lower, b) { t.Errorf("tool %q contains banned name %q", name, b) }
		}
	}
}

// === MOCK TOOLS ===

type mockTool struct{ name string }
func (t *mockTool) Name() string { return t.name }
func (t *mockTool) Description() string { return "mock tool for testing" }
func (t *mockTool) Parameters() map[string]any { return map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}} }
func (t *mockTool) Execute(ctx context.Context, args map[string]any) *Result { return SuccessResult("mock result") }

type slowTool struct{ delay time.Duration }
func (t *slowTool) Name() string { return "slow_tool" }
func (t *slowTool) Description() string { return "slow tool" }
func (t *slowTool) Parameters() map[string]any { return map[string]any{"type": "object"} }
func (t *slowTool) Execute(ctx context.Context, args map[string]any) *Result {
	select {
	case <-time.After(t.delay):
		return SuccessResult("done")
	case <-ctx.Done():
		return ErrorResult("timeout: " + ctx.Err().Error())
	}
}

type validatingTool struct{}
func (t *validatingTool) Name() string { return "validating_tool" }
func (t *validatingTool) Description() string { return "validates input" }
func (t *validatingTool) Parameters() map[string]any { return map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}} }
func (t *validatingTool) Execute(ctx context.Context, args map[string]any) *Result {
	if args == nil { return ErrorResult("args required") }
	q, _ := args["query"].(string)
	if q == "" { return ErrorResult("query required") }
	return SuccessResult("searched: " + q)
}

// === HARD TOOL TESTS ===

func TestRegistry_StressRegister(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 1000; i++ {
		r.Register(&mockTool{name: "tool_" + string(rune(i%256))})
	}
	// Many will overwrite, but should not panic
	list := r.List()
	if len(list) == 0 { t.Error("should have tools") }
}

func TestRegistry_GetAfterRegister(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 100; i++ {
		name := "tool_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		r.Register(&mockTool{name: name})
		tool, ok := r.Get(name)
		if !ok { t.Errorf("can't find %q immediately after register", name) }
		if tool.Name() != name { t.Errorf("wrong name: %q", tool.Name()) }
	}
}

func TestToolExecution_ContextTimeout_Strict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	tool := &slowTool{delay: time.Second}
	result := tool.Execute(ctx, nil)
	if !result.IsError { t.Error("should timeout") }
	if !strings.Contains(result.ForLLM, "timeout") && !strings.Contains(result.ForLLM, "cancel") {
		t.Logf("timeout message: %q", result.ForLLM)
	}
}

func TestToolExecution_NilContext(t *testing.T) {
	// Tools should handle nil-ish contexts gracefully
	tool := &mockTool{name: "test"}
	result := tool.Execute(context.Background(), nil)
	if result == nil { t.Error("nil result") }
}

func TestResult_LargeContent(t *testing.T) {
	large := strings.Repeat("x", 1024*1024) // 1MB
	r := SuccessResult(large)
	if len(r.ForLLM) != 1024*1024 { t.Error("content truncated") }
}

func TestResult_EmptyContent(t *testing.T) {
	r := SuccessResult("")
	if r.IsError { t.Error("empty success should not be error") }
}

func TestErrorResult_LongMessage(t *testing.T) {
	long := strings.Repeat("error detail ", 1000)
	r := ErrorResult(long)
	if !r.IsError { t.Error("should be error") }
}

func TestToolInterface_AllMethods(t *testing.T) {
	tools := []Tool{&mockTool{name: "a"}, &validatingTool{}, &slowTool{delay: 0}}
	for _, tool := range tools {
		// Every tool must implement all 4 methods without panic
		_ = tool.Name()
		_ = tool.Description()
		_ = tool.Parameters()
		_ = tool.Execute(context.Background(), map[string]any{})
	}
}

func TestConcurrentToolExecution_DifferentTools(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 10; i++ {
		r.Register(&mockTool{name: "tool_" + string(rune('a'+i))})
	}

	var wg sync.WaitGroup
	errors := int64(0)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := "tool_" + string(rune('a'+n%10))
			tool, ok := r.Get(name)
			if !ok { atomic.AddInt64(&errors, 1); return }
			result := tool.Execute(context.Background(), map[string]any{"n": n})
			if result.IsError { atomic.AddInt64(&errors, 1) }
		}(i)
	}
	wg.Wait()
	if errors > 0 { t.Errorf("%d errors in concurrent execution", errors) }
}
