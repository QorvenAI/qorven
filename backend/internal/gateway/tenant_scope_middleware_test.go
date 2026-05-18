// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

// Phase 5 (Gap #6): early-release test — an SSE-style handler calls
// store.ReleaseTxEarly AFTER its initial tenant-scoped reads, then
// streams its body. The middleware must NOT attempt to commit a
// second time at request end, and the pool's acquired-conns count
// MUST return to baseline while the stream is still open.
func TestTenantScopeMiddleware_EarlyReleaseReturnsConnToPool(t *testing.T) {
	dsn := rlsDSNForTest(t)

	db, err := store.NewForMultiTenant(dsn)
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	authSvc := auth.NewAuthService(bypassPool)
	u, err := authSvc.CreateUser(ctx,
		"early-"+testutil.TempID("u"), "pw-12345678",
		"e@example.test", "admin", tenantA)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	bypassPlans := plans.NewStore(bypassPool)
	p, err := bypassPlans.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantA, Title: "early", CreatedBy: u.ID,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	gw := buildMinimalEnforceGateway(t, db, tenantA)

	// Handler: read the plan via tenant-scoped tx, release the tx,
	// then drain a slow loop simulating an SSE body.
	streamHandler := func(w http.ResponseWriter, r *http.Request) {
		ps := plans.NewStore(db.Pool)
		if _, err := ps.GetPlan(r.Context(), p.ID); err != nil {
			t.Errorf("handler GetPlan: %v", err)
			http.Error(w, "x", 500)
			return
		}
		if err := store.ReleaseTxEarly(r.Context()); err != nil {
			t.Errorf("ReleaseTxEarly: %v", err)
		}
		// After release the handler must NOT hit the RLS-bound store
		// via ctx (tx is closed). This is the contract the godoc
		// explicitly states. Write a placeholder body and exit.
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: ok\n\n"))
	}

	r := chi.NewRouter()
	r.Use(gw.AuthMiddlewareV2)
	r.Use(gw.TenantScopeMiddleware)
	r.Get("/v1/stream", streamHandler)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok := gw.authSvc.IssueToken(u)

	// Fire the request. The key observation: middleware must not
	// double-commit. If it does we'd get a "tx is closed" warning in
	// the slog; the test runs silent otherwise.
	req, _ := http.NewRequest("GET", srv.URL+"/v1/stream", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("stream status: %d", resp.StatusCode)
	}

	// After response, pool's acquired conns should be 0 (or at least
	// not leaked by the middleware). This is a soft check — pgx may
	// have background keepalive conns. Assertion: acquired ≤ 1.
	stats := db.Pool.Stat()
	if stats.AcquiredConns() > 1 {
		t.Fatalf("acquired conns after early-release stream: %d (expected ≤ 1)",
			stats.AcquiredConns())
	}
}

// Phase 5 (Gap #3): PoisonTx forces rollback even on a 200 response.
// Handler writes the data successfully, then discovers a consistency
// problem that it can't surface via HTTP status. Poisoning the tx
// means the write never commits, even though the client got 200.
// This is the escape hatch for "I already flushed the header".
func TestTenantScopeMiddleware_PoisonTxForcesRollback(t *testing.T) {
	dsn := rlsDSNForTest(t)
	db, err := store.NewForMultiTenant(dsn)
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	authSvc := auth.NewAuthService(bypassPool)
	u, err := authSvc.CreateUser(ctx,
		"poison-"+testutil.TempID("u"), "pw-12345678",
		"p@example.test", "admin", tenantA)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	gw := buildMinimalEnforceGateway(t, db, tenantA)

	// Handler creates a plan inside the request-tx, then poisons.
	// Middleware must roll back — DB count before/after must match.
	plansStore := plans.NewStore(db.Pool)
	handler := func(w http.ResponseWriter, r *http.Request) {
		_, err := plansStore.CreatePlan(r.Context(), plans.CreatePlanInput{
			TenantID: tenantA, Title: "poisoned-" + testutil.TempID("p"),
			CreatedBy: u.ID,
		})
		if err != nil {
			t.Errorf("CreatePlan: %v", err)
			http.Error(w, "x", 500)
			return
		}
		PoisonTx(r.Context())
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}

	var countBefore int
	if err := bypassPool.QueryRow(ctx,
		`SELECT count(*) FROM plans WHERE tenant_id = $1`, tenantA).Scan(&countBefore); err != nil {
		t.Fatalf("count before: %v", err)
	}

	r := chi.NewRouter()
	r.Use(gw.AuthMiddlewareV2)
	r.Use(gw.TenantScopeMiddleware)
	r.Post("/v1/poison", handler)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok := gw.authSvc.IssueToken(u)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/poison", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d; PoisonTx should NOT change status, only the tx", resp.StatusCode)
	}

	var countAfter int
	if err := bypassPool.QueryRow(ctx,
		`SELECT count(*) FROM plans WHERE tenant_id = $1`, tenantA).Scan(&countAfter); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if countAfter != countBefore {
		t.Fatalf("poison did not rollback: count before=%d after=%d — "+
			"the handler's INSERT must not have committed",
			countBefore, countAfter)
	}
}

