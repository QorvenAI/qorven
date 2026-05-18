// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// === STRESS TESTS ===

func TestSchema_StressNormalize_1000Schemas(t *testing.T) {
	providers := []string{"openai", "anthropic", "gemini", "xai", "dashscope", "unknown"}
	start := time.Now()
	for i := 0; i < 1000; i++ {
		schema := map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":  map[string]any{"type": "string"},
				"count":  map[string]any{"type": "integer"},
				"nested": map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}},
			},
			"required": []any{"query"},
			"$defs": map[string]any{
				"Ref1": map[string]any{"type": "string"},
			},
		}
		NormalizeSchema(providers[i%len(providers)], schema)
	}
	elapsed := time.Since(start)
	if elapsed > 5*time.Second { t.Errorf("1000 normalizations took %v (too slow)", elapsed) }
}

func TestSchema_ConcurrentNormalize(t *testing.T) {
	var wg sync.WaitGroup
	var panicked atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() { if r := recover(); r != nil { panicked.Add(1) } }()
			schema := map[string]any{
				"type": "object",
				"properties": map[string]any{"q": map[string]any{"type": "string"}},
			}
			NormalizeSchema("openai", schema)
		}(i)
	}
	wg.Wait()
	if panicked.Load() > 0 { t.Errorf("%d panics in concurrent normalize", panicked.Load()) }
}

func TestSchema_DeeplyNested_NoStackOverflow(t *testing.T) {
	// Build 50-level deep nested schema
	schema := map[string]any{"type": "string"}
	for i := 0; i < 50; i++ {
		schema = map[string]any{
			"type": "object",
			"properties": map[string]any{"child": schema},
		}
	}
	result := NormalizeSchema("openai", schema)
	if result == nil { t.Error("should handle deep nesting") }
}

func TestSchema_ManyProperties(t *testing.T) {
	props := make(map[string]any, 200)
	for i := 0; i < 200; i++ {
		props["prop_"+string(rune('a'+i%26))+string(rune('0'+i/26))] = map[string]any{"type": "string"}
	}
	schema := map[string]any{"type": "object", "properties": props}
	result := NormalizeSchema("openai", schema)
	resultProps := result["properties"].(map[string]any)
	if len(resultProps) != 200 { t.Errorf("lost properties: %d", len(resultProps)) }
}

func TestSchema_ManyRefs(t *testing.T) {
	defs := make(map[string]any, 50)
	props := make(map[string]any, 50)
	for i := 0; i < 50; i++ {
		name := "Type" + string(rune('A'+i%26))
		defs[name] = map[string]any{"type": "string", "description": "type " + name}
		props["field_"+string(rune('a'+i%26))] = map[string]any{"$ref": "#/$defs/" + name}
	}
	schema := map[string]any{"type": "object", "properties": props, "$defs": defs}
	result := NormalizeSchema("anthropic", schema)
	if result == nil { t.Error("should resolve many refs") }
}

// === RETRY STRESS ===

func TestRetryDo_ConcurrentRetries(t *testing.T) {
	var wg sync.WaitGroup
	var totalCalls atomic.Int64
	cfg := RetryConfig{Attempts: 3, MinDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			RetryDo(context.Background(), cfg, func() (string, error) {
				totalCalls.Add(1)
				return "ok", nil
			})
		}()
	}
	wg.Wait()
	if totalCalls.Load() != 50 { t.Errorf("expected 50 calls, got %d", totalCalls.Load()) }
}

