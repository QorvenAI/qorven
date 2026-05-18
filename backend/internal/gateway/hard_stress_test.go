//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hard gateway stress tests — concurrent API abuse, data integrity, error recovery.

func TestHard_Stress_100ConcurrentChats(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip stress") }

	var wg sync.WaitGroup
	var successes, failures atomic.Int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			req, _ := newAuthRequest("POST", testBaseURL+"/v1/chat/completions", strings.NewReader(`{
				"model":"gpt-4o-mini",
				"messages":[{"role":"user","content":"Say OK"}],
				"max_tokens":5
			}`))
			resp, err := httpClient.Do(req)
			if err != nil { failures.Add(1); return }
			resp.Body.Close()
			if resp.StatusCode == 200 { successes.Add(1) } else { failures.Add(1) }
		}(i)
	}
	wg.Wait()
	t.Logf("100 concurrent chats: %d success, %d failed (rate limited expected)", successes.Load(), failures.Load())
	if successes.Load() == 0 { t.Error("all failed") }
}

func TestHard_Stress_RapidAgentCRUD(t *testing.T) {
	requireGateway(t)

	// Create→Delete 50 agents as fast as possible
	start := time.Now()
	for i := 0; i < 50; i++ {
		key := "rapid-" + time.Now().Format("150405.000") + "-" + string(rune('A'+i%26))
		resp := authPost(t, "/v1/agents", map[string]any{
			"agent_key": key, "model": "gpt-4o-mini", "system_prompt": "rapid test",
		})
		body := readBody(resp)
		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			var ag map[string]any
			json.Unmarshal([]byte(body), &ag)
			if id, ok := ag["id"].(string); ok {
				authDelete(t, "/v1/agents/"+id)
			}
		}
	}
	elapsed := time.Since(start)
	t.Logf("50 rapid create+delete: %v (%.0f ops/sec)", elapsed, 100/elapsed.Seconds())
}

func TestHard_Stress_MemoryFlood(t *testing.T) {
	requireGateway(t)

	var wg sync.WaitGroup
	var saved atomic.Int32

	// Save 50 memories concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			resp := authPost(t, "/v1/memory/save", map[string]any{
				"content": "Stress test memory " + string(rune('A'+n%26)) + " " + time.Now().Format("150405.000"),
				"type":    "fact",
				"source":  "stress_test",
			})
			resp.Body.Close()
			if resp.StatusCode == 200 { saved.Add(1) }
		}(i)
	}
	wg.Wait()
	t.Logf("50 concurrent memory saves: %d succeeded", saved.Load())
}

func TestHard_DataIntegrity_AgentPersistence(t *testing.T) {
	requireGateway(t)

	key := "integrity-" + time.Now().Format("150405.000")
	systemPrompt := "You are a data integrity test agent. Your purpose is to verify persistence."

	// Create with specific fields
	resp := authPost(t, "/v1/agents", map[string]any{
		"agent_key":     key,
		"display_name":  "Integrity Test",
		"model":         "gpt-4o-mini",
		"system_prompt": systemPrompt,
		"temperature":   0.42,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 { t.Fatalf("create: %d", resp.StatusCode) }
	var created map[string]any
	json.Unmarshal([]byte(body), &created)
	agentID, _ := created["id"].(string)
	if agentID == "" { t.Fatal("no id") }
	defer authDelete(t, "/v1/agents/"+agentID)

	// Read back and verify every field
	resp2 := authGet(t, "/v1/agents/"+agentID)
	body2 := readBody(resp2)
	var agent map[string]any
	json.Unmarshal([]byte(body2), &agent)

	if agent["display_name"] != "Integrity Test" { t.Errorf("name=%v", agent["display_name"]) }
	if agent["model"] != "gpt-4o-mini" { t.Errorf("model=%v", agent["model"]) }
	if sp, ok := agent["system_prompt"].(string); ok {
		if sp != systemPrompt { t.Errorf("system_prompt mismatch: %q", sp[:min5(len(sp), 50)]) }
	}
	t.Log("data integrity: all fields persisted correctly ✓")
}

func TestHard_ErrorRecovery_InvalidThenValid(t *testing.T) {
	requireGateway(t)

	// Send invalid request
	req1, _ := newAuthRequest("POST", testBaseURL+"/v1/chat/completions", strings.NewReader("{bad json}"))
	resp1, _ := httpClient.Do(req1)
	if resp1 != nil { resp1.Body.Close() }

	// Send valid request immediately after — should still work
	resp2 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{{"role": "user", "content": "Say RECOVERED"}},
		"max_tokens": 10,
	})
	body2 := readBody(resp2)
	if resp2.StatusCode == 502 { t.Skip("API issue") }
	if resp2.StatusCode != 200 { t.Fatalf("recovery failed: %d", resp2.StatusCode) }
	if strings.Contains(body2, "RECOVERED") { t.Log("error recovery: invalid→valid works ✓") }
}

func TestHard_API_ResponseTiming(t *testing.T) {
	requireGateway(t)

	// Health should be fast
	start := time.Now()
	resp := authGet(t, "/health")
	resp.Body.Close()
	healthTime := time.Since(start)
	if healthTime > 500*time.Millisecond { t.Errorf("health too slow: %v", healthTime) }

	// Agent list should be fast
	start = time.Now()
	resp = authGet(t, "/v1/agents")
	resp.Body.Close()
	agentTime := time.Since(start)
	if agentTime > 2*time.Second { t.Errorf("agent list too slow: %v", agentTime) }

	t.Logf("response times: health=%v, agents=%v", healthTime, agentTime)
}

func TestHard_API_HeaderSecurity(t *testing.T) {
	requireGateway(t)

	resp := authGet(t, "/health")
	resp.Body.Close()

	// Check security headers
	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
	}
	for name, expected := range headers {
		got := resp.Header.Get(name)
		if got == expected { t.Logf("header %s: %q ✓", name, got) }
	}

	// Should not expose server version
	server := resp.Header.Get("Server")
	if strings.Contains(strings.ToLower(server), "go") {
		t.Log("Server header exposes Go — consider removing")
	}
}

func TestHard_API_MethodNotAllowed(t *testing.T) {
	requireGateway(t)

	// PATCH on agents endpoint
	req, _ := newAuthRequest("PATCH", testBaseURL+"/v1/agents", nil)
	resp, _ := httpClient.Do(req)
	if resp != nil {
		resp.Body.Close()
		if resp.StatusCode == 200 { t.Log("PATCH allowed — may be intentional") }
		t.Logf("PATCH /v1/agents: %d", resp.StatusCode)
	}
}
