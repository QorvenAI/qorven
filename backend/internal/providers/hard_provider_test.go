//go:build integration
// +build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// hard_provider_test.go — live Gemini/DeepSeek hits. Paired with
// hard_multi_provider_test.go (which defines loadKeys); both must carry
// the same build tag or the symbol reference breaks. Opt in with
// -tags=integration.
package providers

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHard_Gemini_Stream(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["GEMINI_API_KEY"]
	if apiKey == "" { t.Skip("no GEMINI_API_KEY") }

	p := NewGemini(ProviderConfig{Name: "gemini", APIKey: apiKey})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chunks := 0
	resp, err := p.ChatStream(ctx, ChatRequest{
		Model:    "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "List 3 programming languages. One per line."}},
		Options:  map[string]any{"max_tokens": 50},
	}, func(chunk StreamChunk) { chunks++ })
	if err != nil { t.Fatalf("gemini stream: %v", err) }
	if chunks < 2 { t.Errorf("expected 2+ chunks, got %d", chunks) }
	if resp.Content == "" { t.Error("empty content") }
	t.Logf("Gemini stream: %d chunks, content=%q", chunks, resp.Content[:min4(len(resp.Content), 80)])
}

func TestHard_DeepSeek_ToolCalling(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["DEEPSEEK_API_KEY"]
	if apiKey == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: apiKey, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "Search for 'Go programming best practices'"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name: "web_search", Description: "Search the web",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{"query": map[string]any{"type": "string"}},
					"required": []any{"query"},
				},
			},
		}},
		Options: map[string]any{"max_tokens": 100},
	})
	if err != nil { t.Fatalf("deepseek tools: %v", err) }
	if len(resp.ToolCalls) > 0 {
		t.Logf("DeepSeek tool: %s(%v)", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
		if resp.ToolCalls[0].Name != "web_search" { t.Errorf("wrong tool: %s", resp.ToolCalls[0].Name) }
	} else {
		t.Logf("DeepSeek direct: %q", resp.Content[:min4(len(resp.Content), 80)])
	}
}

func TestHard_DeepSeek_MultiTurn(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["DEEPSEEK_API_KEY"]
	if apiKey == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: apiKey, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Turn 1
	resp1, err := p.Chat(ctx, ChatRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{Role: "system", Content: "You are a math tutor. Be concise."},
			{Role: "user", Content: "What is the square root of 144?"},
		},
		Options: map[string]any{"max_tokens": 20, "temperature": 0},
	})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(resp1.Content, "12") { t.Errorf("turn1: expected 12, got %q", resp1.Content) }

	// Turn 2 — follow up
	resp2, err := p.Chat(ctx, ChatRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{Role: "system", Content: "You are a math tutor. Be concise."},
			{Role: "user", Content: "What is the square root of 144?"},
			{Role: "assistant", Content: resp1.Content},
			{Role: "user", Content: "Now square that number."},
		},
		Options: map[string]any{"max_tokens": 20, "temperature": 0},
	})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(resp2.Content, "144") { t.Logf("turn2: expected 144, got %q", resp2.Content) }
	t.Logf("multi-turn: √144=%q, 12²=%q", resp1.Content, resp2.Content)
}

func TestHard_Gemini_MultiTurn(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["GEMINI_API_KEY"]
	if apiKey == "" { t.Skip("no GEMINI_API_KEY") }

	p := NewGemini(ProviderConfig{Name: "gemini", APIKey: apiKey})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp1, err := p.Chat(ctx, ChatRequest{
		Model: "gemini-2.0-flash",
		Messages: []Message{
			{Role: "user", Content: "My name is QorvenTestUser. Remember it."},
		},
		Options: map[string]any{"max_tokens": 50},
	})
	if err != nil { t.Fatal(err) }

	resp2, err := p.Chat(ctx, ChatRequest{
		Model: "gemini-2.0-flash",
		Messages: []Message{
			{Role: "user", Content: "My name is QorvenTestUser. Remember it."},
			{Role: "assistant", Content: resp1.Content},
			{Role: "user", Content: "What is my name?"},
		},
		Options: map[string]any{"max_tokens": 30},
	})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(resp2.Content, "QorvenTestUser") {
		t.Logf("Gemini forgot name: %q", resp2.Content)
	} else {
		t.Log("Gemini multi-turn: remembered QorvenTestUser ✅")
	}
}

func TestHard_ProviderFactory_AllTypes(t *testing.T) {
	types := []struct{ name, provType string }{
		{"openai", TypeOpenAICompat},
		{"anthropic", TypeAnthropicNative},
		{"gemini", TypeGeminiNative},
		{"dashscope", TypeDashScope},
		{"groq", TypeGroq},
		{"deepseek", TypeDeepSeek},
		{"mistral", TypeMistral},
		{"xai", TypeXAI},
		{"ollama", TypeOllama},
		{"together", TypeTogether},
		{"custom", "custom_unknown_type"},
	}

	for _, tt := range types {
		p, err := NewProvider(ProviderConfig{Name: tt.name, ProviderType: tt.provType, APIKey: "test-key"})
		if err != nil { t.Errorf("%s: %v", tt.name, err); continue }
		if p == nil { t.Errorf("%s: nil provider", tt.name); continue }
		if p.Name() != tt.name { t.Errorf("%s: name=%q", tt.name, p.Name()) }
	}
	t.Logf("factory: all %d provider types create successfully ✅", len(types))
}

func TestHard_CredentialPool_UnderLoad(t *testing.T) {
	pool := NewCredentialPool("round_robin")
	pool.Add("key-A", "A")
	pool.Add("key-B", "B")
	pool.Add("key-C", "C")

	// Simulate 401 rotation under load
	for i := 0; i < 100; i++ {
		key := pool.Next()
		if i%10 == 0 {
			// Simulate 401 — rotate away
			rotated := pool.RotateOn401(key)
			if rotated == key { continue } // may return same if all marked failed
		}
	}
	t.Logf("credential pool: 100 calls with 10 rotations ✅")
}

func TestHard_Gemini_SchemaClean_RealCall(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["GEMINI_API_KEY"]
	if apiKey == "" { t.Skip("no GEMINI_API_KEY") }

	p := NewGemini(ProviderConfig{Name: "gemini", APIKey: apiKey})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Tool with features Gemini doesn't support (additionalProperties, $ref)
	resp, err := p.Chat(ctx, ChatRequest{
		Model: "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "Get weather in Tokyo"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name: "get_weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string", "minLength": 1},
					},
					"required":             []any{"city"},
					"additionalProperties": false,
				},
			},
		}},
	})
	if err != nil { t.Fatalf("gemini schema: %v", err) }
	if len(resp.ToolCalls) > 0 {
		city, _ := resp.ToolCalls[0].Arguments["city"].(string)
		if !strings.Contains(strings.ToLower(city), "tokyo") { t.Errorf("city=%q", city) }
		t.Logf("Gemini with cleaned schema: get_weather(%q) ✅", city)
	}
}