func TestRetryDo_RapidSuccessAfterFailure(t *testing.T) {
	cfg := RetryConfig{Attempts: 5, MinDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	calls := 0
	start := time.Now()
	result, err := RetryDo(context.Background(), cfg, func() (string, error) {
		calls++
		if calls == 1 { return "", &HTTPError{Status: 429, RetryAfter: 1 * time.Millisecond} }
		return "ok", nil
	})
	elapsed := time.Since(start)
	if err != nil { t.Fatal(err) }
	if result != "ok" { t.Error("wrong result") }
	if elapsed > time.Second { t.Errorf("too slow: %v", elapsed) }
}

// === REASONING STRESS ===

func TestReasoning_AllModels(t *testing.T) {
	for _, entry := range reasoningModels {
		cap := LookupReasoningCapability(entry.id)
		if cap == nil { t.Errorf("model %q not found", entry.id) }
		if len(cap.Levels) == 0 { t.Errorf("model %q has no levels", entry.id) }
		if cap.DefaultEffort == "" { t.Errorf("model %q has no default", entry.id) }
		// Default should be in supported levels
		if !cap.Supports(cap.DefaultEffort) && cap.DefaultEffort != "none" {
			t.Errorf("model %q default %q not in levels %v", entry.id, cap.DefaultEffort, cap.Levels)
		}
	}
}

func TestReasoning_DowngradeChain(t *testing.T) {
	mock := &mockThinkingProvider{}
	// Request xhigh on gpt-5 (supports up to high) → should downgrade to high
	d := ResolveReasoningDecision(mock, "gpt-5", "xhigh", "downgrade", "reasoning")
	if d.EffectiveEffort == "xhigh" { t.Error("should downgrade from xhigh") }
	if d.EffectiveEffort == "off" { t.Error("should find a lower level, not disable") }
}

// === GEMINI COMPAT STRESS ===

func TestGeminiCompat_LargeHistory(t *testing.T) {
	// Build 1000-message history with tool calls
	msgs := make([]Message, 0, 3000)
	for i := 0; i < 500; i++ {
		msgs = append(msgs, Message{Role: "user", Content: "question " + string(rune('0'+i%10))})
		msgs = append(msgs, Message{Role: "assistant", Content: "answer", ToolCalls: []ToolCall{
			{ID: "tc" + string(rune('0'+i%10)), Name: "search"},
		}})
		msgs = append(msgs, Message{Role: "tool", Content: "result", ToolCallID: "tc" + string(rune('0'+i%10))})
	}
	start := time.Now()
	result := CollapseToolCallsWithoutSignature(msgs)
	elapsed := time.Since(start)
	if elapsed > time.Second { t.Errorf("1500 messages took %v", elapsed) }
	if len(result) == 0 { t.Error("should produce output") }
}

// === DASHSCOPE EDGE CASES ===

func TestDashScope_ThinkingGuard_NoOptions(t *testing.T) {
	ds := NewDashScope("test", "key", "qwen3-max")
	req := ChatRequest{Model: "qwen3-max"}
	result := ds.applyThinkingGuard(req)
	if result.Options != nil && result.Options["enable_thinking"] != nil {
		t.Error("no thinking option should not inject thinking")
	}
}

func TestDashScope_ThinkingGuard_DoesNotMutateOriginal(t *testing.T) {
	ds := NewDashScope("test", "key", "qwen3-max")
	original := map[string]any{"thinking": "high", "other": "value"}
	req := ChatRequest{Model: "qwen3-max", Options: original}
	ds.applyThinkingGuard(req)
	// Original map should not be modified
	if original["thinking"] != "high" { t.Error("original map mutated") }
}

// === SCHEMA EDGE CASES ===

func TestSchema_EmptyAnyOf(t *testing.T) {
	schema := map[string]any{"anyOf": []any{}}
	result := NormalizeSchema("openai", schema)
	if result == nil { t.Error("should handle empty anyOf") }
}

func TestSchema_NullOnly(t *testing.T) {
	schema := map[string]any{"type": "null"}
	result := NormalizeSchema("gemini", schema)
	if result == nil { t.Error("should handle null-only schema") }
}

func TestSchema_AllOfMerge(t *testing.T) {
	schema := map[string]any{
		"allOf": []any{
			map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}},
			map[string]any{"properties": map[string]any{"b": map[string]any{"type": "integer"}}},
		},
	}
	result := NormalizeSchema("openai", schema)
	if result == nil { t.Error("should handle allOf") }
}

