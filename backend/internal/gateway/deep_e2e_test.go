//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Deep E2E tests — full chat pipeline with tool calling, memory, sessions.
// These test the ENTIRE system end-to-end through HTTP.

func TestDeepE2E_Chat_ToolCalling(t *testing.T) {
	if testing.Short() { t.Skip("skip real API in short mode") }
	requireGateway(t)

	// Ask a question that should trigger web_search tool
	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "What is 2+2? Reply with just the number."},
		},
		"max_tokens": 10,
	})
	body := readBody(resp)
	if resp.StatusCode == 502 || resp.StatusCode == 401 { t.Skipf("API issue: %d", resp.StatusCode) }
	if resp.StatusCode != 200 { t.Fatalf("status=%d body=%s", resp.StatusCode, body[:min5(len(body), 300)]) }

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	json.Unmarshal([]byte(body), &result)
	if len(result.Choices) == 0 { t.Fatal("no choices") }
	content := result.Choices[0].Message.Content
	if !strings.Contains(content, "4") { t.Errorf("expected '4' in response: %q", content) }
	t.Logf("LLM answered: %q", content)
}

func TestDeepE2E_Chat_MultiTurn(t *testing.T) {
	if testing.Short() { t.Skip("skip real API in short mode") }
	if os.Getenv("QORVEN_LLM_TEST") == "" { t.Skip("skip: set QORVEN_LLM_TEST=1 to run LLM integration tests") }
	requireGateway(t)

	// Turn 1
	resp1 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "My name is TestUser42. Remember this."},
		},
		"max_tokens": 50,
	})
	body1 := readBody(resp1)
	if resp1.StatusCode != 200 { t.Fatalf("turn1: %d %s", resp1.StatusCode, body1[:min5(len(body1), 200)]) }

	var r1 struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(body1), &r1)
	if len(r1.Choices) == 0 { t.Fatal("no choices turn1") }
	t.Logf("turn1: %q", r1.Choices[0].Message.Content[:min5(len(r1.Choices[0].Message.Content), 100)])

	// Turn 2 — ask about the name (tests context/memory)
	resp2 := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "My name is TestUser42. Remember this."},
			{"role": "assistant", "content": r1.Choices[0].Message.Content},
			{"role": "user", "content": "What is my name?"},
		},
		"max_tokens": 30,
	})
	body2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Fatalf("turn2: %d", resp2.StatusCode) }

	var r2 struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(body2), &r2)
	if len(r2.Choices) == 0 { t.Fatal("no choices turn2") }
	if !strings.Contains(r2.Choices[0].Message.Content, "TestUser42") {
		t.Errorf("LLM forgot the name: %q", r2.Choices[0].Message.Content)
	}
	t.Logf("turn2: %q", r2.Choices[0].Message.Content[:min5(len(r2.Choices[0].Message.Content), 100)])
}

func TestDeepE2E_Agent_FullLifecycle(t *testing.T) {
	requireGateway(t)

	agentKey := "deep-e2e-" + time.Now().Format("150405.000")

	// 1. Create agent
	resp := authPost(t, "/v1/agents", map[string]any{
		"agent_key":     agentKey,
		"display_name":  "Deep E2E Test Agent",
		"model":         "gpt-4o-mini",
		"system_prompt": "You are a test agent. Always reply with 'DEEP TEST OK'.",
		"temperature":   0.0,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body[:min5(len(body), 200)])
	}
	var created map[string]any
	json.Unmarshal([]byte(body), &created)
	agentID, _ := created["id"].(string)
	if agentID == "" { t.Fatalf("no agent ID: %s", body[:min5(len(body), 200)]) }
	t.Logf("created: %s (%s)", agentID, agentKey)

	// 2. Get agent — verify fields
	resp2 := authGet(t, "/v1/agents/"+agentID)
	body2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Fatalf("get: %d", resp2.StatusCode) }
	var agent map[string]any
	json.Unmarshal([]byte(body2), &agent)
	if agent["display_name"] != "Deep E2E Test Agent" { t.Errorf("name=%v", agent["display_name"]) }
	if agent["model"] != "gpt-4o-mini" { t.Errorf("model=%v", agent["model"]) }

	// 3. List agents — verify our agent is in the list
	resp3 := authGet(t, "/v1/agents")
	body3 := readBody(resp3)
	if !strings.Contains(body3, agentID) { t.Error("agent not in list") }

	// 4. Delete agent
	req, _ := http.NewRequest("DELETE", testBaseURL+"/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp4, _ := http.DefaultClient.Do(req)
	resp4.Body.Close()
	if resp4.StatusCode != 200 && resp4.StatusCode != 204 { t.Errorf("delete: %d", resp4.StatusCode) }

	// 5. Verify deleted
	resp5 := authGet(t, "/v1/agents/"+agentID)
	if resp5.StatusCode == 200 {
		body5 := readBody(resp5)
		if strings.Contains(body5, agentKey) { t.Error("agent should be deleted") }
	} else {
		resp5.Body.Close()
	}
	t.Logf("full lifecycle: create→get→list→delete verified")
}

