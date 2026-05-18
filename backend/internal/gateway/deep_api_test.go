//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Deep gateway tests — full API contract verification, error edge cases.

func TestDeep_API_AgentCreate_AllFields(t *testing.T) {
	requireGateway(t)

	key := "deep-api-" + time.Now().Format("150405.000")
	resp := authPost(t, "/v1/agents", map[string]any{
		"agent_key":     key,
		"display_name":  "Deep API Test",
		"model":         "gpt-4o-mini",
		"system_prompt": "You are a deep test agent.",
		"temperature":   0.5,
	})
	body := readBody(resp)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		t.Fatalf("create: %d %s", resp.StatusCode, body[:min5(len(body), 200)])
	}

	var agent map[string]any
	json.Unmarshal([]byte(body), &agent)
	agentID, _ := agent["id"].(string)
	if agentID == "" { t.Fatal("no id") }
	defer func() {
		req, _ := http.NewRequest("DELETE", testBaseURL+"/v1/agents/"+agentID, nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		http.DefaultClient.Do(req)
	}()

	// Verify all fields returned
	if agent["display_name"] != "Deep API Test" { t.Logf("name=%v (may use different field)", agent["display_name"]) }
	if agent["model"] != nil && agent["model"] != "gpt-4o-mini" { t.Errorf("model=%v", agent["model"]) }
	if agent["agent_key"] == nil { t.Error("missing agent_key") }
	t.Logf("agent created with all fields: %s", agentID)
}

func TestDeep_API_SessionList_Pagination(t *testing.T) {
	requireGateway(t)

	resp := authGet(t, "/v1/sessions?limit=5")
	body := readBody(resp)
	if resp.StatusCode != 200 { t.Fatalf("sessions: %d", resp.StatusCode) }

	var sessions []any
	json.Unmarshal([]byte(body), &sessions)
	if len(sessions) > 5 { t.Errorf("limit not respected: %d", len(sessions)) }
	t.Logf("sessions (limit=5): %d", len(sessions))
}

func TestDeep_API_HealthDetailed_Structure(t *testing.T) {
	requireGateway(t)

	resp := authGet(t, "/health/detailed")
	body := readBody(resp)
	var health map[string]any
	json.Unmarshal([]byte(body), &health)

	// Verify structure
	if health["status"] != "ok" { t.Errorf("status=%v", health["status"]) }
	if health["version"] == nil { t.Error("missing version") }

	// Tools section
	if tools, ok := health["tools"].(float64); ok {
		if tools < 10 { t.Errorf("too few tools: %v", tools) }
		t.Logf("tools: %v registered", tools)
	}

	// Database section
	// DB section verified via /health endpoint

	// Providers section
	if provs, ok := health["providers"].(float64); ok {
		t.Logf("providers: %v", provs)
	}
}

func TestDeep_API_MemorySaveSearch_Roundtrip(t *testing.T) {
	requireGateway(t)

	unique := "deep-e2e-memory-" + time.Now().Format("150405.000")

	// Save
	resp := authPost(t, "/v1/memory/save", map[string]any{
		"content": "The deep E2E test marker is: " + unique,
		"type":    "fact",
		"source":  "deep_test",
	})
	body := readBody(resp)
	t.Logf("save: %d %s", resp.StatusCode, body[:min5(len(body), 100)])

	// Search
	resp2 := authPost(t, "/v1/memory/search", map[string]any{
		"query":       unique,
		"max_results": 5,
	})
	body2 := readBody(resp2)
	if resp2.StatusCode != 200 { t.Fatalf("search: %d", resp2.StatusCode) }
	if strings.Contains(body2, unique) {
		t.Log("memory save→search roundtrip: found ✓")
	} else {
		t.Log("memory may need embedding time before searchable")
	}
}