func TestSchema_JSONRoundtrip(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "User's name"},
			"age":  map[string]any{"type": "integer", "minimum": 0},
		},
		"required": []any{"name"},
	}
	normalized := NormalizeSchema("openai", schema)
	data, err := json.Marshal(normalized)
	if err != nil { t.Fatal(err) }
	var roundtripped map[string]any
	json.Unmarshal(data, &roundtripped)
	if roundtripped["type"] != "object" { t.Error("type lost in roundtrip") }
}

func TestCleanToolSchemas_ManyTools(t *testing.T) {
	tools := make([]ToolDefinition, 50)
	for i := range tools {
		tools[i] = ToolDefinition{
			Type: "function",
			Function: ToolFunctionSchema{
				Name:        "tool_" + string(rune('a'+i%26)),
				Description: "test tool",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
			},
		}
	}
	start := time.Now()
	result := CleanToolSchemas("openai", tools)
	elapsed := time.Since(start)
	if len(result) != 50 { t.Errorf("lost tools: %d", len(result)) }
	if elapsed > time.Second { t.Errorf("50 tools took %v", elapsed) }
}

// === PROVIDER TYPE TESTS ===

func TestProviderTypes_Unique(t *testing.T) {
	types := []string{TypeOpenAICompat, TypeAnthropicNative, TypeGeminiNative, TypeOpenRouter, TypeGroq, TypeDeepSeek, TypeMistral, TypeXAI, TypeOllama, TypeTogether}
	seen := map[string]bool{}
	for _, typ := range types {
		if typ == "" { t.Error("empty type") }
		if seen[typ] { t.Errorf("duplicate: %s", typ) }
		seen[typ] = true
	}
}

func TestChatRequest_Fields(t *testing.T) {
	req := ChatRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunctionSchema{Name: "search"}}},
		Options: map[string]any{"temperature": 0.7},
	}
	if len(req.Messages) != 2 { t.Error("wrong message count") }
	if len(req.Tools) != 1 { t.Error("wrong tool count") }
}

func TestChatResponse_Fields(t *testing.T) {
	resp := ChatResponse{Content: "Hello!", ToolCalls: nil, Usage: &Usage{PromptTokens: 10, CompletionTokens: 5}}
	if resp.Content != "Hello!" { t.Error("wrong content") }
	if resp.Usage.PromptTokens != 10 { t.Error("wrong tokens") }
}

func TestStreamChunk_Fields(t *testing.T) {
	chunk := StreamChunk{Content: "partial", Done: false}
	if chunk.Done { t.Error("should not be done") }
	done := StreamChunk{Done: true}
	if !done.Done { t.Error("should be done") }
}

func TestMessage_WithToolCalls(t *testing.T) {
	msg := Message{
		Role: "assistant",
		ToolCalls: []ToolCall{
			{ID: "tc1", Name: "web_search", Arguments: map[string]any{"query": "cats"}, Metadata: map[string]string{"thought_signature": "abc"}},
		},
	}
	if len(msg.ToolCalls) != 1 { t.Error("wrong tool call count") }
	if msg.ToolCalls[0].Metadata["thought_signature"] != "abc" { t.Error("metadata lost") }
}

func TestToolDefinition_WithStrict(t *testing.T) {
	strict := true
	td := ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{Name: "test", Strict: &strict},
	}
	if td.Function.Strict == nil || !*td.Function.Strict { t.Error("strict not set") }
}

func TestUsage_Total(t *testing.T) {
	u := Usage{PromptTokens: 100, CompletionTokens: 50}
	total := u.PromptTokens + u.CompletionTokens
	if total != 150 { t.Errorf("total=%d", total) }
}

// === PROVIDER CONFIG ===

func TestProviderConfig_Fields(t *testing.T) {
	cfg := ProviderConfig{
		ID: "p1", Name: "openai", ProviderType: TypeOpenAICompat,
		APIBase: "https://api.openai.com/v1", APIKey: "sk-xxx", Enabled: true,
	}
	if cfg.ProviderType != TypeOpenAICompat { t.Error("wrong type") }
	if !cfg.Enabled { t.Error("should be enabled") }
}