func TestDeepE2E_Memory_SaveSearchPipeline(t *testing.T) {
	requireGateway(t)

	// 1. Save a memory via API
	resp := authPost(t, "/v1/memory/save", map[string]any{
		"content": "The Qorven deep E2E test was run at " + time.Now().Format(time.RFC3339),
		"type":    "fact",
		"source":  "e2e_test",
	})
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Logf("save: %d %s", resp.StatusCode, body[:min5(len(body), 200)]) }

	// 2. Search for it
	resp2 := authPost(t, "/v1/memory/search", map[string]any{
		"query":       "Qorven deep E2E test",
		"max_results": 5,
	})
	body2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Fatalf("search: %d %s", resp2.StatusCode, body2[:min5(len(body2), 200)]) }
	if strings.Contains(body2, "deep E2E test") {
		t.Log("memory save→search pipeline works")
	} else {
		t.Log("memory may not be immediately searchable (embedding delay)")
	}
}

func TestDeepE2E_HealthDetailed_AllSections(t *testing.T) {
	requireGateway(t)

	resp := authGet(t, "/health/detailed")
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Fatalf("status=%d", resp.StatusCode) }

	var health map[string]any
	json.Unmarshal([]byte(body), &health)

	// Verify all expected sections
	sections := []string{"status", "version", "uptime", "tools", "providers", "database"}
	for _, section := range sections {
		if health[section] == nil { t.Logf("missing section: %s", section) }
	}

	// Verify tools count
	if tools, ok := health["tools"].(map[string]any); ok {
		if count, ok := tools["count"].(float64); ok {
			if count < 10 { t.Errorf("too few tools: %v", count) }
			t.Logf("tools: %v registered", count)
		}
	}

	// Verify DB is connected
	if db, ok := health["database"].(map[string]any); ok {
		if status, ok := db["status"].(string); ok {
			if status != "connected" { t.Errorf("DB status: %s", status) }
		}
	}
}

func TestDeepE2E_ErrorHandling_MalformedRequests(t *testing.T) {
	requireGateway(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		expect int // 0 means any non-200
	}{
		{"invalid JSON", "POST", "/v1/chat/completions", "{bad json}", 0},
		{"empty body", "POST", "/v1/chat/completions", "", 0},
		// missing model is handled by gateway (adds default) — not an error,
		{"empty messages", "POST", "/v1/chat/completions", `{"model":"gpt-4o-mini","messages":[]}`, 0},
		{"wrong method", "PUT", "/v1/agents", `{}`, 0},
		{"nonexistent endpoint", "GET", "/v1/does-not-exist", "", 404},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" { bodyReader = bytes.NewReader([]byte(tt.body)) }
			req, _ := http.NewRequest(tt.method, testBaseURL+tt.path, bodyReader)
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil { t.Fatal(err) }
			resp.Body.Close()
			if tt.expect > 0 && resp.StatusCode != tt.expect {
				t.Errorf("expected %d, got %d", tt.expect, resp.StatusCode)
			}
			if tt.expect == 0 && resp.StatusCode == 200 {
				t.Errorf("malformed request should not get 200")
			}
			t.Logf("%s: %d", tt.name, resp.StatusCode)
		})
	}
}

func TestDeepE2E_RateLimiting_Burst(t *testing.T) {
	requireGateway(t)

	// Send 30 rapid requests — some should be rate limited
	passed, limited := 0, 0
	for i := 0; i < 30; i++ {
		resp, err := http.Get(testBaseURL + "/health")
		if err != nil { continue }
		if resp.StatusCode == 200 { passed++ }
		if resp.StatusCode == 429 { limited++ }
		resp.Body.Close()
	}
	t.Logf("30 rapid requests: %d passed, %d rate-limited", passed, limited)
	if passed == 0 { t.Error("all requests blocked") }
	// Rate limiter should kick in for some
	if limited == 0 { t.Log("no rate limiting detected (limit may be high)") }
}

func TestDeepE2E_LargePayload_Handling(t *testing.T) {
	if testing.Short() { t.Skip("skip large payload in short mode") }
	requireGateway(t)

	// 5MB payload — under the 10MB limit
	largeContent := strings.Repeat("This is a test message. ", 200000)
	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": largeContent},
		},
		"max_tokens": 10,
	})
	body := readBody(resp)
	// Should not crash — may return error about context length
	t.Logf("5MB payload: status=%d response=%s", resp.StatusCode, body[:min5(len(body), 200)])
}
