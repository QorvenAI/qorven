// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

// rlsTestDSN returns a DSN that connects as a NON-superuser role. RLS
// is bypassed by superusers regardless of FORCE, so the dev shortcut
// of making `qorven` a superuser (common on self-hosted boxes) makes
// RLS tests trivially pass for the wrong reason. Production MUST run
// the app under a non-superuser role — this helper enforces that
// discipline in the test suite.
//
// Phase 4 (P4-D3-A): this helper no longer SKIPS when the role is
// missing. Missing role is a CI misconfiguration, not an acceptable
// shortcut. The test fails loud so the operator fixes their workflow.
// The dev-env provisioning instructions are in backend/README.md.
//
// Precedence for the DSN:
//   1. QORVEN_APP_TEST_DSN env var (CI sets this explicitly).
//   2. Derived from QORVEN_TEST_DSN by swapping the user/password to
//      qorven_app — works on the author's local box and matches the
//      CI role name exactly.
func rlsTestDSN(t *testing.T) string {
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
	t.Fatalf("cannot derive non-superuser DSN from %q; "+
		"set QORVEN_APP_TEST_DSN to a DSN that connects as a NOSUPERUSER "+
		"NOBYPASSRLS role (CI provisions qorven_app for this purpose)", base)
	return ""
}

// TestRLS_ConnectionRoleIsRestricted is the gate that protects every
// other test in this file. If the DSN resolves to a role with
// superuser or BYPASSRLS, all the boundary checks below pass
// trivially — RLS is completely inert. Fail loud at the first test
// that runs (Go runs file tests in declaration order, and this is
// top-of-file).
func TestRLS_ConnectionRoleIsRestricted(t *testing.T) {
	enforce, err := store.NewForMultiTenant(rlsTestDSN(t))
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer enforce.Pool.Close()

	var rolsuper, rolbypassrls bool
	if err := enforce.Pool.QueryRow(context.Background(), `
        SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = CURRENT_USER
    `).Scan(&rolsuper, &rolbypassrls); err != nil {
		t.Fatalf("probe role flags: %v", err)
	}
	if rolsuper || rolbypassrls {
		t.Fatalf(
			"QORVEN_APP_TEST_DSN resolves to a role with rolsuper=%v rolbypassrls=%v; "+
				"RLS tests would pass for the wrong reason. Use a NOSUPERUSER NOBYPASSRLS role.",
			rolsuper, rolbypassrls)
	}
}

// TestRLS_EnforcesTenantBoundary_Plans is the ground-truth test for
// Phase 4 migration 040: with RLS bypass OFF and a concrete tenant
// scoped on the connection, the database ITSELF refuses to return
// another tenant's rows. This is the backstop the ruling called for:
// if the Go layer ever leaks, Postgres still says no.
//
// The test goes direct via SQL (no store.* abstractions) so what we
// exercise is exactly what a rogue query would hit.
func TestRLS_EnforcesTenantBoundary_Plans(t *testing.T) {
	// Two tenants, one plan each, seeded via the bypass pool so RLS
	// doesn't block setup.
	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	var planA, planB string
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO plans (tenant_id, title) VALUES ($1, 'rls-test-A')
        RETURNING id::text
    `, tenantA).Scan(&planA); err != nil {
		t.Fatalf("seed plan A: %v", err)
	}
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO plans (tenant_id, title) VALUES ($1, 'rls-test-B')
        RETURNING id::text
    `, tenantB).Scan(&planB); err != nil {
		t.Fatalf("seed plan B: %v", err)
	}

	// Open an RLS-enforcing pool. Note: this test shares the same DB
	// as the bypass pool above (testutil.Pool uses QORVEN_TEST_DSN);
	// the two pools just carry different AfterConnect defaults.
	enforce, err := store.NewForMultiTenant(rlsTestDSN(t))
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer enforce.Pool.Close()

	// Scope a transaction to tenantA, query for both plans by id.
	// tenantA should return exactly 1 row; the cross-tenant row
	// should be invisible even though we asked by id.
	var seenA, seenB bool
	err = enforce.WithTenantTx(ctx, tenantA, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text FROM plans WHERE id::text IN ($1, $2)`,
			planA, planB)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			if id == planA {
				seenA = true
			}
			if id == planB {
				seenB = true
			}
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("WithTenantTx(A): %v", err)
	}
	if !seenA {
		t.Fatalf("tenant A must see its own plan under RLS")
	}
	if seenB {
		t.Fatalf("tenant A saw tenant B's plan — RLS is NOT enforcing")
	}
}

// TestRLS_EnforcesTenantBoundary_Sessions repeats the boundary check
// against sessions. Two tables because sessions and plans use
// independent policy definitions; a regression could affect one
// without the other.
func TestRLS_EnforcesTenantBoundary_Sessions(t *testing.T) {
	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	// sessions.agent_id has a FK — seed minimal agent rows.
	var agentA, agentB string
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'rls-test-A', 'm')
        RETURNING id::text
    `, tenantA, "rls-a-"+testutil.TempID("a")).Scan(&agentA); err != nil {
		t.Fatalf("seed agent A: %v", err)
	}
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'rls-test-B', 'm')
        RETURNING id::text
    `, tenantB, "rls-b-"+testutil.TempID("a")).Scan(&agentB); err != nil {
		t.Fatalf("seed agent B: %v", err)
	}

	var sessA, sessB string
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO sessions (tenant_id, agent_id, session_key, user_id, channel)
        VALUES ($1, $2, $3, 'operator', 'web')
        RETURNING id::text
    `, tenantA, agentA, "rls-test-"+testutil.TempID("s")).Scan(&sessA); err != nil {
		t.Fatalf("seed session A: %v", err)
	}
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO sessions (tenant_id, agent_id, session_key, user_id, channel)
        VALUES ($1, $2, $3, 'operator', 'web')
        RETURNING id::text
    `, tenantB, agentB, "rls-test-"+testutil.TempID("s")).Scan(&sessB); err != nil {
		t.Fatalf("seed session B: %v", err)
	}

	enforce, err := store.NewForMultiTenant(rlsTestDSN(t))
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer enforce.Pool.Close()

	var seenB bool
	err = enforce.WithTenantTx(ctx, tenantA, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text FROM sessions WHERE id::text = $1`, sessB)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			seenB = true
			_ = id
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("WithTenantTx: %v", err)
	}
	if seenB {
		t.Fatalf("tenant A saw tenant B's session under RLS")
	}
}

