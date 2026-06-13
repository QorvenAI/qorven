// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The cron path must be able to resolve a non-empty UserID for a non-UUID agent id,
// and defaultTenant must be set — otherwise scheduled runs hit the permission gate
// with empty identity (the bug this fixes).

// TestCron_DefaultTenantIsSet confirms the package-level defaultTenant constant is
// non-empty. If it were empty, every cron RunRequest.TenantID would be empty and the
// permission gate would reject every scheduled run.
func TestCron_DefaultTenantIsSet(t *testing.T) {
	if defaultTenant == "" {
		t.Fatal("defaultTenant is empty; cron RunRequest.TenantID would be empty")
	}
}

// TestCron_ResolverPassesThroughUUID confirms that resolveTenantUserIDForChannel
// returns a valid UUID sender unchanged (no DB required). This is the cron fast-path
// when runAs is already a UUID-shaped agent id.
func TestCron_ResolverPassesThroughUUID(t *testing.T) {
	gw := &Gateway{} // no db — UUID passthrough requires none
	in := "550e8400-e29b-41d4-a716-446655440000"
	if got := gw.resolveTenantUserIDForChannel(context.Background(), defaultTenant, in); got != in {
		t.Fatalf("UUID passthrough failed: got %q, want %q", got, in)
	}
}

// TestCron_HandlersExist verifies that the three new handler methods compile and
// are present on *Gateway. It exercises them without a DB so we get 503/404 back,
// which confirms the handlers are wired and compile correctly.
func TestCron_HandlersExist(t *testing.T) {
	gw := &Gateway{} // nil db → handlers should return 503 or 200-empty

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		fn     http.HandlerFunc
		want   int
	}{
		{
			name:   "updateCronJob_nilDB_503",
			method: http.MethodPut,
			body:   `{"name":"x","cron_expression":"0 * * * *"}`,
			fn:     gw.handleUpdateCronJob,
			want:   http.StatusServiceUnavailable,
		},
		{
			name:   "runCronJobNow_nilDB_503",
			method: http.MethodPost,
			fn:     gw.handleRunCronJobNow,
			want:   http.StatusServiceUnavailable,
		},
		{
			name:   "listCronJobRuns_nilDB_200empty",
			method: http.MethodGet,
			fn:     gw.handleListCronJobRuns,
			want:   http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, "/cron-jobs/some-id", strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, "/cron-jobs/some-id", nil)
			}
			rr := httptest.NewRecorder()
			tc.fn(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("got status %d, want %d (body: %s)", rr.Code, tc.want, rr.Body.String())
			}
		})
	}
}

// TestCron_CreateAcceptsCanonicalKeys verifies that handleCreateCronJob accepts
// the canonical keys (name, cron_expression) and rejects a body missing them.
// No DB is needed — missing fields return 400 before any DB access.
func TestCron_CreateAcceptsCanonicalKeys(t *testing.T) {
	gw := &Gateway{}

	t.Run("missing_name_returns_400", func(t *testing.T) {
		body := `{"agent_id":"","cron_expression":"0 9 * * *"}`
		req := httptest.NewRequest(http.MethodPost, "/cron-jobs", strings.NewReader(body))
		rr := httptest.NewRecorder()
		gw.handleCreateCronJob(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d (body: %s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("missing_expr_returns_400", func(t *testing.T) {
		body := `{"name":"digest"}`
		req := httptest.NewRequest(http.MethodPost, "/cron-jobs", strings.NewReader(body))
		rr := httptest.NewRecorder()
		gw.handleCreateCronJob(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d (body: %s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("legacy_aliases_accepted_reaches_503", func(t *testing.T) {
		// Both legacy keys → validation passes → db==nil → 503
		body := `{"task":"summarize","expression":"0 9 * * *"}`
		req := httptest.NewRequest(http.MethodPost, "/cron-jobs", strings.NewReader(body))
		rr := httptest.NewRecorder()
		gw.handleCreateCronJob(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 (past validation, no db), got %d (body: %s)", rr.Code, rr.Body.String())
		}
	})

	t.Run("canonical_keys_accepted_reaches_503", func(t *testing.T) {
		// Canonical keys → validation passes → db==nil → 503
		body := `{"name":"digest","cron_expression":"0 9 * * *","instruction":"summarize"}`
		req := httptest.NewRequest(http.MethodPost, "/cron-jobs", strings.NewReader(body))
		rr := httptest.NewRecorder()
		gw.handleCreateCronJob(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 (past validation, no db), got %d (body: %s)", rr.Code, rr.Body.String())
		}
	})
}
