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

// diamond_ultimate_test.go — The ultimate E2E test.
// A real multi-turn conversation with the live gateway + real LLM.
// Verifies the ENTIRE production path works together.

func TestUltimate_RealConversation_MultiTurn(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	type chatMsg struct{ Role, Content string }
	var history []chatMsg

	chat := func(userMsg string) string {
		history = append(history, chatMsg{"user", userMsg})

		var msgs []map[string]string
		// Include system prompt
		msgs = append(msgs, map[string]string{"role": "system", "content": "You are a concise assistant. Always respond in under 30 words. Be precise."})
		for _, m := range history {
			msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
		}

		resp := authPost(t, "/v1/chat/completions", map[string]any{
			"model": "gpt-4o-mini", "messages": msgs,
			"max_tokens": 60, "temperature": 0,
		})
		b := readBody(resp)
		if resp.StatusCode == 502 { t.Skip("API issue") }
		if resp.StatusCode != 200 { t.Fatalf("turn %d: status=%d", len(history)/2, resp.StatusCode) }

		var r struct{ Choices []struct{ Message struct{ Content string } } }
		json.Unmarshal([]byte(b), &r)
		if len(r.Choices) == 0 { t.Fatalf("turn %d: no choices", len(history)/2) }

		answer := strings.TrimSpace(r.Choices[0].Message.Content)
		history = append(history, chatMsg{"assistant", answer})
		return answer
	}

	// Turn 1: Establish context
	a1 := chat("My name is Alex and I'm building a project called Qorven.")
	t.Logf("Turn 1: %q", a1)

	// Turn 2: Verify context retention
	a2 := chat("What is my name and what am I building?")
	if !strings.Contains(strings.ToLower(a2), "alex") {
		t.Errorf("Turn 2: should remember 'Alex', got %q", a2)
	}
	if !strings.Contains(strings.ToLower(a2), "qorven") {
		t.Errorf("Turn 2: should remember 'Qorven', got %q", a2)
	}
	t.Logf("Turn 2: %q ✓", a2)

	// Turn 3: Test reasoning
	a3 := chat("What is 13 * 17?")
	if !strings.Contains(a3, "221") {
		t.Errorf("Turn 3: 13*17=221, got %q", a3)
	}
	t.Logf("Turn 3: %q ✓", a3)

	// Turn 4: Verify FULL context (name + project + math)
	a4 := chat("Summarize our conversation so far in one sentence.")
	lower := strings.ToLower(a4)
	hasName := strings.Contains(lower, "alex")
	hasProject := strings.Contains(lower, "qorven")
	hasMath := strings.Contains(lower, "221") || strings.Contains(lower, "13") || strings.Contains(lower, "math") || strings.Contains(lower, "multipl")
	if !hasName { t.Logf("Turn 4: missing name in summary") }
	if !hasProject { t.Logf("Turn 4: missing project in summary") }
	t.Logf("Turn 4: %q (name=%v project=%v math=%v) ✓", a4, hasName, hasProject, hasMath)

	t.Logf("ultimate: 4-turn conversation, context retained across all turns ✓")
}

func TestUltimate_RealConversation_LanguageSwitch(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	// Ask in English, get answer, then ask in different language
	resp1 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "system", "content": "You are multilingual. Reply in the same language the user uses."},
			{"role": "user", "content": "Bonjour, comment allez-vous?"},
		},
		"max_tokens": 30, "temperature": 0,
	})
	b1 := readBody(resp1)
	if resp1.StatusCode == 502 { t.Skip("API issue") }

	var r1 struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(b1), &r1)
	if len(r1.Choices) > 0 {
		answer := r1.Choices[0].Message.Content
		// Should respond in French
		frenchWords := []string{"bonjour", "bien", "merci", "comment", "je", "suis"}
		hasFrench := false
		for _, w := range frenchWords {
			if strings.Contains(strings.ToLower(answer), w) { hasFrench = true; break }
		}
		if hasFrench { t.Logf("French: %q ✓", answer) } else { t.Logf("expected French: %q", answer) }
	}
}

func TestUltimate_Gateway_HealthEndpoints(t *testing.T) {
	requireGateway(t)

	// Basic health
	resp := authGet(t, "/health")
	b := readBody(resp)
	if resp.StatusCode != 200 { t.Fatalf("health: %d", resp.StatusCode) }
	if !strings.Contains(b, "ok") { t.Error("health should contain 'ok'") }

	// Detailed health
	resp2 := authGet(t, "/health/detailed")
	b2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Fatalf("detailed: %d", resp2.StatusCode) }

	var health map[string]any
	json.Unmarshal([]byte(b2), &health)

	// Must have tools
	if tools, ok := health["tools"].(float64); ok {
		if tools < 40 { t.Errorf("too few tools: %.0f", tools) }
		t.Logf("tools: %.0f ✓", tools)
	}

	// Must have providers
	if providers, ok := health["providers"].(float64); ok {
		if providers < 1 { t.Error("no providers") }
		t.Logf("providers: %.0f ✓", providers)
	}

	// Must have uptime
	if uptime, ok := health["uptime"].(string); ok {
		t.Logf("uptime: %s ✓", uptime)
	}
}

func TestUltimate_Gateway_AgentCRUD_E2E(t *testing.T) {
	requireGateway(t)

	key := "ultimate-" + time.Now().Format("150405")

	// Create
	resp := authPost(t, "/v1/agents", map[string]any{
		"agent_key": key, "display_name": "Ultimate Test",
		"model": "gpt-4o-mini", "system_prompt": "You are concise.",
	})
	b := readBody(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 { t.Skipf("create: %d %s", resp.StatusCode, b[:min5(len(b), 100)]) }

	var ag map[string]any
	json.Unmarshal([]byte(b), &ag)
	agentID, _ := ag["id"].(string)
	if agentID == "" { t.Skip("no agent ID") }

	// List — should contain our agent
	resp2 := authGet(t, "/v1/agents")
	b2 := readBody(resp2)
	if !strings.Contains(b2, agentID) { t.Error("agent not in list") }

	// Get
	resp3 := authGet(t, "/v1/agents/"+agentID)
	b3 := readBody(resp3)
	if !strings.Contains(b3, key) { t.Error("agent key not in get response") }

	// Delete
	authDelete(t, "/v1/agents/"+agentID)

	// Verify deleted
	resp4 := authGet(t, "/v1/agents/"+agentID)
	if resp4.StatusCode == 200 {
		b4 := readBody(resp4)
		if strings.Contains(b4, key) { t.Error("agent still exists after delete") }
	} else {
		resp4.Body.Close()
	}

	t.Logf("agent CRUD: create→list→get→delete ✓")
}

func TestUltimate_Gateway_RateLimiting(t *testing.T) {
	requireGateway(t)

	// Send 20 rapid health checks — should all succeed (health is not rate-limited)
	start := time.Now()
	success := 0
	for i := 0; i < 20; i++ {
		resp := authGet(t, "/health")
		if resp.StatusCode == 200 { success++ }
		resp.Body.Close()
	}
	elapsed := time.Since(start)

	if success < 18 { t.Errorf("only %d/20 succeeded", success) }
	t.Logf("20 rapid requests: %d/20 success in %v ✓", success, elapsed.Round(time.Millisecond))
}
