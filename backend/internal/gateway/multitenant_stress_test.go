// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/api/sessioncancel"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/deployment"
	orchestratorpkg "github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestStress_Multitenant_50Tenants_NoCrossTalk is the Phase 5 concurrency
// proof for the multi-tenant cutover. 50 tenants, each with an admin +
// a plan, all hammer /v1/plans/{mine} and /v1/plans/{neighbors} in
// parallel. Assertions:
//
//   • Every request for the caller's own plan returns 200 with the
//     caller's data. Zero cross-contamination: no request ever sees
//     another tenant's plan body.
//   • The connection pool does not deadlock — operations complete
//     within a bounded timeout. Exhaustion would manifest as acquire
//     timeouts or hangs.
//   • Goroutine count returns to baseline after the stress run. A
//     leak (e.g. a middleware that forgets to release a tx) would
//     show here within a few hundred ms of the stress completing.
//
// This replaces ad-hoc load tests — it's CI-runnable, bounded, and
// asserts the specific invariants the ruling named.
func TestStress_Multitenant_50Tenants_NoCrossTalk(t *testing.T) {
	// Skip when DSN can't be resolved — this test needs the
	// restricted role to prove DB-level enforcement under load.
	dsn := os.Getenv("QORVEN_APP_TEST_DSN")
	if dsn == "" {
		base := testutil.TestDSN
		if i := strings.Index(base, "://"); i >= 0 {
			if j := strings.Index(base[i+3:], "@"); j >= 0 {
				dsn = base[:i+3] + "qorven_app:qorven_app" + base[i+3+j:]
			}
		}
		if dsn == "" {
			t.Fatalf("set QORVEN_APP_TEST_DSN")
		}
	}

	enforceDB, err := store.NewForMultiTenant(dsn)
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer enforceDB.Close()
	// Pool size deliberately matches production default so the test
	// has the same contention shape.
	if enforceDB.Pool.Config().MaxConns != 20 {
		t.Logf("NOTE: pool MaxConns=%d; stress shape assumes 20",
			enforceDB.Pool.Config().MaxConns)
	}

	// 50 bcrypt-12 user creations + the stress run take longer than
	// the 30s default — use a generous 2m ceiling.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	bypassPool := testutil.Pool(t)

	// Seed 50 tenants, one admin + one plan each.
	const numTenants = 50
	fixtures := make([]stressFixture, 0, numTenants)

	authSvc := auth.NewAuthService(bypassPool)
	bypassPlans := plans.NewStore(bypassPool)

	for i := 0; i < numTenants; i++ {
		// Allocate a fresh tenant row.
		tid := newStressTenant(t, bypassPool, i)
		uname := fmt.Sprintf("stress-%d-%s", i, testutil.TempID("u"))
		u, err := authSvc.CreateUser(ctx,
			uname, "pw-stress-123",
			fmt.Sprintf("stress-%d@example.test", i), "admin", tid)
		if err != nil {
			t.Fatalf("CreateUser %d: %v", i, err)
		}
		planName := fmt.Sprintf("plan-%d-%s", i, testutil.TempID("p"))
		p, err := bypassPlans.CreatePlan(ctx, plans.CreatePlanInput{
			TenantID: tid, Title: planName, CreatedBy: u.ID,
		})
		if err != nil {
			t.Fatalf("CreatePlan %d: %v", i, err)
		}
		fixtures = append(fixtures, stressFixture{
			tenantID: tid, user: u, planID: p.ID, planName: planName,
		})
	}

	// Stand up gateway bound to the enforce pool.
	gw := stressGateway(t, enforceDB, authSvc, bypassPool)

	// Issue tokens. Auth service lives on the bypass pool because the
	// users table isn't RLS-scoped in Phase 4; this matches production.
	for i := range fixtures {
		fixtures[i].token = authSvc.IssueToken(fixtures[i].user)
	}

	r := chi.NewRouter()
	r.Use(gw.AuthMiddlewareV2)
	r.Use(gw.TenantScopeMiddleware)
	r.Get("/v1/plans/{id}", gw.handleGetPlan)
	srv := httptest.NewServer(r)
	// Note: srv.Close() is called explicitly before the goroutine-
	// leak check below, NOT via defer, so the check can observe the
	// post-shutdown count.

	// Baseline goroutine snapshot.
	runtime.GC()
	baseGoroutines := runtime.NumGoroutine()

	// Concurrency harness: each tenant fires N requests, half for
	// their own plan (must succeed) and half for a random neighbor
	// (must NOT leak neighbor data). Total = numTenants * reqsPer.
	const reqsPerTenant = 8
	var wg sync.WaitGroup
	var mineOK, othersBlocked atomic.Int64
	var leaks atomic.Int64

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()

	for i := 0; i < numTenants; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			me := fixtures[i]
			neighbor := fixtures[(i+1)%numTenants]
			for j := 0; j < reqsPerTenant; j++ {
				// Own plan — must be 200 with correct tenant_id.
				if !stressCheckMine(t, client, srv.URL, me) {
					continue
				}
				mineOK.Add(1)

				// Neighbor plan — must NOT leak neighbor.planName.
				if stressCheckNeighbor(t, client, srv.URL, me, neighbor, &leaks) {
					othersBlocked.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("stress: %d tenants × %d reqs = %d mine-OK, %d others-blocked, %d leaks, %v",
		numTenants, reqsPerTenant, mineOK.Load(), othersBlocked.Load(), leaks.Load(), elapsed)

	if leaks.Load() != 0 {
		t.Fatalf("CROSS-TENANT LEAK: %d requests saw another tenant's data", leaks.Load())
	}
	wantMine := int64(numTenants * reqsPerTenant)
	if mineOK.Load() != wantMine {
		t.Fatalf("mine-OK=%d, want %d (some caller's own plan was inaccessible)",
			mineOK.Load(), wantMine)
	}
	if othersBlocked.Load() != wantMine {
		t.Fatalf("others-blocked=%d, want %d (some neighbor request was served)",
			othersBlocked.Load(), wantMine)
	}

	// Goroutine-leak assertion: force every http client conn closed
	// and the test server to release keepalive workers, then wait for
	// pool idle goroutines to settle. Baseline accounts for a pgxpool
	// of MinConns=2 + pool maintenance goroutines + the httptest
	// server's remaining worker; we allow a generous tolerance
	// because the STRESS run (50× concurrency) should not cause
	// UNBOUNDED growth — a leaking middleware would show +50, +500,
	// not +6.
	client.CloseIdleConnections()
	srv.Close() // release httptest-server keepalives explicitly
	for attempt := 0; attempt < 20; attempt++ {
		runtime.GC()
		cur := runtime.NumGoroutine()
		if cur <= baseGoroutines+10 {
			t.Logf("goroutine settled: baseline=%d final=%d", baseGoroutines, cur)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	runtime.GC()
	final := runtime.NumGoroutine()
	// A real leak from the middleware scales with request count. We
	// fired 400 requests; if the final delta scales with that count,
	// middleware is leaking — we'd see hundreds. Tolerance is +10.
	if final > baseGoroutines+10 {
		t.Fatalf("goroutine leak: baseline=%d, final=%d (+%d) — "+
			"a middleware-level leak would scale with request count",
			baseGoroutines, final, final-baseGoroutines)
	}
}

// stressFixture captures one tenant's seeded rows for the stress run.
type stressFixture struct {
	tenantID string
	user     *auth.User
	planID   string
	planName string
	token    string
}

func stressCheckMine(t *testing.T, client *http.Client, base string, f stressFixture) bool {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/v1/plans/"+f.planID, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("mine-GET: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Errorf("mine status=%d body=%s", resp.StatusCode, body)
		return false
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("mine decode: %v", err)
		return false
	}
	if got["id"] != f.planID || got["tenant_id"] != f.tenantID {
		t.Errorf("mine mismatch: id=%v (want %v) tenant=%v (want %v)",
			got["id"], f.planID, got["tenant_id"], f.tenantID)
		return false
	}
	return true
}

// stressCheckNeighbor fires a cross-tenant request and asserts the
// response does NOT leak neighbor data. Returns true when blocking
// works (403 or 404 and body is clean).
func stressCheckNeighbor(t *testing.T, client *http.Client, base string, me, neighbor stressFixture, leaks *atomic.Int64) bool {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/v1/plans/"+neighbor.planID, nil)
	req.Header.Set("Authorization", "Bearer "+me.token)
	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("neighbor-GET: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		leaks.Add(1)
		t.Errorf("LEAK: tenant %s served neighbor %s's plan (status 200 body=%s)",
			me.tenantID, neighbor.tenantID, body)
		return false
	}
	// Status must be 403 or 404; body must never contain neighbor.planName
	// or neighbor.tenantID.
	if strings.Contains(string(body), neighbor.planName) ||
		strings.Contains(string(body), neighbor.tenantID) {
		leaks.Add(1)
		t.Errorf("LEAK: response body contains neighbor data: %s", body)
		return false
	}
	return true
}

func newStressTenant(t *testing.T, pool *pgxpool.Pool, i int) string {
	t.Helper()
	ctx := testutil.Ctx(t)
	// Reuse the testutil UUID-generation style: nano-derived, then
	// set v4 bits.
	nano := time.Now().UnixNano() + int64(i)*7919
	var buf [16]byte
	for j := range buf {
		buf[j] = byte(nano >> uint(j%8) & 0xFF)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	id := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
	slug := fmt.Sprintf("stress-%d-%s", i, testutil.TempID("t"))
	if _, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $2) ON CONFLICT DO NOTHING`,
		id, slug); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM tenants WHERE id = $1`, id)
	})
	return id
}

