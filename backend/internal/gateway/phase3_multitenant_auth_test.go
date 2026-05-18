// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// ownedSession is a test fixture: a fully-seeded session row in a
// specific tenant with a specific owner_actor_id. Used to stand up
// the matrix below.
type ownedSession struct {
	sessionID string
	tenantID  string
	ownerID   string
}

func seedOwnedSession(t *testing.T, gw *Gateway, tenantID string, ownerID string) ownedSession {
	t.Helper()
	ctx := context.Background()

	// Minimal agent row for the sessions FK.
	var agentID string
	if err := gw.db.Pool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'authz-test', 'm')
        RETURNING id
    `, tenantID, "authz-"+testutil.TempID("a")).Scan(&agentID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	sess, err := gw.sessions.CreateWithOwner(ctx, tenantID, agentID, "operator", ownerID, "web")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return ownedSession{sessionID: sess.ID, tenantID: tenantID, ownerID: ownerID}
}

// seedUser seeds a user row directly on gw's pool/auth service and
// returns the user struct.
func seedUser(t *testing.T, gw *Gateway, tenantID, role string) *auth.User {
	t.Helper()
	uniq := testutil.TempID("u")
	u, err := gw.authSvc.CreateUser(
		context.Background(),
		role+"-"+uniq, "pw-"+uniq, role+"@example.test", role, tenantID,
	)
	if err != nil {
		t.Fatalf("CreateUser(%s): %v", role, err)
	}
	return u
}

// TestAuthorize_Matrix_SingleTenant is the regression guard for the
// ruling's "single-tenant behavior unchanged" mandate. Every pre-Phase-3
// outcome MUST hold here.
//
// Matrix columns: no-actor | owner | outsider-same-tenant | admin-same-tenant
//                 | admin-cross-tenant | service-account | legacy-no-owner-target
func TestAuthorize_Matrix_SingleTenant(t *testing.T) {
	gw, pool, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeSingleTenant,
	})

	owner := seedUser(t, gw, tenantA, "user")
	outsider := seedUser(t, gw, tenantA, "user")
	adminSame := seedUser(t, gw, tenantA, "admin")

	// Cross-tenant admin: seed a second tenant + an admin in it.
	tenantB := isolateSecondTenant(t, pool)
	adminCross := seedUser(t, gw, tenantB, "admin")

	sess := seedOwnedSession(t, gw, tenantA, owner.ID)

	// Legacy-no-owner: create a session then NULL its owner_actor_id.
	legacy := seedOwnedSession(t, gw, tenantA, owner.ID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE sessions SET owner_actor_id = NULL WHERE id = $1`, legacy.sessionID); err != nil {
		t.Fatalf("null owner for legacy session: %v", err)
	}

	// Service account: insert as a GLOBAL infra actor. The matrix row
	// this fixture backs asserts the legacy "service-account blanket
	// pass" behavior — i.e. a NULL-tenant SA. Tenant-scoped SAs get
	// their own dedicated matrix in service_account_tenant_test.go.
	sa := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.AddGlobal(context.Background(), sa.ID, "service", "authz-test", "test"); err != nil {
		t.Fatalf("add service account: %v", err)
	}

	cases := []struct {
		name     string
		actor    *auth.User
		target   string // session id
		wantCode string // empty = permit
	}{
		{"no-actor", nil, sess.sessionID, "no_actor"},
		{"owner", owner, sess.sessionID, ""},
		{"outsider-same-tenant", outsider, sess.sessionID, "not_owner"},
		{"admin-same-tenant", adminSame, sess.sessionID, ""},
		{"admin-cross-tenant-SINGLE_permits", adminCross, sess.sessionID, ""},
		{"service-account", sa, sess.sessionID, ""},
		{"admin-on-legacy-row-SINGLE_permits", adminSame, legacy.sessionID, ""},
		{"outsider-on-legacy-row", outsider, legacy.sessionID, "legacy_session_no_owner"},
	}

	// Use strict-auth mode (gw.cfg.Auth.Token != "") so the no-actor
	// branch is exercised instead of dev-mode silent admit.
	gw.cfg.Auth.Token = "force-strict-auth"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.actor != nil {
				ctx = withUser(ctx, tc.actor)
			}
			err := gw.authorize(ctx, AuthScope{SessionID: tc.target})
			assertAuthz(t, err, tc.wantCode)
		})
	}
}

