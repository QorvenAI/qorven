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
	"sync"
	"testing"
	"time"
)

// End-to-end HTTP tests against the LIVE gateway at localhost:4200.
// These test the full stack: HTTP → router → auth → handler → DB → response.
// Skipped if gateway is not running.

const testBaseURL = "http://localhost:4200"

var testToken = func() string {
	if t := os.Getenv("QORVEN_TOKEN"); t != "" { return t }
	if t := os.Getenv("QORVEN_GATEWAY_TOKEN"); t != "" { return t }
	return "test123"
}()

func requireGateway(t *testing.T) {
	t.Helper()
	resp, err := http.Get(testBaseURL + "/health")
	if err != nil { t.Skipf("gateway not running: %v", err) }
	resp.Body.Close()
	if resp.StatusCode != 200 { t.Skipf("gateway unhealthy: %d", resp.StatusCode) }
}

func authGet(t *testing.T, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", testBaseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatalf("GET %s: %v", path, err) }
	return resp
}

func authPost(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", testBaseURL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatalf("POST %s: %v", path, err) }
	return resp
}

func readBody(resp *http.Response) string {
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// === HEALTH ENDPOINT ===

func TestE2E_Health(t *testing.T) {
	requireGateway(t)
	resp, err := http.Get(testBaseURL + "/health")
	if err != nil { t.Fatal(err) }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { t.Errorf("status=%d", resp.StatusCode) }
	body := readBody(resp)
	if !strings.Contains(body, "ok") { t.Errorf("body=%q", body) }
}

func TestE2E_HealthDetailed(t *testing.T) {
	requireGateway(t)
	resp := authGet(t, "/health/detailed")
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Errorf("status=%d body=%s", resp.StatusCode, body[:min5(len(body), 200)]) }
	var result map[string]any
	json.Unmarshal([]byte(body), &result)
	if result["status"] != "ok" { t.Errorf("status=%v", result["status"]) }
	if result["tools"] == nil { t.Error("missing tools in detailed health") }
}

// === AUTH ===

func TestE2E_Auth_NoToken(t *testing.T) {
	requireGateway(t)
	resp, _ := http.Get(testBaseURL + "/v1/agents")
	_ = readBody(resp)
	if resp.StatusCode != 401 && resp.StatusCode != 403 {
		if resp.StatusCode == 200 { t.Error("no-auth should not get 200") }
	}
}

func TestE2E_Auth_InvalidToken(t *testing.T) {
	requireGateway(t)
	req, _ := http.NewRequest("GET", testBaseURL+"/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-xyz")
	resp, _ := http.DefaultClient.Do(req)
	body := readBody(resp)
	if resp.StatusCode == 200 { t.Error("invalid token should not get 200") }
	_ = body
}

func TestE2E_Auth_ValidToken(t *testing.T) {
	requireGateway(t)
	resp := authGet(t, "/v1/agents")
	if resp.StatusCode != 200 { t.Errorf("valid token got %d", resp.StatusCode) }
	resp.Body.Close()
}

// === AGENT ENDPOINTS ===

func TestE2E_ListAgents(t *testing.T) {
	requireGateway(t)
	resp := authGet(t, "/v1/agents")
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Fatalf("status=%d", resp.StatusCode) }
	var agents []any
	json.Unmarshal([]byte(body), &agents)
	t.Logf("agents: %d", len(agents))
}

func TestE2E_AgentCRUD(t *testing.T) {
	requireGateway(t)

	// Create
	resp := authPost(t, "/v1/agents", map[string]any{
		"agent_key": "e2e-test-" + time.Now().Format("150405"),
		"model":     "gpt-4o-mini",
		"system_prompt": "You are an E2E test agent.",
	})
	body := readBody(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body[:min5(len(body), 200)])
	}
	var created map[string]any
	json.Unmarshal([]byte(body), &created)
	agentID, _ := created["id"].(string)
	if agentID == "" { t.Fatalf("no agent ID in response: %s", body[:min5(len(body), 200)]) }
	t.Logf("created agent: %s", agentID)

	// Get
	resp2 := authGet(t, "/v1/agents/"+agentID)
	body2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Errorf("get: %d", resp2.StatusCode) }
	if !strings.Contains(body2, agentID) { t.Error("agent ID not in response") }

	// Delete
	req, _ := http.NewRequest("DELETE", testBaseURL+"/v1/agents/"+agentID, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp3, _ := http.DefaultClient.Do(req)
	resp3.Body.Close()
	if resp3.StatusCode != 200 && resp3.StatusCode != 204 {
		t.Errorf("delete: %d", resp3.StatusCode)
	}
}

