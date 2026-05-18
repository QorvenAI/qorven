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
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestTenantQuotaMiddleware_HTTP_Returns429AndRetryAfter is the
// HTTP-surface integration: when a tenant is at its cap, the
// gateway responds 429 with Retry-After and a structured body.
// AI agents + dashboards rely on the error code 'tenant_quota' to
// render a specific "slow down" prompt rather than a generic 5xx.
//
// We test against the real /v1/wasm-plugins upload route because
// it goes through both AuthMiddlewareV2 (resolves tenant) AND
// TenantQuotaMiddleware, matching production.
func TestTenantQuotaMiddleware_HTTP_Returns429AndRetryAfter(t *testing.T) {
	// Tiny limits so we can trip 429 in one or two requests.
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      1,
		TenantRateLimitPerSecond: 1,
	})
	defer restore()

	// setupPluginEnv builds a real gateway + chi router covering
	// /v1/wasm-plugins. It leaves gw.tenantQuota nil — we install a
	// fresh one matching the BYOM knobs we just set.
	env := setupPluginEnv(t)
	env.gw.tenantQuota = NewTenantQuota(t.Context())

	// Replace the default router with one that actually applies
	// gw.TenantQuotaMiddleware. The env's server was built via
	// buildV1Router which we now re-use with a wrapped handler.
	env.server.Close()

	r := chi.NewRouter()
	buildV1Router(env.gw, r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Small helper to fire one upload POST. Returns (status, body).
	doUpload := func(t *testing.T, name string) (int, map[string]any) {
		t.Helper()
		body := &bytes.Buffer{}
		mw := multipart.NewWriter(body)
		_ = mw.WriteField("name", name)
		fw, _ := mw.CreateFormFile("wasm", "plugin.wasm")
		_, _ = fw.Write([]byte("x"))
		_ = mw.Close()
		req, _ := http.NewRequest("POST", srv.URL+"/v1/wasm-plugins", body)
		req.Header.Set("Authorization", "Bearer "+env.adminTok)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var got map[string]any
		_ = json.Unmarshal(raw, &got)
		return resp.StatusCode, got
	}

	// First upload — burst of 1 available, passes. It'll still 400
	// because the wasm bytes are garbage; we only care that it's
	// NOT 429 yet.
	status, _ := doUpload(t, "plug_one")
	if status == http.StatusTooManyRequests {
		t.Fatalf("first upload should not be rate-limited; got 429")
	}

	// Second upload fires immediately — the rate bucket is
	// exhausted (rps=1) and the concurrency slot was released,
	// but the tokens haven't refilled yet.
	status, body := doUpload(t, "plug_two")
	if status != http.StatusTooManyRequests {
		t.Fatalf("second upload status=%d want 429; body=%v", status, body)
	}
	if body["code"] != "tenant_quota" {
		t.Fatalf("error code=%v, want tenant_quota", body["code"])
	}
	if body["retry_after"] == nil {
		t.Fatalf("429 body missing retry_after field: %v", body)
	}
}

// Ensure the testutil import is used even when only referenced
// indirectly via setupPluginEnv.
var _ = testutil.TempID
