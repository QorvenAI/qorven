// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
	"github.com/qorvenai/qorven/internal/testutil"
)

// phase2TestEnv wires a minimal in-process gateway for the Phase 2
// HTTP auth suite. Real DB + real auth + real chi router under
// httptest. The goal is to prove the middleware chain rejects
// unauthorized callers against every new Phase 2 endpoint, not to
// replay the whole app loop.
type phase2TestEnv struct {
	t        *testing.T
	pool     *pgxpool.Pool
	tenantID string
	auth     *auth.AuthService
	gw       *Gateway
	server   *httptest.Server
	baseURL  string

	// Actors seeded by setupEnv.
	owner         *auth.User
	ownerToken    string
	outsider      *auth.User
	outsiderToken string
	admin         *auth.User
	adminToken    string

	serviceAccountID    string
	serviceAccountToken string // JWT minted as if the SA were a user

	planID      string
	sessionID   string
	approvalID  string
	permReqID   string
}

// JWT-sign secret is now owned by newMinimalGateway via
// testMinimalGatewayJWTSecret. Kept here only as a comment pointer.

func setupEnv(t *testing.T) *phase2TestEnv {
	t.Helper()

	// Ruling item #6: all Gateway field setup lives in newMinimalGateway.
	// This test and every future auth-matrix test uses it — no more
	// hand-duplication of the 30+-line field list.
	gw, pool, tenantID := newMinimalGateway(t, MinimalGatewayOpts{})

	// Use the SAME router-building code gateway.New() uses. Any drift
	// in middleware chain or route-mount order is caught because tests
	// and production share the function.
	r := chi.NewRouter()
	buildV1Router(gw, r)

	server := httptest.NewServer(r)
	t.Cleanup(server.Close)

	env := &phase2TestEnv{
		t: t, pool: pool, tenantID: tenantID, auth: gw.authSvc, gw: gw,
		server: server, baseURL: server.URL,
	}
	env.seedActors()
	return env
}