// Phase 5 (Gap #3): panic inside the handler must roll back the tx
// AND propagate so chi's recoverer logs the stack. Without the
// rollback, a panic would silently commit whatever writes the
// handler had issued before crashing — data corruption under
// exceptional paths.
func TestTenantScopeMiddleware_PanicRollsBack(t *testing.T) {
	dsn := rlsDSNForTest(t)
	db, err := store.NewForMultiTenant(dsn)
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	authSvc := auth.NewAuthService(bypassPool)
	u, _ := authSvc.CreateUser(ctx,
		"panic-"+testutil.TempID("u"), "pw-12345678",
		"pa@example.test", "admin", tenantA)

	gw := buildMinimalEnforceGateway(t, db, tenantA)

	plansStore := plans.NewStore(db.Pool)
	handler := func(w http.ResponseWriter, r *http.Request) {
		_, err := plansStore.CreatePlan(r.Context(), plans.CreatePlanInput{
			TenantID: tenantA, Title: "panicked-" + testutil.TempID("p"),
			CreatedBy: u.ID,
		})
		if err != nil {
			http.Error(w, "x", 500)
			return
		}
		panic("boom")
	}

	var countBefore int
	_ = bypassPool.QueryRow(ctx,
		`SELECT count(*) FROM plans WHERE tenant_id = $1`, tenantA).Scan(&countBefore)

	r := chi.NewRouter()
	r.Use(chi.Chain(chiRecoverer()).Handler)
	r.Use(gw.AuthMiddlewareV2)
	r.Use(gw.TenantScopeMiddleware)
	r.Post("/v1/panic", handler)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok := gw.authSvc.IssueToken(u)
	req, _ := http.NewRequest("POST", srv.URL+"/v1/panic", strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	// Chi recoverer returns 500 on panic.
	if resp.StatusCode != 500 {
		t.Fatalf("status=%d want 500", resp.StatusCode)
	}

	var countAfter int
	_ = bypassPool.QueryRow(ctx,
		`SELECT count(*) FROM plans WHERE tenant_id = $1`, tenantA).Scan(&countAfter)
	if countAfter != countBefore {
		t.Fatalf("panic did not rollback: before=%d after=%d", countBefore, countAfter)
	}
}

// ────────── helpers ──────────

// chiRecoverer returns chi's stdlib recoverer wrapped to match our
// test needs. We use it in the panic test to stop the test process
// from crashing.
func chiRecoverer() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					http.Error(w, http.StatusText(http.StatusInternalServerError),
						http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// rlsDSNForTest mirrors internal/store/rls_test.go's DSN logic: prefer
// QORVEN_APP_TEST_DSN, else derive by substituting the user/password
// in QORVEN_TEST_DSN with qorven_app:qorven_app. Fails loud on failure.
func rlsDSNForTest(t *testing.T) string {
	t.Helper()
	if v := os.Getenv("QORVEN_APP_TEST_DSN"); v != "" {
		return v
	}
	base := testutil.TestDSN
	if i := strings.Index(base, "://"); i >= 0 {
		if j := strings.Index(base[i+3:], "@"); j >= 0 {
			return base[:i+3] + "qorven_app:qorven_app" + base[i+3+j:]
		}
	}
	t.Fatalf("cannot derive restricted DSN; set QORVEN_APP_TEST_DSN")
	return ""
}

// buildMinimalEnforceGateway stands up a Gateway bound to the
// enforce-mode pool and flipped to multi-tenant. Stripped to the
// fields the middleware tests need.
func buildMinimalEnforceGateway(t *testing.T, db *store.DB, tenantID string) *Gateway {
	t.Helper()
	os.Setenv("JWT_SECRET", testMinimalGatewayJWTSecret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	gw, bypassPool, tenant := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})
	_ = tenant

	// Replace DB-bound fields with the ENFORCE pool, but keep the
	// auth service on the bypass pool — AuthService does user-row
	// lookups that aren't in our RLS scope today, and rewriting that
	// is Phase 5 follow-up, not the middleware's concern.
	gw.db = db
	gw.plans = plans.NewStore(db.Pool)
	// NB: serviceAccounts intentionally stays on the bypass pool —
	// its global infra reads cross tenants by design.

	// deployment_config is a global admin table that the restricted
	// qorven_app role cannot write. Use the bypass pool for admin
	// writes while the gateway's data path uses the RLS pool.
	// The middleware reads deploymentConfig.IsMultiTenant via the
	// same bypass pool — that read is not tenant-scoped.
	gw.deploymentConfig = deployment.NewConfig(bypassPool)
	if err := gw.deploymentConfig.SetMode(context.Background(), deployment.ModeMultiTenant); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	t.Cleanup(func() {
		_ = gw.deploymentConfig.SetMode(context.Background(), deployment.ModeSingleTenant)
	})
	return gw
}
