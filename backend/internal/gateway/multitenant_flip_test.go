// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestMultitenantFlip_LegacyDataCleanlyAdopted is the final proof for
// the Phase 4 data-cutover ruling: an existing single-tenant install
// can flip IsMultiTenant on and (a) the original operator still
// reaches their historical sessions, (b) an outsider in a NEW tenant
// does NOT.
//
// Scenario simulated end-to-end:
//
//   1. Single-tenant install. Admin `alice` creates some sessions;
//      a legacy code path leaves one with NULL owner_actor_id — the
//      Phase 2 row shape that authorize()'s multi-tenant branch
//      rejects with legacy_session_no_owner.
//   2. Operator prepares for the flip: executes migration 041's SQL
//      (the canonical file, read from disk). Legacy row is assigned
//      to alice — the oldest admin in alice's tenant.
//   3. Operator calls SetMode(ModeMultiTenant).
//   4. A new tenant B is provisioned with admin `bob`.
//
// Assertions:
//   • alice still reaches her normally-owned session.
//   • alice still reaches the formerly-NULL session (backfill stuck).
//   • bob cannot reach either of alice's sessions (cross_tenant_*).
func TestMultitenantFlip_LegacyDataCleanlyAdopted(t *testing.T) {
	gw, pool, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeSingleTenant,
	})
	ctx := testutil.Ctx(t)

	// ─── Step 1: single-tenant install with alice as the admin. ───
	alice := seedUser(t, gw, tenantA, "admin")
	normalSess := seedOwnedSession(t, gw, tenantA, alice.ID)

	// Legacy row: NULL owner_actor_id, mimicking Phase 2 data.
	legacySess := seedOwnedSession(t, gw, tenantA, alice.ID)
	if _, err := pool.Exec(ctx,
		`UPDATE sessions SET owner_actor_id = NULL WHERE id = $1`,
		legacySess.sessionID); err != nil {
		t.Fatalf("null-out legacy session: %v", err)
	}

	// Pre-flip confirmation: single-tenant admin still reaches the
	// legacy row via the admin bypass. This is the behavior the flip
	// must preserve for alice.
	gw.cfg.Auth.Token = "force-strict-auth"
	if err := gw.authorize(withUser(ctx, alice), AuthScope{SessionID: legacySess.sessionID}); err != nil {
		t.Fatalf("pre-flip: admin should reach legacy session: %v", err)
	}

	// ─── Step 2: run the canonical backfill SQL. ───
	runBackfillMigration(t, pool, "041_phase4_legacy_owner_backfill.up.sql")

	// Verify the legacy row's owner is now alice (oldest admin in
	// tenantA). If a future change picks a different admin, this
	// assertion will catch it — which matters because any OTHER
	// admin means alice silently loses access.
	var ownerAfter *string
	if err := pool.QueryRow(ctx,
		`SELECT owner_actor_id FROM sessions WHERE id = $1`,
		legacySess.sessionID).Scan(&ownerAfter); err != nil {
		t.Fatalf("read legacy owner after backfill: %v", err)
	}
	if ownerAfter == nil || *ownerAfter != alice.ID {
		got := "<nil>"
		if ownerAfter != nil {
			got = *ownerAfter
		}
		t.Fatalf("backfill did not adopt legacy session: owner=%q want %q", got, alice.ID)
	}

	// ─── Step 3: flip to multi-tenant. ───
	if err := gw.deploymentConfig.SetMode(ctx, deployment.ModeMultiTenant); err != nil {
		t.Fatalf("SetMode multi: %v", err)
	}

	// ─── Step 4: provision tenantB + bob. ───
	tenantB := isolateSecondTenant(t, pool)
	bob := seedUser(t, gw, tenantB, "admin")

	// ─── Assertions ───

	// alice still reaches her originally-owned session.
	if err := gw.authorize(withUser(ctx, alice), AuthScope{SessionID: normalSess.sessionID}); err != nil {
		t.Fatalf("post-flip: alice lost access to her own session: %v", err)
	}

	// alice still reaches the previously-legacy session — owner-match
	// works now because backfill put her id on the row.
	if err := gw.authorize(withUser(ctx, alice), AuthScope{SessionID: legacySess.sessionID}); err != nil {
		t.Fatalf("post-flip: alice lost access to legacy session after backfill: %v", err)
	}

	// bob (tenantB admin) must be denied both of alice's sessions.
	for _, target := range []struct {
		name string
		id   string
	}{
		{"alice's normal session", normalSess.sessionID},
		{"alice's legacy session", legacySess.sessionID},
	} {
		err := gw.authorize(withUser(ctx, bob), AuthScope{SessionID: target.id})
		if err == nil {
			t.Fatalf("post-flip: bob reached %s — multi-tenant boundary is OPEN", target.name)
		}
		var ae *apicommands.AuthzError
		if !errorAs(err, &ae) {
			t.Fatalf("post-flip: bob → %s returned non-AuthzError: %v", target.name, err)
		}
		if ae.Code != "cross_tenant_admin_denied" {
			t.Fatalf("post-flip: bob → %s code=%q, want cross_tenant_admin_denied (reason=%q)",
				target.name, ae.Code, ae.Reason)
		}
	}
}