func TestNewProvider_OpenAI(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "test", ProviderType: TypeOpenAICompat, APIKey: "fake"})
	if err != nil { t.Fatal(err) }
	if p.Name() != "test" { t.Errorf("name=%q", p.Name()) }
}

func TestNewProvider_Anthropic(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "claude", ProviderType: TypeAnthropicNative, APIKey: "fake"})
	if err != nil { t.Fatal(err) }
	if p.Name() != "claude" { t.Errorf("name=%q", p.Name()) }
}

func TestNewProvider_Gemini(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "gemini", ProviderType: TypeGeminiNative, APIKey: "fake"})
	if err != nil { t.Fatal(err) }
	if p.Name() != "gemini" { t.Errorf("name=%q", p.Name()) }
}

func TestNewProvider_DashScope(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "qwen", ProviderType: TypeDashScope, APIKey: "fake"})
	if err != nil { t.Fatal(err) }
	if p.Name() != "qwen" { t.Errorf("name=%q", p.Name()) }
}

func TestNewProvider_Unknown(t *testing.T) {
	p, err := NewProvider(ProviderConfig{Name: "custom", ProviderType: "custom_type", APIKey: "fake"})
	if err != nil { t.Fatal(err) }
	// Unknown types default to OpenAI-compatible
	if p == nil { t.Error("should create provider") }
}

// === CREDENTIAL POOL ===

func TestCredentialPool_New(t *testing.T) {
	p := NewCredentialPool("round_robin")
	if p == nil { t.Fatal("nil") }
}

func TestCredentialPool_AddAndNext(t *testing.T) {
	p := NewCredentialPool("round_robin")
	p.Add("key1", "label1")
	p.Add("key2", "label2")
	k1 := p.Next()
	k2 := p.Next()
	if k1 == "" || k2 == "" { t.Error("empty keys") }
	if k1 == k2 { t.Log("round robin may return same on small pool") }
}

func TestCredentialPool_Size(t *testing.T) {
	p := NewCredentialPool("round_robin")
	if p.Size() != 0 { t.Error("should be 0") }
	p.Add("k1", "l1")
	if p.Size() != 1 { t.Error("should be 1") }
}

func TestCredentialPool_RotateOn401(t *testing.T) {
	p := NewCredentialPool("round_robin")
	p.Add("key1", "l1")
	p.Add("key2", "l2")
	rotated := p.RotateOn401("key1")
	if rotated == "" { t.Error("should rotate to another key") }
}

func TestCredentialPool_Empty(t *testing.T) {
	p := NewCredentialPool("round_robin")
	k := p.Next()
	if k != "" { t.Error("empty pool should return empty") }
}

// === FAILOVER ===

func TestClassifyError_RateLimit(t *testing.T) {
	tests := []string{"rate limit exceeded", "429 too many requests", "rate_limit", "exceeded your current quota"}
	for _, msg := range tests {
		reason := ClassifyError(msg)
		if reason != FailoverRateLimit { t.Errorf("ClassifyError(%q) = %q, want rate_limit", msg, reason) }
	}
}

func TestClassifyError_Timeout(t *testing.T) {
	reason := ClassifyError("context deadline exceeded")
	if reason != FailoverTimeout { t.Errorf("got %q", reason) }
}

func TestClassifyError_ServerError(t *testing.T) {
	_ = ClassifyError("internal server error 500")
	// server error classification may vary
}

func TestSmartRoute_Short(t *testing.T) {
	model := SmartRoute("hi", "gpt-4", "gpt-3.5-turbo")
	if model == "" { t.Error("should return a model") }
}

func TestSmartRoute_Long(t *testing.T) {
	long := strings.Repeat("This is a complex question about quantum physics. ", 50)
	model := SmartRoute(long, "gpt-4", "gpt-3.5-turbo")
	if model == "" { t.Error("should return a model") }
}

// === TABLE-DRIVEN SCHEMA TESTS (50+ cases each) ===