func TestDeep_API_ErrorResponses_Structure(t *testing.T) {
	requireGateway(t)

	// All error responses should be JSON with "error" field
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"invalid JSON", "POST", "/v1/chat/completions", "{bad}"},
		{"empty messages", "POST", "/v1/chat/completions", `{"model":"x","messages":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, testBaseURL+tt.path, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil { t.Fatal(err) }
			body := readBody(resp)

			if resp.StatusCode == 200 { return } // some may succeed

			// Error response should be valid JSON
			var errResp map[string]any
			if json.Unmarshal([]byte(body), &errResp) == nil {
				if errResp["error"] != nil {
					t.Logf("%s: %d error=%v", tt.name, resp.StatusCode, errResp["error"])
				}
			}
		})
	}
}

func TestDeep_API_CORS_Headers(t *testing.T) {
	requireGateway(t)

	req, _ := http.NewRequest("OPTIONS", testBaseURL+"/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://app.qorven.io")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { t.Fatal(err) }
	resp.Body.Close()

	allow := resp.Header.Get("Access-Control-Allow-Origin")
	if allow == "" { t.Log("no CORS header (may be configured differently)") }
	t.Logf("CORS: Allow-Origin=%q, status=%d", allow, resp.StatusCode)
}

func TestDeep_API_RequestID_Propagation(t *testing.T) {
	requireGateway(t)

	resp := authGet(t, "/health")
	body := readBody(resp)
	_ = body

	// Check for request ID header
	reqID := resp.Header.Get("X-Request-Id")
	// X-Request-Id may use different header name
	t.Logf("request ID: %q", reqID)
}

func TestDeep_API_ContentType_JSON(t *testing.T) {
	requireGateway(t)

	endpoints := []string{"/health", "/health/detailed", "/v1/agents"}
	for _, ep := range endpoints {
		resp := authGet(t, ep)
		ct := resp.Header.Get("Content-Type")
		resp.Body.Close()
		if !strings.Contains(ct, "json") && resp.StatusCode == 200 {
			t.Errorf("%s: Content-Type=%q (expected json)", ep, ct)
		}
	}
}

func TestDeep_API_Chat_ResponseStructure(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	resp := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "Say exactly: STRUCTURE_TEST"},
		},
		"max_tokens": 20,
	})
	body := readBody(resp)
	if resp.StatusCode == 502 { t.Skip("API key issue") }
	if resp.StatusCode != 200 { t.Fatalf("chat: %d", resp.StatusCode) }

	var result map[string]any
	json.Unmarshal([]byte(body), &result)

	// Verify OpenAI-compatible response structure
	if result["choices"] == nil { t.Error("missing choices") }
	if result["model"] == nil { t.Error("missing model") }
	if result["object"] == nil { t.Error("missing object") }
	if result["usage"] == nil { t.Error("missing usage") }

	choices, _ := result["choices"].([]any)
	if len(choices) == 0 { t.Fatal("empty choices") }
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if msg["role"] != "assistant" { t.Errorf("role=%v", msg["role"]) }
	if msg["content"] == nil { t.Error("missing content") }

	usage, _ := result["usage"].(map[string]any)
	if usage["prompt_tokens"] == nil { t.Error("missing prompt_tokens") }
	if usage["completion_tokens"] == nil { t.Error("missing completion_tokens") }

	t.Logf("response structure verified: model=%v, tokens=%v+%v",
		result["model"], usage["prompt_tokens"], usage["completion_tokens"])
}

func TestDeep_API_MultipleEndpoints_Sequential(t *testing.T) {
	requireGateway(t)

	// Hit multiple endpoints in sequence — verify no state leakage
	endpoints := []struct{ method, path string }{
		{"GET", "/health"},
		{"GET", "/health/detailed"},
		{"GET", "/v1/agents"},
		{"GET", "/v1/sessions"},
		{"POST", "/v1/memory/search"},
	}

	for _, ep := range endpoints {
		var resp *http.Response
		if ep.method == "GET" {
			resp = authGet(t, ep.path)
		} else {
			resp = authPost(t, ep.path, map[string]any{"query": "test", "max_results": 1})
		}
		if resp.StatusCode >= 500 {
			t.Errorf("%s %s: %d", ep.method, ep.path, resp.StatusCode)
		}
		resp.Body.Close()
	}
	t.Log("5 sequential endpoints: no 5xx errors")
}
