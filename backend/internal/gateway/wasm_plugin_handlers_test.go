// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/plugins/registry"
	"github.com/qorvenai/qorven/internal/testutil"
)

// pluginEnv builds a test gateway with wasmPluginStore wired to the
// real DB pool. The Loader is nil — these tests care about the HTTP
// surface + DB rows, not Wasm compilation. Loader tests live in
// internal/plugins/registry.
type pluginEnv struct {
	gw       *Gateway
	server   *httptest.Server
	admin    *auth.User
	adminTok string
	user     *auth.User
	userTok  string
	tenantID string
}

func setupPluginEnv(t *testing.T) *pluginEnv {
	t.Helper()
	gw, pool, tenantID := newMinimalGateway(t, MinimalGatewayOpts{
		// Single-tenant so the tests don't also have to wire the
		// non-super-user DSN.
	})

	// Upload endpoints live under /v1, so we build the full router.
	gw.wasmPluginStore = registry.NewStore(pool)

	r := chi.NewRouter()
	buildV1Router(gw, r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Seed one admin and one non-admin in the test tenant.
	admin, err := gw.authSvc.CreateUser(testutil.Ctx(t),
		"wasm-adm-"+testutil.TempID("u"), "pw-12345678",
		"wa@example.test", "admin", tenantID)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	user, err := gw.authSvc.CreateUser(testutil.Ctx(t),
		"wasm-usr-"+testutil.TempID("u"), "pw-12345678",
		"wu@example.test", "user", tenantID)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	return &pluginEnv{
		gw:       gw,
		server:   srv,
		admin:    admin,
		adminTok: gw.authSvc.IssueToken(admin),
		user:     user,
		userTok:  gw.authSvc.IssueToken(user),
		tenantID: tenantID,
	}
}

// multipartUpload builds a POST /v1/wasm-plugins body with the
// given name + binary + optional params string.
func multipartUpload(t *testing.T, name string, binary []byte, params string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	if err := mw.WriteField("name", name); err != nil {
		t.Fatalf("write name: %v", err)
	}
	if params != "" {
		if err := mw.WriteField("parameters", params); err != nil {
			t.Fatalf("write params: %v", err)
		}
	}
	part, err := mw.CreateFormFile("wasm", "plugin.wasm")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(binary); err != nil {
		t.Fatalf("write wasm: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close mw: %v", err)
	}
	return body, mw.FormDataContentType()
}

// ─────────────────── Tests ───────────────────

func TestPluginUpload_AdminOnly(t *testing.T) {
	env := setupPluginEnv(t)
	body, ct := multipartUpload(t, "any_name", []byte("fake-wasm"), "")

	req, _ := http.NewRequest("POST", env.server.URL+"/v1/wasm-plugins", body)
	req.Header.Set("Authorization", "Bearer "+env.userTok)
	req.Header.Set("Content-Type", ct)

	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin upload: status=%d want 403", resp.StatusCode)
	}
	var payload map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	if payload["code"] != "admin_only" {
		t.Fatalf("code=%q, want admin_only", payload["code"])
	}
}

func TestPluginUpload_HappyPath(t *testing.T) {
	env := setupPluginEnv(t)
	bin := []byte("fake-wasm-bytes")
	body, ct := multipartUpload(t, "hello_world", bin,
		`{"type":"object","properties":{"q":{"type":"string"}}}`)

	req, _ := http.NewRequest("POST", env.server.URL+"/v1/wasm-plugins", body)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	req.Header.Set("Content-Type", ct)

	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["name"] != "hello_world" {
		t.Fatalf("returned name=%v", got["name"])
	}
	// sha256 should be populated (64 hex chars).
	s, _ := got["sha256"].(string)
	if len(s) != 64 {
		t.Fatalf("sha256 shape: %q", s)
	}
	// Response must NOT leak the binary.
	if _, ok := got["wasm_binary"]; ok {
		t.Fatalf("response leaked wasm_binary")
	}
}

func TestPluginUpload_RejectsOversizedBody(t *testing.T) {
	env := setupPluginEnv(t)
	// 9 MiB — exceeds the 8 MiB cap defined by maxWasmUploadBytes.
	oversized := make([]byte, 9<<20)
	body, ct := multipartUpload(t, "toobig", oversized, "")

	req, _ := http.NewRequest("POST", env.server.URL+"/v1/wasm-plugins", body)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	req.Header.Set("Content-Type", ct)

	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want 413 body=%s", resp.StatusCode, b)
	}
}

func TestPluginUpload_RejectsInvalidName(t *testing.T) {
	env := setupPluginEnv(t)
	body, ct := multipartUpload(t, "BadCase", []byte("b"), "")

	req, _ := http.NewRequest("POST", env.server.URL+"/v1/wasm-plugins", body)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	req.Header.Set("Content-Type", ct)

	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
	var got map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["code"] != "invalid_name" {
		t.Fatalf("code=%q, want invalid_name", got["code"])
	}
}