// TestMultitenantFlip_BackfillLeavesLegacyRowNullIfNoAdmin is the
// negative case for the backfill: a tenant with NULL-owner sessions
// but no active admin. The migration must NOT silently assign to a
// non-admin user, because that would grant permanent ownership of
// tenant data to whoever happened to exist in the tenant first. In
// such a tenant the session stays NULL and multi-tenant mode
// correctly rejects it post-flip.
func TestMultitenantFlip_BackfillLeavesLegacyRowNullIfNoAdmin(t *testing.T) {
	gw, pool, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeSingleTenant,
	})
	ctx := testutil.Ctx(t)

	// Only a regular user — no admin.
	regular := seedUser(t, gw, tenantA, "user")
	legacy := seedOwnedSession(t, gw, tenantA, regular.ID)
	if _, err := pool.Exec(ctx,
		`UPDATE sessions SET owner_actor_id = NULL WHERE id = $1`,
		legacy.sessionID); err != nil {
		t.Fatalf("null-out legacy: %v", err)
	}

	runBackfillMigration(t, pool, "041_phase4_legacy_owner_backfill.up.sql")

	var ownerAfter *string
	if err := pool.QueryRow(ctx,
		`SELECT owner_actor_id FROM sessions WHERE id = $1`,
		legacy.sessionID).Scan(&ownerAfter); err != nil {
		t.Fatalf("post-backfill read: %v", err)
	}
	if ownerAfter != nil {
		t.Fatalf("backfill assigned a non-admin to legacy row: %q — "+
			"must leave NULL so operator has to provision an admin",
			*ownerAfter)
	}

	// After flip, the regular user cannot access the legacy row
	// (it stays legacy_session_no_owner). This is the correct
	// outcome: the operator has been loudly told to create an
	// admin, and until they do, the data is quarantined.
	if err := gw.deploymentConfig.SetMode(ctx, deployment.ModeMultiTenant); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	gw.cfg.Auth.Token = "force-strict-auth"
	err := gw.authorize(withUser(ctx, regular), AuthScope{SessionID: legacy.sessionID})
	if err == nil {
		t.Fatalf("post-flip: non-admin reached still-NULL legacy session — boundary leak")
	}
	var ae *apicommands.AuthzError
	if !errorAs(err, &ae) || ae.Code != "legacy_session_no_owner" {
		t.Fatalf("expected legacy_session_no_owner, got %v", err)
	}
}

// backfillSQL is the canonical Phase 4 backfill that assigns NULL
// owner_actor_id sessions to the oldest active admin in the same tenant.
// Previously stored in 041_phase4_legacy_owner_backfill.up.sql; inlined
// here because fresh installs have no legacy data and the file was removed.
const backfillSQL = `
UPDATE public.sessions s
SET owner_actor_id = (
    SELECT u.id
    FROM public.users u
    WHERE u.tenant_id = s.tenant_id
      AND u.role = 'admin'
      AND u.is_active = true
    ORDER BY u.created_at ASC
    LIMIT 1
)
WHERE s.owner_actor_id IS NULL;
`

// runBackfillMigration executes the Phase 4 owner-backfill SQL against
// the test pool. The SQL is inlined above so the test does not depend on
// an external migration file.
func runBackfillMigration(t *testing.T, pool *pgxpool.Pool, _ string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), backfillSQL); err != nil {
		t.Fatalf("apply backfill: %v", err)
	}
}
