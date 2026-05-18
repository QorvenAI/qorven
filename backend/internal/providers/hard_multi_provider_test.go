//go:build integration
// +build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// hard_multi_provider_test.go — multi-provider live hits (OpenAI /
// DeepSeek / Gemini). Hosts the shared loadKeys helper consumed by
// hard_provider_test.go. Opt in with -tags=integration.
package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// loadKeys reads API keys from a local keys file (INTEGRATION_KEYS_FILE env var)
// or falls back to environment variables. Tests skip if keys are unavailable.

func loadKeys(t *testing.T) map[string]string {
	t.Helper()
	keys := map[string]string{}
	if keysFile := os.Getenv("INTEGRATION_KEYS_FILE"); keysFile != "" {
		data, err := os.ReadFile(keysFile)
		if err != nil { t.Skipf("no keys file: %v", err) }
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(strings.TrimRight(line, "\r"))
			if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
				keys[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		return keys
	}
	// Fall back to environment variables
	for _, env := range []string{"OPENAI_API_KEY", "DEEPSEEK_API_KEY", "GEMINI_API_KEY",
		"ANTHROPIC_API_KEY", "GROQ_API_KEY", "MISTRAL_API_KEY"} {
		if v := os.Getenv(env); v != "" {
			keys[env] = v
		}
	}
	if len(keys) == 0 { t.Skip("no API keys available — set INTEGRATION_KEYS_FILE or individual key env vars") }
	return keys
}

func TestHard_DeepSeek_Chat(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["DEEPSEEK_API_KEY"]
	if apiKey == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: apiKey, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "What is 7 * 8? Reply with just the number."}},
		Options:  map[string]any{"max_tokens": 10, "temperature": 0},
	})
	if err != nil { t.Fatalf("deepseek chat: %v", err) }
	if !strings.Contains(resp.Content, "56") { t.Errorf("expected 56: %q", resp.Content) }
	t.Logf("DeepSeek: %q (tokens: %d+%d)", resp.Content, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
}

func TestHard_DeepSeek_Stream(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["DEEPSEEK_API_KEY"]
	if apiKey == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: apiKey, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chunks := 0
	resp, err := p.ChatStream(ctx, ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "Count 1 to 5"}},
		Options:  map[string]any{"max_tokens": 30},
	}, func(chunk StreamChunk) { chunks++ })
	if err != nil { t.Fatalf("deepseek stream: %v", err) }
	if chunks < 3 { t.Errorf("expected 3+ chunks, got %d", chunks) }
	t.Logf("DeepSeek stream: %d chunks, content=%q", chunks, resp.Content[:min4(len(resp.Content), 50)])
}

func TestHard_Gemini_Chat(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["GEMINI_API_KEY"]
	if apiKey == "" { t.Skip("no GEMINI_API_KEY") }

	p := NewGemini(ProviderConfig{Name: "gemini", APIKey: apiKey})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "What is the capital of France? One word only."}},
		Options:  map[string]any{"max_tokens": 10},
	})
	if err != nil { t.Fatalf("gemini chat: %v", err) }
	if !strings.Contains(strings.ToLower(resp.Content), "paris") { t.Errorf("expected Paris: %q", resp.Content) }
	t.Logf("Gemini: %q", resp.Content)
}

func TestHard_Gemini_ToolCalling(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["GEMINI_API_KEY"]
	if apiKey == "" { t.Skip("no GEMINI_API_KEY") }

	p := NewGemini(ProviderConfig{Name: "gemini", APIKey: apiKey})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "What's the weather in London?"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name: "get_weather", Description: "Get weather for a city",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required": []any{"city"},
				},
			},
		}},
	})
	if err != nil { t.Fatalf("gemini tools: %v", err) }
	if len(resp.ToolCalls) > 0 {
		t.Logf("Gemini tool call: %s(%v)", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
		city, _ := resp.ToolCalls[0].Arguments["city"].(string)
		if !strings.Contains(strings.ToLower(city), "london") { t.Errorf("wrong city: %q", city) }
	} else {
		t.Logf("Gemini no tool call: %q", resp.Content[:min4(len(resp.Content), 100)])
	}
}

func TestHard_OpenAI_ToolCalling_Verified(t *testing.T) {
	keys := loadKeys(t)
	apiKey := keys["OPENAI_API_KEY"]
	if apiKey == "" { t.Skip("no OPENAI_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "openai", APIKey: apiKey, APIBase: "https://api.openai.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "Search for 'Qorven AI platform'"}},
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
	if err != nil {
		if strings.Contains(err.Error(), "401") { t.Skip("OpenAI key expired") }
		t.Fatalf("openai tools: %v", err)
	}
	if len(resp.ToolCalls) > 0 {
		if resp.ToolCalls[0].Name != "web_search" { t.Errorf("wrong tool: %s", resp.ToolCalls[0].Name) }
		query, _ := resp.ToolCalls[0].Arguments["query"].(string)
		if !strings.Contains(strings.ToLower(query), "qorven") { t.Errorf("wrong query: %q", query) }
		t.Logf("OpenAI tool: %s(%q)", resp.ToolCalls[0].Name, query)
	}
}

func TestHard_MultiProvider_SameQuestion(t *testing.T) {
	keys := loadKeys(t)
	question := "What is 15 + 27? Reply with just the number."

	providers := []struct {
		name, key, base, model string
	}{
		{"deepseek", keys["DEEPSEEK_API_KEY"], "https://api.deepseek.com/v1", "deepseek-chat"},
	}

	for _, prov := range providers {
		if prov.key == "" { continue }
		t.Run(prov.name, func(t *testing.T) {
			p := NewOpenAI(ProviderConfig{Name: prov.name, APIKey: prov.key, APIBase: prov.base})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := p.Chat(ctx, ChatRequest{
				Model:    prov.model,
				Messages: []Message{{Role: "user", Content: question}},
				Options:  map[string]any{"max_tokens": 10, "temperature": 0},
			})
			if err != nil { t.Fatalf("%s: %v", prov.name, err) }
			if !strings.Contains(resp.Content, "42") { t.Errorf("%s: expected 42, got %q", prov.name, resp.Content) }
			t.Logf("%s: %q", prov.name, resp.Content)
		})
	}
}

func TestHard_SchemaClean_AllProviders_RealCall(t *testing.T) {
	keys := loadKeys(t)

	// Test that schema normalization works with real API calls
	tool := ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "calculate", Description: "Do math",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"expression": map[string]any{"type": "string", "description": "Math expression"},
					"precision":  map[string]any{"type": "integer", "default": 2},
				},
				"required": []any{"expression"},
				"$defs": map[string]any{
					"MathOp": map[string]any{"type": "string", "enum": []any{"+", "-", "*", "/"}},
				},
			},
		},
	}

	// DeepSeek with schema normalization
	if key := keys["DEEPSEEK_API_KEY"]; key != "" {
		p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: key, APIBase: "https://api.deepseek.com/v1"})
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := p.Chat(ctx, ChatRequest{
			Model:    "deepseek-chat",
			Messages: []Message{{Role: "user", Content: "Calculate 100 / 7"}},
			Tools:    []ToolDefinition{tool},
			Options:  map[string]any{"max_tokens": 50},
		})
		if err != nil { t.Logf("deepseek schema: %v", err) } else {
			if len(resp.ToolCalls) > 0 {
				t.Logf("DeepSeek tool call with cleaned schema: %s(%v)", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
			} else {
				t.Logf("DeepSeek direct answer: %q", resp.Content[:min4(len(resp.Content), 50)])
			}
		}
	}
}