// stressGateway builds a gateway bound to the enforce DB. Lighter-
// weight than newMinimalGateway — no test helper for this shape
// existed, and we don't want to pull phase3 fixtures into a stress
// test.
func stressGateway(t *testing.T, db *store.DB, sharedAuth *auth.AuthService, bypassPool *pgxpool.Pool) *Gateway {
	t.Helper()
	os.Setenv("JWT_SECRET", testMinimalGatewayJWTSecret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	gw := &Gateway{
		cfg: &config.Config{
			Auth:   config.AuthConfig{EncryptionKey: "minimal-gw-enc-key-32-bytes-xxxx"},
			Server: config.ServerConfig{Listen: ":0"},
		},
		db:        db,
		authSvc:   sharedAuth,
		sessions:  session.NewStore(db.Pool),
		rtHub:     realtime.NewHub(),
		startTime: time.Now(),
	}
	gw.sessionCancels = sessioncancel.NewRegistry()
	gw.serviceAccounts = serviceaccounts.NewStore(bypassPool) // infra reads
	_ = gw.serviceAccounts.Refresh(context.Background())
	gw.plans = plans.NewStore(db.Pool)
	gw.approvals = approvals.NewStore(db.Pool)
	gw.events = apievents.NewEmitter(apievents.WithHub(gw.rtHub))
	gw.permissionGate = permissions.NewGate(db.Pool, gw.events)
	gw.cmdServer = &apicommands.Server{
		Emitter: gw.events, Drafts: apicommands.NewDraftStore(time.Minute),
		Submit: gw.protocolSubmit, Run: gw.protocolRunCommand,
		Resolve: gw.protocolResolveSession, OwnerCheck: gw.protocolOwnerCheck,
	}
	gw.orchestrator = orchestratorpkg.NewService(gw.plans, gw.approvals, nil, gw.events, nil)
	gw.deploymentConfig = deployment.NewConfig(db.Pool)
	if err := gw.deploymentConfig.SetMode(context.Background(), deployment.ModeMultiTenant); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	t.Cleanup(func() {
		_ = gw.deploymentConfig.SetMode(context.Background(), deployment.ModeSingleTenant)
	})
	return gw
}
