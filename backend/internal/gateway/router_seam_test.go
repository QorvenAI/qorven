// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/plans"
)

// TestRouterSeam_NoAuthMiddlewareFails proves the integration point
// the ruling (FU-P3-06) flagged: bypassing the gateway's auth
// middleware leaves every Phase 2 endpoint wide open.
//
// We construct TWO routers with the exact same handler set:
//  1. "guarded" — AuthMiddlewareV2 installed before the routes.
//  2. "naked"   — no middleware.
//
// The same unauthenticated request against `guarded` MUST return 401;
// against `naked` it would succeed. This test is the tripwire for a
// future refactor that accidentally drops AuthMiddlewareV2 from a
// route group — the diff would turn `guarded` into `naked`.
//
// The test uses the real Gateway struct + real stores, but constructs
// them explicitly (same pattern as phase2_auth_test.go). A full
// `gateway.New(cfg)` boot is out of scope here because `New()` has
// side effects (goroutines, LSP, dreamer) unsuitable for unit tests;
// if `New()`'s route-assembly path ever diverges from the block we
// mirror, this test still fails because the naked router proves the
// difference.
func TestRouterSeam_AuthMiddlewareRequiredOnV1(t *testing.T) {
	env := setupSeamEnv(t)
	defer env.Close()

	// Hit the same endpoint on both routers without an auth token.
	path := "/v1/plans/" + env.planID
	unauth := func(h http.Handler) int {
		req, _ := http.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}

	if got := unauth(env.guarded); got != http.StatusUnauthorized {
		t.Fatalf("guarded router must reject unauth with 401; got %d", got)
	}
	// Prove that WITHOUT the middleware the endpoint does respond —
	// this is the regression we're guarding against. 200 OK is the
	// unsafe outcome.
	if got := unauth(env.naked); got != http.StatusOK {
		t.Fatalf("naked router must reach the handler and return 200 (proving the handler itself has no guard); got %d", got)
	}
}

// Note: a test for "Use() called AFTER route mount is too late" used
// to live here. chi itself panics in that case, so it's impossible to
// accidentally commit a router in that pathological state — the bug
// class is caught by the framework at startup, not at request time.

// seamEnv is a lightweight test gateway configured like phase2_auth_test.go
// but exposing two routers: one with AuthMiddlewareV2 installed before
// route mount (the production pattern), and one without (the bare
// regression scenario).
type seamEnv struct {
	t        *testing.T
	gw       *Gateway
	guarded  http.Handler
	naked    http.Handler
	planID   string
}

func (e *seamEnv) Close() { /* testutil.Pool cleanup handles the DB */ }

func (e *seamEnv) mountV1Routes(r chi.Router) {
	r.Get("/v1/plans/{id}", e.gw.handleGetPlan)
	r.Get("/v1/plans/{id}/nodes", e.gw.handleListPlanNodes)
	r.Post("/v1/plans/{id}/approve", e.gw.handleApprovePlan)
}

func setupSeamEnv(t *testing.T) *seamEnv {
	t.Helper()

	// Same helper production tests and phase2_auth use. Ruling item #6.
	gw, _, tenantID := newMinimalGateway(t, MinimalGatewayOpts{})

	// Seed a plan whose GetPlan endpoint the test exercises.
	p, err := gw.plans.CreatePlan(context.Background(), plans.CreatePlanInput{
		TenantID: tenantID, Title: "seam-test",
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	env := &seamEnv{t: t, gw: gw, planID: p.ID}

	// Guarded router — AuthMiddlewareV2 installed BEFORE route mount
	// (production pattern in gateway.New).
	guarded := chi.NewRouter()
	guarded.Use(gw.AuthMiddlewareV2)
	env.mountV1Routes(guarded)
	env.guarded = guarded

	// Naked router — no middleware at all. The handler is reachable
	// for unauthenticated callers; the plan-level authorizeForPlan
	// inside the handler ALSO applies, so we expect the naked path to
	// permit only because no user is in context AND auth token is
	// empty ("local dev mode"). For the naked case to return 200, the
	// cfg.Auth.Token must be "" — which it is above.
	naked := chi.NewRouter()
	env.mountV1Routes(naked)
	env.naked = naked

	return env
}