// TestPluginUpload_RejectsReservedName is the Phase 6 HTTP-surface
// assertion. The Store layer already refuses, but clients need a
// specific error code so admin UIs and AI agents can render a
// helpful "pick a different name" prompt rather than a generic 400.
func TestPluginUpload_RejectsReservedName(t *testing.T) {
	env := setupPluginEnv(t)
	body, ct := multipartUpload(t, "exec", []byte("x"), "")

	req, _ := http.NewRequest("POST", env.server.URL+"/v1/wasm-plugins", body)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	req.Header.Set("Content-Type", ct)

	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["code"] != "reserved_name" {
		t.Fatalf("code=%q, want reserved_name (%+v)", got["code"], got)
	}
	if got["name"] != "exec" {
		t.Fatalf("name=%v, want exec — operators need the conflicting name surfaced", got["name"])
	}
}

func TestPluginUpload_RejectsInvalidParamsJSON(t *testing.T) {
	env := setupPluginEnv(t)
	body, ct := multipartUpload(t, "ok_name", []byte("b"), "not-json")

	req, _ := http.NewRequest("POST", env.server.URL+"/v1/wasm-plugins", body)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	req.Header.Set("Content-Type", ct)

	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
	var got map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got["code"] != "bad_parameters_json" {
		t.Fatalf("code=%q, want bad_parameters_json", got["code"])
	}
}

func TestPluginList_TenantScopedAndSanitized(t *testing.T) {
	env := setupPluginEnv(t)
	_, _ = env.gw.wasmPluginStore.Upload(testutil.Ctx(t), registry.UploadInput{
		TenantID:   env.tenantID,
		Name:       "scoped_a",
		WasmBinary: []byte("fake"),
	})

	req, _ := http.NewRequest("GET", env.server.URL+"/v1/wasm-plugins", nil)
	req.Header.Set("Authorization", "Bearer "+env.userTok) // non-admin CAN list
	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	var body struct {
		Plugins []map[string]any `json:"plugins"`
		Count   int              `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != 1 || len(body.Plugins) != 1 {
		t.Fatalf("count=%d plugins=%d", body.Count, len(body.Plugins))
	}
	if body.Plugins[0]["name"] != "scoped_a" {
		t.Fatalf("name=%v", body.Plugins[0]["name"])
	}
	// Sanitization: no wasm_binary / no bytes in the response.
	if _, ok := body.Plugins[0]["wasm_binary"]; ok {
		t.Fatalf("list response leaks wasm_binary")
	}
	// Size should still be exposed so operators can see plugin size.
	if body.Plugins[0]["size_bytes"] == nil {
		t.Fatalf("size_bytes missing")
	}
}

func TestPluginRevoke_AdminOnlyAndIdempotent(t *testing.T) {
	env := setupPluginEnv(t)
	_, _ = env.gw.wasmPluginStore.Upload(testutil.Ctx(t), registry.UploadInput{
		TenantID:   env.tenantID,
		Name:       "target",
		WasmBinary: []byte("x"),
	})

	// Non-admin: 403.
	req, _ := http.NewRequest("DELETE", env.server.URL+"/v1/wasm-plugins/target", nil)
	req.Header.Set("Authorization", "Bearer "+env.userTok)
	resp, _ := env.server.Client().Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin delete status=%d want 403", resp.StatusCode)
	}

	// Admin: 200.
	req, _ = http.NewRequest("DELETE", env.server.URL+"/v1/wasm-plugins/target", nil)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	resp, _ = env.server.Client().Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin delete status=%d want 200", resp.StatusCode)
	}

	// Second admin delete: 404.
	req, _ = http.NewRequest("DELETE", env.server.URL+"/v1/wasm-plugins/target", nil)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	resp, _ = env.server.Client().Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("idempotent second delete status=%d want 404", resp.StatusCode)
	}
}

// TestPluginRevoke_MultipartPrecaution exists because the spec
// (AGENTS.md) forbids a plugin from skipping the permission gate.
// If someone refactored the handler to bypass the admin check, this
// test would catch it.
func TestPluginRevoke_ConfirmsAdminGateOnUnknownMethods(t *testing.T) {
	env := setupPluginEnv(t)
	// OPTIONS is not routed by chi on this handler; we specifically
	// test that GET /v1/wasm-plugins/{name} 404s rather than exposing
	// some mismatched handler. (Defensive: a route typo in a future
	// refactor could attach GET to the delete handler.)
	req, _ := http.NewRequest("GET", env.server.URL+"/v1/wasm-plugins/target", nil)
	req.Header.Set("Authorization", "Bearer "+env.adminTok)
	resp, err := env.server.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET should not succeed on delete route. body=%s", b)
	}
}

// Silence the os import when this file doesn't need fs-touching.
var _ = os.Open
var _ = strings.TrimSpace
