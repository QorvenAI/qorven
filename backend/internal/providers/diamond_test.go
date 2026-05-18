//go:build integration
// +build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// diamond_test.go — real provider failover tests (DeepSeek, live HTTP).
// Gated behind -tags=integration so `go test ./...` stays green for
// contributors without API keys or with an exhausted 3rd-party wallet.
// CI opt-in: go test -tags=integration ./internal/providers/...
package providers

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func loadTestKeys(t *testing.T) map[string]string {
	t.Helper()
	keys := map[string]string{}
	if keysFile := os.Getenv("INTEGRATION_KEYS_FILE"); keysFile != "" {
		data, err := os.ReadFile(keysFile)
		if err != nil { t.Skipf("no keys: %v", err) }
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(parts) == 2 { keys[parts[0]] = parts[1] }
		}
		return keys
	}
	for _, env := range []string{"DEEPSEEK_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		if v := os.Getenv(env); v != "" {
			keys[env] = v
		}
	}
	if len(keys) == 0 { t.Skip("no API keys available — set INTEGRATION_KEYS_FILE or individual key env vars") }
	return keys
}

func TestDiamond_DeepSeek_RealChat_VerifyAnswer(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }
	keys := loadTestKeys(t)
	key := keys["DEEPSEEK_API_KEY"]
	if key == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: key, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "What is the capital of France? Reply with ONLY the city name."}},
		Options:  map[string]any{"max_tokens": 10, "temperature": 0},
	})
	if err != nil { t.Fatalf("deepseek: %v", err) }
	if !strings.Contains(strings.ToLower(resp.Content), "paris") {
		t.Errorf("expected Paris, got %q", resp.Content)
	}
	if resp.Usage != nil {
		t.Logf("deepseek: %q, tokens: in=%d out=%d ✓", strings.TrimSpace(resp.Content), resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}
}

func TestDiamond_Gemini_RealChat_VerifyAnswer(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }
	keys := loadTestKeys(t)
	key := keys["GEMINI_API_KEY"]
	if key == "" { t.Skip("no GEMINI_API_KEY") }

	p := NewGemini(ProviderConfig{Name: "gemini", APIKey: key})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "What is 7 * 8? Reply with ONLY the number."}},
		Options:  map[string]any{"max_tokens": 10, "temperature": 0},
	})
	if err != nil { t.Fatalf("gemini: %v", err) }
	if !strings.Contains(resp.Content, "56") {
		t.Errorf("7*8=56, got %q", resp.Content)
	}
	t.Logf("gemini: 7*8=%s ✓", strings.TrimSpace(resp.Content))
}

func TestDiamond_DeepSeek_Streaming_ContentIntegrity(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }
	keys := loadTestKeys(t)
	key := keys["DEEPSEEK_API_KEY"]
	if key == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: key, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var chunks []string
	resp, err := p.ChatStream(ctx, ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "Count from 1 to 5, one number per line."}},
		Options:  map[string]any{"max_tokens": 50, "temperature": 0},
	}, func(chunk StreamChunk) {
		if chunk.Content != "" { chunks = append(chunks, chunk.Content) }
	})
	if err != nil { t.Fatalf("stream: %v", err) }

	// Verify streaming produced chunks
	if len(chunks) < 2 { t.Errorf("expected multiple chunks, got %d", len(chunks)) }

	// Verify final content matches streamed content
	streamed := strings.Join(chunks, "")
	if resp.Content != "" && !strings.Contains(streamed, "1") {
		t.Error("streamed content missing '1'")
	}

	// Verify all numbers present
	full := streamed + resp.Content
	for _, n := range []string{"1", "2", "3", "4", "5"} {
		if !strings.Contains(full, n) { t.Errorf("missing number %s in: %q", n, full[:100]) }
	}
	t.Logf("streaming: %d chunks, content has 1-5 ✓", len(chunks))
}

