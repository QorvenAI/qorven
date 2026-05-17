//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// authDelete is a helper for DELETE requests with auth (not in e2e_test.go).
func authDelete(t *testing.T, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("DELETE", testBaseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

// unauthPost is a POST without auth token.
func unauthPost(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	resp, err := http.Post(testBaseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func TestAppToolEndpoint(t *testing.T) {
	requireGateway(t)

	dir := t.TempDir()

	toolScript := filepath.Join(dir, "echo_tool.sh")
	os.WriteFile(toolScript, []byte(`#!/bin/sh
args=$(cat)
printf '#!qorven:json\n{"text":"echo: %s","user":"echo: %s"}' "$args" "$args"
`), 0755)

	appYAML := fmt.Sprintf(`slug: test-echo-app
display_name: Test Echo App
version: 0.0.1
description: Integration test app
author: test
permissions:
  - tool_register
tools:
  - name: echo_args
    description: Echoes args back
    command: %s
`, toolScript)
	os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(appYAML), 0644)

	installResp := authPost(t, "/v1/apps", map[string]any{"path": dir})
	body := readBody(installResp)
	if installResp.StatusCode != 201 {
		t.Fatalf("install: %d %s", installResp.StatusCode, body)
	}
	var appRow map[string]any
	json.Unmarshal([]byte(body), &appRow)
	appID, _ := appRow["id"].(string)
	if appID == "" {
		t.Fatalf("no app id in response: %s", body)
	}
	defer func() {
		r := authDelete(t, "/v1/apps/"+appID)
		r.Body.Close()
	}()

	resp := authPost(t, "/v1/apps/test-echo-app/tools/echo_args", map[string]any{
		"args": map[string]any{"hello": "world"},
	})
	body = readBody(resp)
	if resp.StatusCode != 200 {
		t.Fatalf("run tool: %d %s", resp.StatusCode, body)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("parse result: %v\nbody: %s", err, body)
	}
	content, _ := result["content"].(string)
	if content == "" {
		t.Errorf("expected non-empty content, got: %v", result)
	}
	t.Logf("tool result: %s", content)
}

func TestAppToolEndpoint_NotFound_App(t *testing.T) {
	requireGateway(t)

	resp := authPost(t, "/v1/apps/no-such-app/tools/some_tool", map[string]any{})
	body := readBody(resp)
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for missing app, got %d: %s", resp.StatusCode, body)
	}
}

func TestAppToolEndpoint_NotFound_Tool(t *testing.T) {
	requireGateway(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.yaml"), []byte(`slug: test-notool-app
display_name: No Tool App
version: 0.0.1
permissions:
  - tool_register
tools: []
`), 0644)

	installResp := authPost(t, "/v1/apps", map[string]any{"path": dir})
	body := readBody(installResp)
	if installResp.StatusCode != 201 {
		t.Fatalf("install: %d %s", installResp.StatusCode, body)
	}
	var appRow map[string]any
	json.Unmarshal([]byte(body), &appRow)
	appID, _ := appRow["id"].(string)
	defer func() {
		r := authDelete(t, "/v1/apps/"+appID)
		r.Body.Close()
	}()

	resp := authPost(t, "/v1/apps/test-notool-app/tools/missing", map[string]any{})
	body = readBody(resp)
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for missing tool, got %d: %s", resp.StatusCode, body)
	}
}

func TestAppToolEndpoint_Unauthenticated(t *testing.T) {
	requireGateway(t)

	resp := unauthPost(t, "/v1/apps/any-app/tools/any_tool", map[string]any{})
	body := readBody(resp)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d: %s", resp.StatusCode, body)
	}
}
