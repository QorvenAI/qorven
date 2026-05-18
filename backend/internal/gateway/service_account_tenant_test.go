// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
)

// Phase 4 Day 1 — Gap #5 regression matrix for tenant-scoped service
// accounts. Parallel to TestAuthorize_Matrix_{Single,Multi}Tenant for
// human users; this file proves the SA boundary holds with the same
// level of rigor.
//
// Terminology for the matrix columns:
//   • "global-SA"      — tenant_id IS NULL (system/orchestrator/qoros
//                         class). Blanket-passes in both modes for the
//                         infra use cases the code was originally
//                         built for.
//   • "tenant-A-SA"    — SA bound to tenant A. Must bypass for tenant A
//                         resources only.
//   • "session-A"      — a session in tenant A with a user-owner.
//   • "session-B"      — a session in tenant B (different tenant than
//                         the SA's binding).
//
// Expected outcomes:
//
//   ┌──────────────────┬────────────────┬────────────────────────────┐
//   │ mode             │ actor          │ target     → outcome       │
//   ├──────────────────┼────────────────┼────────────────────────────┤
//   │ single-tenant    │ global-SA      │ session-A → permit         │
//   │ single-tenant    │ tenant-A-SA    │ session-A → permit         │
//   │ single-tenant    │ tenant-A-SA    │ session-B → permit         │ (tenant binding informational)
//   │ multi-tenant     │ global-SA      │ session-A → permit         │ (legacy infra)
//   │ multi-tenant     │ global-SA      │ session-B → permit         │ (legacy infra)
//   │ multi-tenant     │ tenant-A-SA    │ session-A → permit         │
//   │ multi-tenant     │ tenant-A-SA    │ session-B → DENY           │ (cross_tenant_sa_denied)
//   │ multi-tenant     │ tenant-A-SA    │ session-less → DENY        │ (sa_tenant_scope_required)
//   └──────────────────┴────────────────┴────────────────────────────┘

func TestServiceAccount_TenantIsolation_MultiTenant(t *testing.T) {
	gw, pool, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})
	tenantB := isolateSecondTenant(t, pool)

	// Owners for session-A and session-B.
	ownerA := seedUser(t, gw, tenantA, "user")
	ownerB := seedUser(t, gw, tenantB, "user")
	sessA := seedOwnedSession(t, gw, tenantA, ownerA.ID)
	sessB := seedOwnedSession(t, gw, tenantB, ownerB.ID)

	// Seed the two flavors of SA. AddGlobal is the boundary-crossing
	// variant — used for the global-SA test fixture specifically to
	// prove the legacy infra-blanket-pass still works. Every other SA
	// in new code uses tenant-scoped Add.
	globalSA := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.AddGlobal(context.Background(),
		globalSA.ID, "admin", "global infra SA", "test"); err != nil {
		t.Fatalf("add global SA: %v", err)
	}

	tenantSA := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.Add(context.Background(), serviceaccounts.AddInput{
		ID: tenantSA.ID, Role: "service", Description: "tenant-A SA",
		CreatedBy: "test",
		TenantID:  tenantA,
	}); err != nil {
		t.Fatalf("add tenant-A SA: %v", err)
	}
	gw.serviceAccounts.Invalidate()

	gw.cfg.Auth.Token = "force-strict-auth"

	cases := []struct {
		name     string
		actor    *auth.User
		target   string // session id; "" = session-less
		wantCode string // "" = permit
	}{
		{"global-SA → session-A permits", globalSA, sessA.sessionID, ""},
		{"global-SA → session-B permits (legacy infra)", globalSA, sessB.sessionID, ""},
		{"tenant-A-SA → session-A permits", tenantSA, sessA.sessionID, ""},
		{"tenant-A-SA → session-B DENIED", tenantSA, sessB.sessionID, "cross_tenant_sa_denied"},
		{"tenant-A-SA → session-less DENIED", tenantSA, "", "sa_tenant_scope_required"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := withUser(context.Background(), tc.actor)
			err := gw.authorize(ctx, AuthScope{SessionID: tc.target})
			assertAuthz(t, err, tc.wantCode)
		})
	}
}

// TestServiceAccount_TenantIsolation_SingleTenant locks in the
// byte-for-byte single-tenant contract: SA tenant binding is
// INFORMATIONAL only. A tenant-A-SA must still pass for every
// resource, because in single-tenant there is one tenant by definition.
// Breaking this test means the Phase 4 work regressed the Phase 3
// ruling's "single-tenant unchanged" mandate.
func TestServiceAccount_TenantIsolation_SingleTenant(t *testing.T) {
	gw, pool, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeSingleTenant,
	})
	tenantB := isolateSecondTenant(t, pool)

	ownerA := seedUser(t, gw, tenantA, "user")
	ownerB := seedUser(t, gw, tenantB, "user")
	sessA := seedOwnedSession(t, gw, tenantA, ownerA.ID)
	sessB := seedOwnedSession(t, gw, tenantB, ownerB.ID)

	globalSA := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.AddGlobal(context.Background(),
		globalSA.ID, "admin", "global SA single-tenant", "test"); err != nil {
		t.Fatalf("add global SA: %v", err)
	}
	tenantSA := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.Add(context.Background(), serviceaccounts.AddInput{
		ID: tenantSA.ID, Role: "service", Description: "tenant-A SA single-tenant",
		CreatedBy: "test", TenantID: tenantA,
	}); err != nil {
		t.Fatalf("add tenant-A SA: %v", err)
	}
	gw.serviceAccounts.Invalidate()

	gw.cfg.Auth.Token = "force-strict-auth"

	// Every row permits in single-tenant — that's the contract.
	cases := []struct {
		name  string
		actor *auth.User
		sess  string
	}{
		{"global-SA → A permits", globalSA, sessA.sessionID},
		{"global-SA → B permits", globalSA, sessB.sessionID},
		{"tenant-A-SA → A permits", tenantSA, sessA.sessionID},
		{"tenant-A-SA → B permits (single-tenant: binding informational)", tenantSA, sessB.sessionID},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx := withUser(context.Background(), tc.actor)
			if err := gw.authorize(ctx, AuthScope{SessionID: tc.sess}); err != nil {
				t.Fatalf("single-tenant must permit: got %v", err)
			}
		})
	}
}

// TestServiceAccount_CacheReflectsTenantBinding is the lower-level
// assertion that the store's cache actually carries the tenant field.
// A cache bug (e.g. silently dropping TenantID in a future refactor)
// would make the matrix above pass for the wrong reason — we'd permit
// because the SA *looks* global even though the DB says tenant-bound.
func TestServiceAccount_CacheReflectsTenantBinding(t *testing.T) {
	gw, _, tenantA := newMinimalGateway(t, MinimalGatewayOpts{
		DeploymentMode: deployment.ModeMultiTenant,
	})

	bound := seedUser(t, gw, tenantA, "user")
	if _, err := gw.serviceAccounts.Add(context.Background(), serviceaccounts.AddInput{
		ID: bound.ID, Role: "service", Description: "cache-check",
		CreatedBy: "test", TenantID: tenantA,
	}); err != nil {
		t.Fatalf("add bound SA: %v", err)
	}
	gw.serviceAccounts.Invalidate()

	_, cacheTenant, ok := gw.serviceAccounts.LookupDetailed(context.Background(), bound.ID)
	if !ok {
		t.Fatalf("SA not found in cache")
	}
	if cacheTenant != tenantA {
		t.Fatalf("cache tenant=%q, want %q — the cache is dropping tenant_id", cacheTenant, tenantA)
	}
}

