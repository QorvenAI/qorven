// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package serviceaccounts_test

import (
	"context"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/serviceaccounts"
	"github.com/qorvenai/qorven/internal/testutil"
)

func TestServiceAccounts_AddRevokeLookup(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)

	id := "test-sa-" + testutil.TempID("sa")
	if _, err := s.AddGlobal(ctx, id, serviceaccounts.RoleService, "test service", "tester"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Lookup via cache.
	if role, ok := s.Lookup(ctx, id); !ok || role != serviceaccounts.RoleService {
		t.Fatalf("lookup: %v ok=%v", role, ok)
	}
	// Revoke.
	if err := s.Revoke(ctx, id, "tester"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := s.Lookup(ctx, id); ok {
		t.Fatalf("revoked id should not lookup")
	}
}

func TestServiceAccounts_AddIdempotent(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)

	id := "test-sa-idem-" + testutil.TempID("sa")
	if _, err := s.AddGlobal(ctx, id, serviceaccounts.RoleService, "", "t1"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := s.AddGlobal(ctx, id, serviceaccounts.RoleOrchestrator, "role change", "t2"); err != nil {
		t.Fatalf("Add idem: %v", err)
	}
	role, ok := s.Lookup(ctx, id)
	if !ok || role != serviceaccounts.RoleOrchestrator {
		t.Fatalf("role after update: %v ok=%v", role, ok)
	}
}

func TestServiceAccounts_RevokeThenReactivate(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)

	id := "test-sa-react-" + testutil.TempID("sa")
	_, _ = s.AddGlobal(ctx, id, serviceaccounts.RoleService, "", "t1")
	_ = s.Revoke(ctx, id, "t1")
	if _, err := s.AddGlobal(ctx, id, serviceaccounts.RoleService, "back", "t2"); err != nil {
		t.Fatalf("Add after revoke: %v", err)
	}
	if _, ok := s.Lookup(ctx, id); !ok {
		t.Fatalf("reactivated id must lookup")
	}
}

func TestServiceAccounts_Seeded(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)

	// Migration 037 seeded system/orchestrator/qoros.
	for _, id := range []string{"system", "orchestrator", "qoros"} {
		if !s.IsServiceAccount(ctx, id) {
			t.Errorf("seed missing: %s", id)
		}
	}
}

func TestServiceAccounts_InvalidRole(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)
	if _, err := s.AddGlobal(ctx, "x", "mayor", "", ""); err == nil {
		t.Fatalf("expected invalid role rejection")
	}
}

func TestServiceAccounts_RefreshCacheExpiry(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)
	s.RefreshInterval = 10 * time.Millisecond

	id := "sa-cache-" + testutil.TempID("sa")
	_, _ = s.AddGlobal(ctx, id, serviceaccounts.RoleService, "", "t")

	// First lookup populates cache.
	if !s.IsServiceAccount(ctx, id) {
		t.Fatalf("initial lookup failed")
	}

	// Revoke directly via pool to simulate an out-of-process admin.
	_, err := pool.Exec(ctx, `UPDATE service_accounts SET revoked_at = NOW() WHERE id = $1`, id)
	if err != nil {
		t.Fatalf("direct revoke: %v", err)
	}
	// Immediately after revoke, cache may still say active.
	// Wait past RefreshInterval + lookup → cache must rebuild.
	time.Sleep(20 * time.Millisecond)
	if s.IsServiceAccount(ctx, id) {
		t.Fatalf("cache did not refresh after interval")
	}
}

func TestServiceAccounts_Invalidate(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := serviceaccounts.NewStore(pool)
	s.RefreshInterval = time.Hour // never expire naturally

	id := "sa-inv-" + testutil.TempID("sa")
	if sa, err := s.AddGlobal(ctx, id, serviceaccounts.RoleService, "", "t"); err != nil {
		t.Fatalf("Add: %v", err)
	} else if sa == nil {
		t.Fatalf("Add returned nil account")
	}
	// Confirm via direct read so we rule out visibility glitches.
	got, err := s.Get(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("Get after Add: err=%v got=%+v", err, got)
	}
	if got.RevokedAt != nil {
		t.Fatalf("newly added account must not be revoked: %+v", got)
	}
	// Force refresh — Add already invalidates but be explicit.
	if err := s.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !s.IsServiceAccount(ctx, id) {
		t.Fatalf("initial lookup failed (id=%s)", id)
	}
	_ = s.Revoke(ctx, id, "t") // Revoke already invalidates
	if s.IsServiceAccount(ctx, id) {
		t.Fatalf("Revoke did not invalidate cache")
	}
}

// Context used only to keep unused imports honest when DB is unreachable.
var _ = context.Background()