// TestAuthorize_Matrix_MultiTenant proves the strict branches fire
// when the instance is in multi_tenant mode. Same matrix, different
// outcomes for the cross-tenant and legacy-no-owner rows.
func TestAuthorize_Matrix_MultiTenant(t *testing.T) {
	gw, pool, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})

	owner := seedUser(t, gw, tenantA, "user")
	outsider := seedUser(t, gw, tenantA, "user")
	adminSame := seedUser(t, gw, tenantA, "admin")
	tenantB := isolateSecondTenant(t, pool)
	adminCross := seedUser(t, gw, tenantB, "admin")
	userCross := seedUser(t, gw, tenantB, "user")

	sess := seedOwnedSession(t, gw, tenantA, owner.ID)
	legacy := seedOwnedSession(t, gw, tenantA, owner.ID)
	if _, err := pool.Exec(context.Background(),
		`UPDATE sessions SET owner_actor_id = NULL WHERE id = $1`, legacy.sessionID); err != nil {
		t.Fatalf("null owner for legacy session: %v", err)
	}

	// Global (NULL-tenant) SA — asserts the legacy "infra bypass"
	// path still works in multi-tenant mode. The tenant-scoped SA
	// matrix lives in service_account_tenant_test.go.
	sa := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.AddGlobal(context.Background(), sa.ID, "service", "authz-test-multi", "test"); err != nil {
		t.Fatalf("add service account: %v", err)
	}

	gw.cfg.Auth.Token = "force-strict-auth"

	cases := []struct {
		name     string
		actor    *auth.User
		target   string
		wantCode string
	}{
		// no-actor must deny even in dev-mode (no Auth.Token needed).
		{"no-actor-rejected-even-without-token", nil, sess.sessionID, "no_actor"},
		{"owner-same-tenant", owner, sess.sessionID, ""},
		{"outsider-same-tenant", outsider, sess.sessionID, "not_owner"},
		{"admin-same-tenant", adminSame, sess.sessionID, ""},
		{"admin-cross-tenant-MULTI_denies", adminCross, sess.sessionID, "cross_tenant_admin_denied"},
		{"user-cross-tenant-denies", userCross, sess.sessionID, "not_owner"},
		{"service-account-global", sa, sess.sessionID, ""},
		// Legacy rows are rejected for EVERYONE in multi-tenant,
		// including admins. They must be backfilled before the flip.
		{"admin-on-legacy-MULTI_denies", adminSame, legacy.sessionID, "legacy_session_no_owner"},
		{"owner-on-legacy-MULTI_denies", owner, legacy.sessionID, "legacy_session_no_owner"},
		// Session-not-found is a hard deny in multi-tenant.
		{"missing-session-MULTI_denies", owner, "00000000-0000-0000-0000-000000000000", "session_not_found"},
	}

	// Gate the non-actor test by dropping the token AND having no
	// user in context. In multi-tenant, authorize must deny even here.
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// For the no-actor case, simulate the dev-mode condition
			// (no token) — we prove that multi-tenant ignores the
			// dev fallback entirely.
			oldTok := gw.cfg.Auth.Token
			if tc.actor == nil {
				gw.cfg.Auth.Token = ""
				defer func() { gw.cfg.Auth.Token = oldTok }()
			}
			ctx := context.Background()
			if tc.actor != nil {
				ctx = withUser(ctx, tc.actor)
			}
			err := gw.authorize(ctx, AuthScope{SessionID: tc.target})
			assertAuthz(t, err, tc.wantCode)
		})
	}
}

// TestAuthorize_MultiTenant_DevModeFallbackRetired makes the "no
// silent admit when no token is configured" invariant its own test so
// the intent is discoverable in isolation.
func TestAuthorize_MultiTenant_DevModeFallbackRetired(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})
	gw.cfg.Auth.Token = "" // simulate dev-mode config

	// No user on ctx + no token — single-tenant would permit; multi
	// must deny.
	err := gw.authorize(context.Background(), AuthScope{SessionID: "whatever"})
	if err == nil {
		t.Fatalf("multi-tenant must deny unauthenticated access even with empty Auth.Token")
	}
	var ae *apicommands.AuthzError
	if !errorAs(err, &ae) || ae.Code != "no_actor" {
		t.Fatalf("expected AuthzError no_actor, got %T %v", err, err)
	}
}

// ────────── helpers ──────────

// isolateSecondTenant creates a second tenant row on the same pool
// for tests that need two distinct tenants. Registers cleanup.
func isolateSecondTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := newV4UUID()
	if _, err := pool.Exec(context.Background(), `
        INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $3)
        ON CONFLICT DO NOTHING
    `, id, "mt-test-"+id[:8], "mt-test-"+id[:8]); err != nil {
		t.Fatalf("seed second tenant: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, id)
	})
	return id
}

// newV4UUID builds a v4 UUID. Same recipe as testutil.NewIsolatedTenant.
func newV4UUID() string {
	var buf [16]byte
	for i := range buf {
		buf[i] = byte(testutilRandByte(i))
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmtUUID(buf)
}

// testutilRandByte / fmtUUID are intentionally tiny inlined helpers
// so this test file doesn't depend on testutil internals.
func testutilRandByte(i int) byte {
	// Deterministic-enough pseudo-random based on call counter.
	v2nonce++
	return byte((int64(v2nonce) ^ int64(i)<<3) & 0xFF)
}

var v2nonce uint64

func fmtUUID(b [16]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 36)
	pos := 0
	for i, bt := range b {
		out[pos] = hex[bt>>4]
		out[pos+1] = hex[bt&0x0f]
		pos += 2
		if i == 3 || i == 5 || i == 7 || i == 9 {
			out[pos] = '-'
			pos++
		}
	}
	return string(out)
}

// assertAuthz asserts err matches the expected AuthzError code
// (empty want = permit).
func assertAuthz(t *testing.T, err error, wantCode string) {
	t.Helper()
	if wantCode == "" {
		if err != nil {
			t.Fatalf("expected permit; got: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected deny with code %q; got permit", wantCode)
	}
	var ae *apicommands.AuthzError
	if !errorAs(err, &ae) {
		t.Fatalf("expected *AuthzError, got %T: %v", err, err)
	}
	if ae.Code != wantCode {
		t.Fatalf("code: got %q want %q (reason=%q)", ae.Code, wantCode, ae.Reason)
	}
}

// errorAs is a thin wrapper over errors.As that doesn't import the
// package into this file.
func errorAs(err error, target any) bool {
	// go:noinline hint not needed; this is just a delegator.
	if ae, ok := err.(*apicommands.AuthzError); ok {
		if p, ok := target.(**apicommands.AuthzError); ok {
			*p = ae
			return true
		}
	}
	return false
}

// Silence the plans import in case a future matrix row consumes it.
var _ = plans.Plan{}
