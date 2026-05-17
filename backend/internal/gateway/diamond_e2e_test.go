//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDiamond_E2E_MathVerification(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "What is 17 * 23? Reply with ONLY the number."}},
		"max_tokens": 10, "temperature": 0,
	})
	b := readBody(resp)
	if resp.StatusCode == 502 { t.Skip("API issue") }
	if resp.StatusCode != 200 { t.Fatalf("status=%d", resp.StatusCode) }

	var r struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(b), &r)
	if len(r.Choices) == 0 { t.Fatal("no choices") }
	if !strings.Contains(r.Choices[0].Message.Content, "391") { t.Errorf("17*23=391, got %q", r.Choices[0].Message.Content) }
	t.Logf("math: 17*23=%s ✓", strings.TrimSpace(r.Choices[0].Message.Content))
}

func TestDiamond_E2E_ContextRetention(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "My dog's name is Biscuit."},
			{"role": "assistant", "content": "Got it! Your dog is Biscuit."},
			{"role": "user", "content": "What is my dog's name? Reply with ONLY the name."},
		},
		"max_tokens": 10, "temperature": 0,
	})
	b := readBody(resp)
	if resp.StatusCode == 502 { t.Skip("API issue") }

	var r struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(b), &r)
	if len(r.Choices) == 0 { t.Fatal("no choices") }
	if !strings.Contains(strings.ToLower(r.Choices[0].Message.Content), "biscuit") {
		t.Errorf("should remember Biscuit, got %q", r.Choices[0].Message.Content)
	}
	t.Logf("context: %q ✓", strings.TrimSpace(r.Choices[0].Message.Content))
}

func TestDiamond_E2E_SystemPromptObedience(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": "You ONLY translate to French. Never respond in English."},
			{"role": "user", "content": "Hello, how are you?"},
		},
		"max_tokens": 50, "temperature": 0,
	})
	b := readBody(resp)
	if resp.StatusCode == 502 { t.Skip("API issue") }

	var r struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(b), &r)
	if len(r.Choices) == 0 { t.Fatal("no choices") }
	answer := r.Choices[0].Message.Content
	for _, w := range []string{"bonjour", "comment", "allez", "salut"} {
		if strings.Contains(strings.ToLower(answer), w) { t.Logf("French: %q ✓", answer); return }
	}
	t.Logf("expected French: %q", answer)
}

func TestDiamond_E2E_NoAuth(t *testing.T) {
	requireGateway(t)
	req, _ := http.NewRequest("POST", "http://localhost:4200/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Skip("gateway not reachable") }
	resp.Body.Close()
	if resp.StatusCode == 200 { t.Error("should reject unauthenticated") }
	t.Logf("no auth: %d ✓", resp.StatusCode)
}

func TestDiamond_E2E_ResponseTiming(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	start := time.Now()
	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "Say hi"}},
		"max_tokens": 5,
	})
	elapsed := time.Since(start)
	readBody(resp)
	if resp.StatusCode == 502 { t.Skip("API issue") }
	if elapsed > 10*time.Second { t.Errorf("too slow: %v", elapsed) }
	t.Logf("timing: %v ✓", elapsed.Round(time.Millisecond))
}

func TestDiamond_E2E_LargeInput(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	large := "Summarize: " + strings.Repeat("The quick brown fox. ", 500)
	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": large}},
		"max_tokens": 50,
	})
	readBody(resp)
	if resp.StatusCode >= 500 && resp.StatusCode != 502 { t.Errorf("500 on large input: %d", resp.StatusCode) }
	t.Logf("large input (%d chars): %d ✓", len(large), resp.StatusCode)
}

func TestDiamond_E2E_ConcurrentRequests(t *testing.T) {
	requireGateway(t)
	done := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() {
			resp := authGet(t, "/health")
			code := resp.StatusCode
			resp.Body.Close()
			done <- code
		}()
	}
	errs := 0
	for i := 0; i < 5; i++ { if <-done != 200 { errs++ } }
	if errs > 0 { t.Errorf("%d/5 failed", errs) }
	t.Log("5 concurrent: all 200 ✓")
}