// === SESSION ENDPOINTS ===

func TestE2E_ListSessions(t *testing.T) {
	requireGateway(t)
	resp := authGet(t, "/v1/sessions")
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Errorf("status=%d body=%s", resp.StatusCode, body[:min5(len(body), 100)]) }
}

// === MEMORY ENDPOINTS ===

func TestE2E_MemorySearch(t *testing.T) {
	requireGateway(t)
	resp := authPost(t, "/v1/memory/search", map[string]any{
		"query":       "test query",
		"max_results": 5,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Errorf("status=%d body=%s", resp.StatusCode, body[:min5(len(body), 200)]) }
}

// === CHAT ENDPOINT ===

func TestE2E_Chat(t *testing.T) {
	requireGateway(t)
	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with exactly: PONG"},
		},
		"max_tokens": 10,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 {
		t.Fatalf("chat: %d %s", resp.StatusCode, body[:min5(len(body), 300)])
	}
	t.Logf("chat response: %s", body[:min5(len(body), 200)])
}

// === CONCURRENT REQUESTS ===

func TestE2E_ConcurrentHealth(t *testing.T) {
	if testing.Short() { t.Skip("skip concurrent in short mode") }
	requireGateway(t)
	var wg sync.WaitGroup
	errors := int32(0)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(testBaseURL + "/health")
			if err != nil || resp.StatusCode != 200 {
				errors++
			}
			if resp != nil { resp.Body.Close() }
		}()
	}
	wg.Wait()
	if errors > 25 { t.Errorf("%d/50 health checks failed (rate limiting expected)", errors) }
}

func TestE2E_ConcurrentAgentList(t *testing.T) {
	if testing.Short() { t.Skip("skip concurrent in short mode") }
	requireGateway(t)
	var wg sync.WaitGroup
	errors := int32(0)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", testBaseURL+"/v1/agents", nil)
			req.Header.Set("Authorization", "Bearer "+testToken)
			resp, err := http.DefaultClient.Do(req)
			if err != nil || resp.StatusCode != 200 { errors++ }
			if resp != nil { resp.Body.Close() }
		}()
	}
	wg.Wait()
	if errors > 15 { t.Errorf("%d/20 agent lists failed (rate limiting expected)", errors) }
}

// === ERROR HANDLING ===

func TestE2E_NotFound(t *testing.T) {
	requireGateway(t)
	req, _ := http.NewRequest("GET", testBaseURL+"/v1/nonexistent-endpoint-xyz", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	resp.Body.Close()
	if resp.StatusCode == 200 { t.Error("nonexistent endpoint should not return 200") }
	t.Logf("nonexistent: %d", resp.StatusCode)
}

func TestE2E_InvalidJSON(t *testing.T) {
	requireGateway(t)
	req, _ := http.NewRequest("POST", testBaseURL+"/v1/chat/completions", strings.NewReader("{invalid json}"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	resp.Body.Close()
	if resp.StatusCode == 200 { t.Error("invalid JSON should not get 200") }
	t.Logf("invalid JSON: %d", resp.StatusCode)
}

func TestE2E_EmptyBody(t *testing.T) {
	requireGateway(t)
	req, _ := http.NewRequest("POST", testBaseURL+"/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	resp.Body.Close()
	if resp.StatusCode == 200 { t.Error("empty body should not get 200") }
	t.Logf("empty body: %d", resp.StatusCode)
}

func TestE2E_LargeBody(t *testing.T) {
	requireGateway(t)
	// 5MB body — under the 10MB limit
	large := strings.Repeat("x", 5*1024*1024)
	req, _ := http.NewRequest("POST", testBaseURL+"/v1/chat/completions", strings.NewReader(large))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	// Should not crash the server
	t.Logf("large body response: %d", resp.StatusCode)
}

// === ALL GET ENDPOINTS ===

func TestE2E_AllGetEndpoints(t *testing.T) {
	requireGateway(t)
	endpoints := []string{
		"/health", "/health/detailed",
		"/v1/agents", "/v1/sessions", "/v1/teams",
		"/v1/plugins", "/v1/cron", "/v1/memory/scopes",
	}
	for _, ep := range endpoints {
		resp := authGet(t, ep)
		body := readBody(resp)
		if resp.StatusCode >= 500 {
			t.Errorf("%s: %d %s", ep, resp.StatusCode, body[:min5(len(body), 100)])
		} else {
			t.Logf("%s: %d", ep, resp.StatusCode)
		}
	}
}

func min5(a, b int) int { if a < b { return a }; return b }
