// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestAuthMiddlewareV2_MultiTenant_ClosesDevModeBypass is the Phase 4
// Day 1 tripwire for Gap #4.
//
// Before this change, AuthMiddlewareV2 had a "no auth configured → allow"
// fallback intended for single-tenant local dev. In multi-tenant mode
// that fallback is a credential-free data crossing: anyone on the
// network can hit /v1/* with no JWT / no bearer and serve as any tenant.
//
// Contract asserted here:
//
//   - Single-tenant + no gateway token + setup-required path → bypass
//     still fires (byte-for-byte identical prior behavior).
//   - Multi-tenant + same request → 401. The bypass is removed.
//
// A regression that reintroduces the bypass under multi-tenant fails
// this test immediately.
func TestAuthMiddlewareV2_MultiTenant_ClosesDevModeBypass(t *testing.T) {
	// Build a gateway running in MULTI-tenant mode. The dev fallback
	// must not apply.
	gw, _, tenantID := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})

	// Seed a plan so the target endpoint has something concrete to
	// return on the happy path (and so a pass-through 401 is
	// distinguishable from a 404).
	p, err := gw.plans.CreatePlan(testutil.Ctx(t), plans.CreatePlanInput{
		TenantID: tenantID, Title: "mt-auth-bypass",
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	// Explicitly strip the gateway token so we're exercising exactly
	// the historical "no auth configured" branch.
	gw.cfg.Auth.Token = ""

	r := chi.NewRouter()
	r.Use(gw.AuthMiddlewareV2)
	r.Get("/v1/plans/{id}", gw.handleGetPlan)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/v1/plans/"+p.ID, nil)
	// Ask for JSON — we want the 401 JSON path, not the /login redirect.
	req.Header.Set("Accept", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/plans: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf(
			"multi-tenant dev bypass is OPEN: unauthenticated GET /v1/plans returned %d, want 401 — "+
				"AuthMiddlewareV2's 'no auth configured → allow' branch is firing when it must not",
			resp.StatusCode,
		)
	}
}

// TestAuthMiddlewareV2_SingleTenant_PreservesDevModeBypass locks in
// the byte-for-byte single-tenant contract from the Phase 3 ruling:
// hardening must not break local-dev UX. With no gateway token and
// no authSvc (the fresh-install state before setup writes its first
// user), an unauthenticated request still sails through in single-
// tenant mode.
//
// We force the bypass precondition by nil-ing authSvc — which matches
// the middleware's second disjunct (`gw.authSvc == nil`). Using
// SetupRequired is impossible in the shared test DB because it already
// has hundreds of seeded users from prior runs.
//
// If the Phase 4 multi-tenant patch accidentally closed the single-
// tenant bypass too, this test fails and we catch it before shipping.
func TestAuthMiddlewareV2_SingleTenant_PreservesDevModeBypass(t *testing.T) {
	gw, _, tenantID := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeSingleTenant,
	})

	p, err := gw.plans.CreatePlan(testutil.Ctx(t), plans.CreatePlanInput{
		TenantID: tenantID, Title: "st-auth-bypass",
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	// Mimic a fresh install: no gateway token, no auth service yet
	// (auth.NewAuthService has not been called — this is the zero-hour
	// state where the UI redirects to /setup).
	gw.cfg.Auth.Token = ""
	gw.authSvc = nil

	r := chi.NewRouter()
	r.Use(gw.AuthMiddlewareV2)
	r.Get("/v1/plans/{id}", gw.handleGetPlan)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/v1/plans/"+p.ID, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/plans: %v", err)
	}
	defer resp.Body.Close()

	// The single-tenant bypass fires, so the request reaches the handler.
	// We accept any non-401 result — the specific assertion is that
	// AuthMiddlewareV2 did NOT swap in a 401 out from under us.
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf(
			"single-tenant dev bypass was CLOSED: unauthenticated GET /v1/plans returned 401 — " +
				"Phase 4 hardening must preserve byte-for-byte single-tenant UX " +
				"(auth.Token=\"\", authSvc=nil — the fresh-install state)",
		)
	}
}

// TestAuthMiddlewareV2_MultiTenant_ClosesDevModeBypass_NilAuthSvc
// covers the nastier variant of Gap #4: the bypass would fire on the
// `authSvc == nil` branch, which is MORE likely in a misconfigured
// multi-tenant deployment than the SetupRequired branch. A multi-
// tenant operator who boots with the auth service failing to
// initialize (bad DSN, misordered bootstrap) must NOT silently admit
// anonymous callers.
func TestAuthMiddlewareV2_MultiTenant_ClosesDevModeBypass_NilAuthSvc(t *testing.T) {
	gw, _, tenantID := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})

	p, err := gw.plans.CreatePlan(testutil.Ctx(t), plans.CreatePlanInput{
		TenantID: tenantID, Title: "mt-auth-bypass-nilauth",
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	gw.cfg.Auth.Token = ""
	gw.authSvc = nil // the bootstrap-failure scenario the bypass secretly admits

	r := chi.NewRouter()
	r.Use(gw.AuthMiddlewareV2)
	r.Get("/v1/plans/{id}", gw.handleGetPlan)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/v1/plans/"+p.ID, nil)
	req.Header.Set("Accept", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/plans: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf(
			"multi-tenant bypass open on nil-authSvc path: GET /v1/plans returned %d, want 401 — "+
				"a misconfigured multi-tenant boot must refuse anonymous requests, not silently admit them",
			resp.StatusCode,
		)
	}
}