func (e *phase2TestEnv) seedActors() {
	ctx := context.Background()
	uniq := testutil.TempID("p2")

	owner, err := e.auth.CreateUser(ctx, "owner-"+uniq, "pw-owner-"+uniq, "owner@example.test", "user", e.tenantID)
	if err != nil {
		e.t.Fatalf("create owner: %v", err)
	}
	outsider, err := e.auth.CreateUser(ctx, "outsider-"+uniq, "pw-outsider-"+uniq, "outsider@example.test", "user", e.tenantID)
	if err != nil {
		e.t.Fatalf("create outsider: %v", err)
	}
	admin, err := e.auth.CreateUser(ctx, "admin-"+uniq, "pw-admin-"+uniq, "admin@example.test", "admin", e.tenantID)
	if err != nil {
		e.t.Fatalf("create admin: %v", err)
	}
	e.owner, e.ownerToken = owner, e.auth.IssueToken(owner)
	e.outsider, e.outsiderToken = outsider, e.auth.IssueToken(outsider)
	e.admin, e.adminToken = admin, e.auth.IssueToken(admin)

	// Seed a service account whose id matches a real user id. The
	// ownership check consults service_accounts by the actor id (which
	// ValidateToken resolves from the JWT's "sub"), so we reuse the
	// user's UUID as the service_accounts.id directly.
	saUser, err := e.auth.CreateUser(ctx, "svc-user-"+uniq, "sa-pw-"+uniq, "sa@example.test", "user", e.tenantID)
	if err != nil {
		e.t.Fatalf("create sa user: %v", err)
	}
	e.serviceAccountID = saUser.ID
	// Phase 2 fixture predates tenant-bound SAs; the phase 2 matrix
	// asserts the legacy blanket-pass SA behavior. Global (NULL-tenant)
	// is the correct flavor here.
	if _, err := e.gw.serviceAccounts.AddGlobal(ctx, e.serviceAccountID, serviceaccounts.RoleService, "test", "phase2"); err != nil {
		e.t.Fatalf("add service account: %v", err)
	}
	e.serviceAccountToken = e.auth.IssueToken(saUser)

	// Seed a minimal agent row (sessions.agent_id → agents.id FK).
	var agentID string
	if err := e.pool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'test', 'test-model')
        RETURNING id
    `, e.tenantID, "test-agent-"+uniq).Scan(&agentID); err != nil {
		e.t.Fatalf("seed agent: %v", err)
	}

	// Seed an owned session so protocolOwnerCheck has something real.
	sess, err := e.gw.sessions.CreateWithOwner(ctx, e.tenantID, agentID, "operator", e.owner.ID, "web")
	if err != nil {
		e.t.Fatalf("create session: %v", err)
	}
	e.sessionID = sess.ID

	// Seed a plan linked to the session.
	pl, err := e.gw.plans.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: e.tenantID, Title: "auth-test-" + uniq,
		SessionID: sess.ID, CreatedBy: e.owner.ID,
	})
	if err != nil {
		e.t.Fatalf("create plan: %v", err)
	}
	e.planID = pl.ID

	// Seed a pending approval so approve/reject/revise have a target.
	node, err := e.gw.plans.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: pl.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	if err != nil {
		e.t.Fatalf("append node: %v", err)
	}
	ap, err := e.gw.approvals.Request(ctx, pl.ID, node.ID, "system", nil)
	if err != nil {
		e.t.Fatalf("request approval: %v", err)
	}
	e.approvalID = ap.ID

	// Seed a pending permission request for permissions/reply path.
	var permReqID string
	if err := e.pool.QueryRow(ctx, `
        INSERT INTO permission_requests (session_id, tool, args, state, requested_by, expires_at)
        VALUES ($1::uuid, 'gh_push_file', '{"path":"x"}', 'pending', 'system', NOW() + INTERVAL '5 minutes')
        RETURNING id
    `, sess.ID).Scan(&permReqID); err != nil {
		e.t.Fatalf("seed permission request: %v", err)
	}
	e.permReqID = permReqID

	// Phase 7 test hygiene — permission_requests has no FK cascade
	// from sessions, so rows orphan when the tenant is dropped. Add a
	// scoped cleanup so the suite doesn't accumulate rows across runs.
	e.t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = e.pool.Exec(cctx, `DELETE FROM permission_requests WHERE id = $1`, permReqID)
	})
}

func (e *phase2TestEnv) do(method, path, token string, body any) *http.Response {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.baseURL+path, r)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("do request: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}

// ─────────── tests ───────────

// TestPhase2_AuthMatrix_Plans exercises the full matrix on a plan-bound
// endpoint. Every row MUST produce the expected HTTP status to prove
// the middleware chain actually enforces ownership.
func TestPhase2_AuthMatrix_Plans(t *testing.T) {
	env := setupEnv(t)

	cases := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"no-auth", "", http.StatusUnauthorized},
		{"owner", env.ownerToken, http.StatusOK},
		{"outsider", env.outsiderToken, http.StatusForbidden},
		{"admin", env.adminToken, http.StatusOK},
		{"service-account", env.serviceAccountToken, http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/GET_plan", func(t *testing.T) {
			resp := env.do("GET", "/v1/plans/"+env.planID, tc.token, nil)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got %d want %d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
			resp.Body.Close()
		})
		t.Run(tc.name+"/GET_plan_nodes", func(t *testing.T) {
			resp := env.do("GET", "/v1/plans/"+env.planID+"/nodes", tc.token, nil)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got %d want %d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
			resp.Body.Close()
		})
	}
}

// TestPhase2_AuthMatrix_ApproveReject covers the mutating plan endpoints.
// These require a valid pending approval, which the env seeds.
func TestPhase2_AuthMatrix_Reject(t *testing.T) {
	env := setupEnv(t)
	// Use reject so each sub-test can re-seed without fighting over the
	// same approval: we rebuild the env per case.
	cases := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"no-auth", "", http.StatusUnauthorized},
		{"outsider", env.outsiderToken, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.do("POST", "/v1/plans/"+env.planID+"/reject", tc.token,
				map[string]string{"comment": "unauthorized attempt"})
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got %d want %d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
			resp.Body.Close()
		})
	}
	// Owner: a real reject. Must succeed.
	t.Run("owner", func(t *testing.T) {
		resp := env.do("POST", "/v1/plans/"+env.planID+"/reject", env.ownerToken,
			map[string]string{"comment": "nope"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %d body=%s", resp.StatusCode, readBody(t, resp))
		}
		resp.Body.Close()
	})
}

// TestPhase2_AuthMatrix_Commands validates ownership on the command API,
// specifically submit_prompt (the highest-risk operation because it
// fires an agent run).
func TestPhase2_AuthMatrix_Commands(t *testing.T) {
	env := setupEnv(t)

	payload := map[string]any{"session_id": env.sessionID, "text": "hi"}

	cases := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"no-auth", "", http.StatusUnauthorized},
		{"outsider", env.outsiderToken, http.StatusForbidden},
		// Owner gets 503 because we don't wire gw.agentLoop in the test
		// env — but 503 is NOT 403, which is the important property:
		// auth passed, the handler reached Submit and failed for a
		// non-auth reason. That's the exact property this test must
		// prove (authorized requests reach the handler).
		{"owner", env.ownerToken, http.StatusInternalServerError},
		{"admin", env.adminToken, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.do("POST", "/v1/commands/submit_prompt", tc.token, payload)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got %d want %d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
			resp.Body.Close()
		})
	}
}

// TestPhase2_AuthMatrix_PermissionReply asserts the reply endpoint
// enforces session ownership.
func TestPhase2_AuthMatrix_PermissionReply(t *testing.T) {
	env := setupEnv(t)

	cases := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"no-auth", "", http.StatusUnauthorized},
		{"outsider", env.outsiderToken, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.do("POST", "/v1/permissions/"+env.permReqID+"/reply", tc.token,
				map[string]any{"decision": "allow"})
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got %d want %d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
			resp.Body.Close()
		})
	}
	// Owner gets 200 (allowed to reply).
	t.Run("owner", func(t *testing.T) {
		resp := env.do("POST", "/v1/permissions/"+env.permReqID+"/reply", env.ownerToken,
			map[string]any{"decision": "allow"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("got %d body=%s", resp.StatusCode, readBody(t, resp))
		}
		resp.Body.Close()
	})
}

// TestPhase2_ListPendingPermissions_OwnerOnly verifies the list
// endpoint enforces the query-parameter session id against the
// ownership check.
func TestPhase2_ListPendingPermissions_OwnerOnly(t *testing.T) {
	env := setupEnv(t)
	path := "/v1/permissions?session_id=" + env.sessionID

	for _, tc := range []struct {
		name       string
		token      string
		wantStatus int
	}{
		{"no-auth", "", http.StatusUnauthorized},
		{"outsider", env.outsiderToken, http.StatusForbidden},
		{"owner", env.ownerToken, http.StatusOK},
		{"admin", env.adminToken, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.do("GET", path, tc.token, nil)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("got %d want %d body=%s", resp.StatusCode, tc.wantStatus, readBody(t, resp))
			}
			resp.Body.Close()
		})
	}
}