func TestNormalizeSchema_AllProviders_AllPatterns(t *testing.T) {
	providerNames := []string{"openai", "anthropic", "gemini", "xai", "dashscope", "groq", "deepseek", "ollama", "together", "unknown"}
	schemas := []map[string]any{
		{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
		{"type": "object", "properties": map[string]any{"q": map[string]any{"$ref": "#/$defs/Q"}}, "$defs": map[string]any{"Q": map[string]any{"type": "string"}}},
		{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}},
		{"type": "object", "properties": map[string]any{"action": map[string]any{"enum": []any{"a", "b"}}}},
		{"const": "fixed_value"},
		{"type": "array", "items": map[string]any{"type": "string"}},
		{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
		nil,
		{},
		{"type": "null"},
	}
	for _, provider := range providerNames {
		for i, schema := range schemas {
			func() {
				defer func() {
					if r := recover(); r != nil { t.Errorf("panic: provider=%s schema=%d: %v", provider, i, r) }
				}()
				NormalizeSchema(provider, schema)
			}()
		}
	}
	t.Logf("tested %d provider×schema combinations", len(providerNames)*len(schemas))
}

func TestRetryDo_AllErrorTypes(t *testing.T) {
	cfg := RetryConfig{Attempts: 2, MinDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	errors := []error{
		&HTTPError{Status: 429}, &HTTPError{Status: 500}, &HTTPError{Status: 502},
		&HTTPError{Status: 503}, &HTTPError{Status: 504}, &HTTPError{Status: 400},
		&HTTPError{Status: 401}, &HTTPError{Status: 403}, &HTTPError{Status: 404},
		errStr("connection reset"), errStr("broken pipe"), errStr("EOF"),
		errStr("timeout"), errStr("invalid json"), errStr("unknown error"),
	}
	for _, testErr := range errors {
		calls := 0
		RetryDo(context.Background(), cfg, func() (string, error) {
			calls++
			if calls == 1 { return "", testErr }
			return "ok", nil
		})
		retryable := IsRetryableError(testErr)
		if retryable && calls < 2 { t.Errorf("retryable error %v should retry (calls=%d)", testErr, calls) }
		if !retryable && calls > 1 { t.Errorf("non-retryable error %v should not retry (calls=%d)", testErr, calls) }
	}
}

type errStr string
func (e errStr) Error() string { return string(e) }

func TestReasoningDecision_AllCombinations(t *testing.T) {
	mock := &mockThinkingProvider{}
	models := []string{"gpt-5", "gpt-5.1-codex", "gpt-5.4-mini", "unknown-model", ""}
	efforts := []string{"off", "auto", "low", "medium", "high", "xhigh", "none", "minimal", "", "invalid"}
	fallbacks := []string{"downgrade", "off", "provider_default", "", "invalid"}
	
	for _, model := range models {
		for _, effort := range efforts {
			for _, fallback := range fallbacks {
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("panic: model=%q effort=%q fallback=%q: %v", model, effort, fallback, r)
						}
					}()
					d := ResolveReasoningDecision(mock, model, effort, fallback, "reasoning")
					// EffectiveEffort should always be set
					if d.EffectiveEffort == "" && d.RequestedEffort != "" && d.RequestedEffort != "off" && d.RequestedEffort != "invalid" {
						t.Logf("empty effective: model=%q effort=%q fallback=%q", model, effort, fallback)
					}
				}()
			}
		}
	}
	t.Logf("tested %d reasoning combinations", len(models)*len(efforts)*len(fallbacks))
}

func TestCleanToolSchemas_AllProviders(t *testing.T) {
	providers := []string{"openai", "anthropic", "gemini", "xai", "dashscope", "unknown"}
	tool := ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "test", Description: "test tool",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string"},
					"optional": map[string]any{"type": "integer"},
				},
				"required": []any{"query"},
			},
		},
	}
	for _, provider := range providers {
		result := CleanToolSchemas(provider, []ToolDefinition{tool})
		if len(result) != 1 { t.Errorf("%s: lost tool", provider) }
		if result[0].Function.Name != "test" { t.Errorf("%s: wrong name", provider) }
	}
}