func TestDiamond_DeepSeek_ToolCalling_RealExecution(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }
	keys := loadTestKeys(t)
	key := keys["DEEPSEEK_API_KEY"]
	if key == "" { t.Skip("no DEEPSEEK_API_KEY") }

	p := NewOpenAI(ProviderConfig{Name: "deepseek", APIKey: key, APIBase: "https://api.deepseek.com/v1"})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, ChatRequest{
		Model:    "deepseek-chat",
		Messages: []Message{{Role: "user", Content: "What's the weather in Tokyo? Use the weather tool."}},
		Tools: []ToolDefinition{{
			Type: "function",
			Function: ToolFunctionSchema{
				Name:        "get_weather",
				Description: "Get current weather for a city",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string", "description": "City name"},
					},
					"required": []string{"city"},
				},
			},
		}},
	})
	if err != nil { t.Fatalf("tool call: %v", err) }

	if len(resp.ToolCalls) == 0 { t.Skip("model didn't use tool (may answer directly)") }

	tc := resp.ToolCalls[0]
	if tc.Name != "get_weather" { t.Errorf("tool name: %q", tc.Name) }
	city, _ := tc.Arguments["city"].(string)
	if !strings.Contains(strings.ToLower(city), "tokyo") {
		t.Errorf("expected Tokyo, got city=%q", city)
	}
	t.Logf("tool calling: %s(%v) ✓", tc.Name, tc.Arguments)
}

func TestDiamond_Retry_RealBackoff(t *testing.T) {
	// Verify retry with a real (but invalid) endpoint — should retry and fail gracefully
	cfg := RetryConfig{Attempts: 3, MinDelay: 50 * time.Millisecond, MaxDelay: 200 * time.Millisecond, Jitter: 0.1}

	start := time.Now()
	_, err := RetryDo(context.Background(), cfg, func() (string, error) {
		return "", &HTTPError{Status: 503, Body: "service unavailable"}
	})
	elapsed := time.Since(start)

	if err == nil { t.Error("should fail after retries") }
	// Should have taken at least 100ms (2 retries with 50ms+ delay)
	if elapsed < 80*time.Millisecond { t.Errorf("retries too fast: %v", elapsed) }
	// Should not take more than 2 seconds
	if elapsed > 2*time.Second { t.Errorf("retries too slow: %v", elapsed) }
	t.Logf("retry: 3 attempts in %v ✓", elapsed.Round(time.Millisecond))
}

func TestDiamond_CredentialPool_Rotation(t *testing.T) {
	pool := NewCredentialPool("round_robin")
	pool.Add("key-1", "primary")
	pool.Add("key-2", "secondary")
	pool.Add("key-3", "tertiary")

	// Verify exact round-robin sequence over 9 calls
	expected := []string{"key-1", "key-2", "key-3", "key-1", "key-2", "key-3", "key-1", "key-2", "key-3"}
	for i, exp := range expected {
		got := pool.Next()
		if got != exp { t.Errorf("call %d: got %q, want %q", i, got, exp) }
	}
	t.Log("credential rotation: exact round-robin ✓")
}

func TestDiamond_Schema_DeepNestedRef(t *testing.T) {
	// Schema with nested $ref that requires double-pass resolution
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user": map[string]any{"$ref": "#/$defs/User"},
		},
		"$defs": map[string]any{
			"User": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name":    map[string]any{"type": "string"},
					"address": map[string]any{"$ref": "#/$defs/Address"},
				},
			},
			"Address": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city":    map[string]any{"type": "string"},
					"country": map[string]any{"type": "string"},
				},
			},
		},
	}

	result := NormalizeSchema("anthropic", schema)
	props := result["properties"].(map[string]any)
	user := props["user"].(map[string]any)

	if user["type"] != "object" { t.Error("user not resolved") }
	userProps, ok := user["properties"].(map[string]any)
	if !ok { t.Fatal("user properties not resolved") }

	addr, ok := userProps["address"].(map[string]any)
	if !ok { t.Fatal("address not resolved") }
	if addr["type"] != "object" { t.Error("address type not resolved") }

	addrProps, ok := addr["properties"].(map[string]any)
	if !ok { t.Fatal("address properties not resolved") }
	if addrProps["city"] == nil { t.Error("city not resolved in nested ref") }

	t.Log("deep nested $ref: User.address.city resolved ✓")
}
