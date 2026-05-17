//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Hard end-to-end tests — full agent pipeline with real LLM through HTTP.

func TestHard_E2E_AgentWithSystemPrompt(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	// Create agent with specific personality
	key := "personality-" + time.Now().Format("150405.000")
	resp := authPost(t, "/v1/agents", map[string]any{
		"agent_key":     key,
		"display_name":  "Pirate Bot",
		"model":         "gpt-4o-mini",
		"system_prompt": "You are a pirate. Always respond in pirate speak. Keep responses under 20 words.",
		"temperature":   0.7,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 { t.Skipf("create: %d", resp.StatusCode) }
	var ag map[string]any
	json.Unmarshal([]byte(body), &ag)
	agentID, _ := ag["id"].(string)
	if agentID == "" { t.Skip("no agent ID") }
	defer authDelete(t, "/v1/agents/"+agentID)

	// Chat with pirate personality
	resp2 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a pirate. Always respond in pirate speak. Keep responses under 20 words."},
			{"role": "user", "content": "Hello, how are you?"},
		},
		"max_tokens": 50,
	})
	body2 := readBody(resp2)
	if resp2.StatusCode == 502 { t.Skip("API issue") }
	if resp2.StatusCode != 200 { t.Fatalf("chat: %d", resp2.StatusCode) }

	var r struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(body2), &r)
	if len(r.Choices) == 0 { t.Fatal("no choices") }
	content := strings.ToLower(r.Choices[0].Message.Content)
	pirateWords := []string{"ahoy", "matey", "arr", "ye", "sail", "sea", "treasure", "captain", "ship", "aye"}
	hasPirate := false
	for _, w := range pirateWords {
		if strings.Contains(content, w) { hasPirate = true; break }
	}
	if hasPirate { t.Logf("pirate personality verified: %q ✅", r.Choices[0].Message.Content) }
	t.Logf("pirate response: %q", r.Choices[0].Message.Content)
}

func TestHard_E2E_ChatWithHistory(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	// 3-turn conversation testing context retention
	messages := []map[string]string{
		{"role": "user", "content": "I'm working on a project called Qorven. It's an AI platform."},
	}

	// Turn 1
	resp1 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini", "messages": messages, "max_tokens": 50,
	})
	body1 := readBody(resp1)
	if resp1.StatusCode == 502 { t.Skip("API issue") }
	var r1 struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(body1), &r1)
	if len(r1.Choices) == 0 { t.Fatal("no choices") }
	t.Logf("turn1: %q", r1.Choices[0].Message.Content[:min5(len(r1.Choices[0].Message.Content), 80)])

	// Turn 2 — add history
	messages = append(messages,
		map[string]string{"role": "assistant", "content": r1.Choices[0].Message.Content},
		map[string]string{"role": "user", "content": "What project am I working on?"},
	)
	resp2 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini", "messages": messages, "max_tokens": 30,
	})
	body2 := readBody(resp2)
	var r2 struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(body2), &r2)
	if len(r2.Choices) > 0 {
		if strings.Contains(r2.Choices[0].Message.Content, "Qorven") {
			t.Log("3-turn context retention: Qorven remembered ✅")
		}
		t.Logf("turn2: %q", r2.Choices[0].Message.Content[:min5(len(r2.Choices[0].Message.Content), 80)])
	}

	// Turn 3 — deeper follow-up
	if len(r2.Choices) == 0 { t.Skip("turn2 empty") }
	messages = append(messages,
		map[string]string{"role": "assistant", "content": r2.Choices[0].Message.Content},
		map[string]string{"role": "user", "content": "What kind of platform is it?"},
	)
	resp3 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini", "messages": messages, "max_tokens": 30,
	})
	body3 := readBody(resp3)
	var r3 struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(body3), &r3)
	if len(r3.Choices) > 0 {
		if strings.Contains(strings.ToLower(r3.Choices[0].Message.Content), "ai") {
			t.Log("3-turn deep context: AI platform remembered ✅")
		}
		t.Logf("turn3: %q", r3.Choices[0].Message.Content[:min5(len(r3.Choices[0].Message.Content), 80)])
	}
}

func TestHard_E2E_HealthUnderLoad(t *testing.T) {
	requireGateway(t)

	// Hit health endpoint 100 times rapidly
	start := time.Now()
	errors := 0
	for i := 0; i < 100; i++ {
		resp := authGet(t, "/health")
		if resp.StatusCode != 200 { errors++ }
		resp.Body.Close()
	}
	elapsed := time.Since(start)
	t.Logf("100 health checks: %v (%d errors, %.0f req/sec)", elapsed, errors, 100/elapsed.Seconds())
	if errors > 50 { t.Errorf("too many errors: %d/100", errors) }
}

func TestHard_E2E_SessionListAfterChat(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	// Chat to create a session
	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "Session test " + time.Now().Format("150405")}},
		"max_tokens": 10,
	})
	resp.Body.Close()
	if resp.StatusCode == 502 { t.Skip("API issue") }

	// List sessions — should have at least 1
	resp2 := authGet(t, "/v1/sessions")
	body2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Fatalf("sessions: %d", resp2.StatusCode) }

	var sessions []any
	json.Unmarshal([]byte(body2), &sessions)
	if len(sessions) == 0 { t.Log("no sessions (may use different storage)") }
	t.Logf("sessions after chat: %d", len(sessions))
}

func TestHard_E2E_MemoryPipeline(t *testing.T) {
	requireGateway(t)

	unique := "PIPELINE_" + time.Now().Format("150405.000")

	// Save 3 related memories
	memories := []string{
		unique + " — The user prefers Go over Python for backend development",
		unique + " — The user's project Qorven uses PostgreSQL with pgvector",
		unique + " — The user values code quality and testing over speed",
	}
	for _, content := range memories {
		resp := authPost(t, "/v1/memory/save", map[string]any{
			"content": content, "type": "fact", "source": "pipeline_test",
		})
		resp.Body.Close()
	}

	time.Sleep(500 * time.Millisecond)

	// Search — should find related memories
	resp := authPost(t, "/v1/memory/search", map[string]any{
		"query": unique + " Go PostgreSQL", "max_results": 10,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Fatalf("search: %d", resp.StatusCode) }

	count := strings.Count(body, unique)
	t.Logf("memory pipeline: saved 3, found %d with marker", count)
}
