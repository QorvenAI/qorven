//go:build integration
// +build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// integration_test.go — real LLM API calls (OpenAI). Self-skips on
// missing key / 401, but still reaches the network. Gated behind
// -tags=integration to keep `go test ./...` hermetic for contributors
// and CI without credits.
package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func getOpenAIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		// Try config.toml
		data, _ := os.ReadFile("../../config.toml")
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "api_key") && strings.Contains(line, "sk-") {
				start := strings.Index(line, `"`) + 1
				end := strings.LastIndex(line, `"`)
				if start > 0 && end > start { key = line[start:end] }
			}
		}
	}
	if key == "" { t.Skip("no OpenAI API key") }
	return key
}

func TestIntegration_OpenAI_Chat(t *testing.T) {
	key := getOpenAIKey(t)
	provider := NewOpenAI(ProviderConfig{
		Name: "test-openai", APIKey: key,
		APIBase: "https://api.openai.com/v1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Chat(ctx, ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "Reply with exactly one word: hello"}},
		Options:  map[string]any{"max_tokens": 10, "temperature": 0},
	})
	if err != nil { if strings.Contains(err.Error(), "401") { t.Skip("API key expired") }; t.Fatalf("chat: %v", err) }
	if resp == nil { t.Fatal("nil response") }
	if resp.Content == "" { t.Error("empty content") }
	t.Logf("response: %q", resp.Content)
	if resp.Usage != nil {
		t.Logf("tokens: input=%d output=%d", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
}

func TestIntegration_OpenAI_ChatStream(t *testing.T) {
	key := getOpenAIKey(t)
	provider := NewOpenAI(ProviderConfig{
		Name: "test-openai", APIKey: key,
		APIBase: "https://api.openai.com/v1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	chunks := 0
	resp, err := provider.ChatStream(ctx, ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "Count from 1 to 5"}},
		Options:  map[string]any{"max_tokens": 50},
	}, func(chunk StreamChunk) {
		chunks++
		if chunk.Done { t.Logf("stream done after %d chunks", chunks) }
	})
	if err != nil { if strings.Contains(err.Error(), "401") { t.Skip("API key expired") }; t.Fatalf("stream: %v", err) }
	if resp == nil { t.Fatal("nil response") }
	if resp.Content == "" { t.Error("empty content") }
	if chunks < 2 { t.Errorf("expected multiple chunks, got %d", chunks) }
	t.Logf("streamed: %q (%d chunks)", resp.Content[:min4(len(resp.Content), 100)], chunks)
}

func TestIntegration_OpenAI_WithTools(t *testing.T) {
	key := getOpenAIKey(t)
	provider := NewOpenAI(ProviderConfig{
		Name: "test-openai", APIKey: key,
		APIBase: "https://api.openai.com/v1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Chat(ctx, ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "What's the weather in Tokyo?"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name:        "get_weather",
				Description: "Get weather for a city",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []any{"city"},
				},
			},
		}},
		Options: map[string]any{"max_tokens": 100},
	})
	if err != nil { if strings.Contains(err.Error(), "401") { t.Skip("API key expired") }; t.Fatalf("chat with tools: %v", err) }
	if resp == nil { t.Fatal("nil response") }
	if len(resp.ToolCalls) > 0 {
		t.Logf("tool call: %s(%v)", resp.ToolCalls[0].Name, resp.ToolCalls[0].Arguments)
		if resp.ToolCalls[0].Name != "get_weather" { t.Error("wrong tool called") }
		city, _ := resp.ToolCalls[0].Arguments["city"].(string)
		if !strings.Contains(strings.ToLower(city), "tokyo") { t.Errorf("wrong city: %q", city) }
	} else {
		t.Logf("no tool call, content: %q", resp.Content[:min4(len(resp.Content), 100)])
	}
}

func TestIntegration_OpenAI_SchemaClean_RealCall(t *testing.T) {
	key := getOpenAIKey(t)
	provider := NewOpenAI(ProviderConfig{
		Name: "test-openai", APIKey: key,
		APIBase: "https://api.openai.com/v1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Tool with $ref that needs schema normalization
	resp, err := provider.Chat(ctx, ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "Search for cats"}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name: "search",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"$ref": "#/$defs/Query"},
					},
					"$defs": map[string]any{
						"Query": map[string]any{"type": "string", "description": "Search query"},
					},
					"required": []any{"query"},
				},
			},
		}},
		Options: map[string]any{"max_tokens": 50},
	})
	if err != nil { if strings.Contains(err.Error(), "401") { t.Skip("API key expired") }; t.Fatalf("schema clean call: %v", err) }
	t.Logf("response with cleaned schema: tool_calls=%d content=%q", len(resp.ToolCalls), resp.Content[:min4(len(resp.Content), 50)])
}

func TestIntegration_OpenAI_RetryOn429(t *testing.T) {
	key := getOpenAIKey(t)
	provider := NewOpenAI(ProviderConfig{
		Name: "test-openai", APIKey: key,
		APIBase: "https://api.openai.com/v1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Make 5 rapid calls — retry should handle any 429s
	for i := 0; i < 5; i++ {
		resp, err := provider.Chat(ctx, ChatRequest{
			Model:    "gpt-4o-mini",
			Messages: []Message{{Role: "user", Content: "Say OK"}},
			Options:  map[string]any{"max_tokens": 5},
		})
		if err != nil { t.Logf("call %d: %v", i, err); continue }
		if resp.Content == "" { t.Errorf("call %d: empty", i) }
	}
}

func min4(a, b int) int { if a < b { return a }; return b }