// TestRLS_WithTenantTx_RejectsEmptyTenant guards against the obvious
// footgun — passing "" for tenantID would set app.current_tenant_id
// to '' which our helper treats as NULL. The RLS policies deny every
// row on NULL tenant, so queries would silently return empty instead
// of failing loud. We reject up front.
func TestRLS_WithTenantTx_RejectsEmptyTenant(t *testing.T) {
	enforce, err := store.NewForMultiTenant(rlsTestDSN(t))
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer enforce.Pool.Close()

	err = enforce.WithTenantTx(context.Background(), "", func(tx pgx.Tx) error {
		return nil
	})
	if err == nil {
		t.Fatalf("empty tenant must be rejected")
	}
}

// TestRLS_LegacyPoliciesHonorBypass is the Phase 5 Gap #2 tripwire:
// legacy tables (agents, tasks, memories, cron_jobs) had pre-Phase-4
// RLS policies with no bypass clause. A restricted-role connection
// without tenant scope couldn't read them even for legitimate infra
// paths (migrator, sweeper boot). Migration 042 added the bypass.
// This test proves:
//
//   1. Bypass pool (single-tenant / infra) sees rows across tenants.
//   2. Enforce pool + WithTenantTx sees ONLY the scoped tenant's rows.
//
// We use `agents` as the sentinel because it's the most-used legacy
// table and its policy was one of the blocking ones pre-042.
func TestRLS_LegacyPoliciesHonorBypass(t *testing.T) {
	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	// Seed one agent per tenant via the bypass pool.
	var agentA, agentB string
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'rls-legacy-A', 'm')
        RETURNING id::text
    `, tenantA, "legacy-a-"+testutil.TempID("a")).Scan(&agentA); err != nil {
		t.Fatalf("seed agent A: %v", err)
	}
	if err := bypassPool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'rls-legacy-B', 'm')
        RETURNING id::text
    `, tenantB, "legacy-b-"+testutil.TempID("a")).Scan(&agentB); err != nil {
		t.Fatalf("seed agent B: %v", err)
	}

	// (1) Bypass pool must see both. Without migration 042's bypass
	// clause, a restricted role on this path would error out on the
	// ::uuid cast of an unset GUC.
	var countBoth int
	if err := bypassPool.QueryRow(ctx,
		`SELECT count(*) FROM agents WHERE id::text IN ($1, $2)`,
		agentA, agentB).Scan(&countBoth); err != nil {
		t.Fatalf("bypass pool count: %v", err)
	}
	if countBoth != 2 {
		t.Fatalf("bypass pool should see both legacy-table rows, got %d", countBoth)
	}

	// (2) Enforce pool + WithTenantTx(A) must see only A's row.
	enforce, err := store.NewForMultiTenant(rlsTestDSN(t))
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer enforce.Pool.Close()

	var seenB bool
	err = enforce.WithTenantTx(ctx, tenantA, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id::text FROM agents WHERE id::text IN ($1, $2)`,
			agentA, agentB)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			if id == agentB {
				seenB = true
			}
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("WithTenantTx(A) on agents: %v", err)
	}
	if seenB {
		t.Fatalf("tenant A saw tenant B's agent under RLS — migration 042 didn't take effect on agents")
	}
}

// TestRLS_BypassPool_SeesEverything is the companion positive case:
// the default store.New pool (bypass=on) still reads across tenants,
// so single-tenant installs see no behavior change. The whole suite
// passing after migration 040 is itself this test; this one just
// makes the invariant discoverable on its own.
func TestRLS_BypassPool_SeesEverything(t *testing.T) {
	bypassPool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	var planA, planB string
	_ = bypassPool.QueryRow(ctx,
		`INSERT INTO plans (tenant_id, title) VALUES ($1, 'bypass-A') RETURNING id::text`,
		tenantA).Scan(&planA)
	_ = bypassPool.QueryRow(ctx,
		`INSERT INTO plans (tenant_id, title) VALUES ($1, 'bypass-B') RETURNING id::text`,
		tenantB).Scan(&planB)

	var count int
	if err := bypassPool.QueryRow(ctx,
		`SELECT count(*) FROM plans WHERE id::text IN ($1, $2)`,
		planA, planB).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("bypass pool should see both plans: got %d", count)
	}
}
