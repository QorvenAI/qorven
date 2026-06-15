// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func TestCarvedAllocation_RejectsOverParent(t *testing.T) {
	err := validateCarved(100.0, 80.0, 30.0) // 80+30=110 > 100
	if err == nil {
		t.Fatal("carved over-allocation must be rejected")
	}
}

func TestCarvedAllocation_AllowsWithinParent(t *testing.T) {
	if err := validateCarved(100.0, 80.0, 20.0); err != nil { // exactly 100
		t.Fatalf("within parent must be allowed, got %v", err)
	}
}

func TestFreshAllocation_NotDrawnFromParent(t *testing.T) {
	if err := validateAllocation("fresh", 100.0, 80.0, 999.0); err != nil {
		t.Fatalf("fresh must skip draw-down, got %v", err)
	}
}

func TestCarvedAllocation_ZeroParentMeansUnlimited(t *testing.T) {
	if err := validateAllocation("carved", 0, 0, 500.0); err != nil {
		t.Fatalf("unlimited parent must allow any carved child, got %v", err)
	}
}

func TestReconcile_DeclaredIsBinding(t *testing.T) {
	r := reconcile(50_000_000, 200_000_000) // declared 50, providers 200
	if r.EffectiveUUSD != 50_000_000 || r.Binding != "declared" {
		t.Fatalf("declared should bind: %+v", r)
	}
	if len(r.Warnings) != 0 {
		t.Fatalf("no warning when declared <= providers, got %v", r.Warnings)
	}
}

func TestReconcile_ProvidersAreBinding_Warns(t *testing.T) {
	r := reconcile(200_000_000, 50_000_000) // declared 200, providers only 50
	if r.EffectiveUUSD != 50_000_000 || r.Binding != "providers" {
		t.Fatalf("providers should bind: %+v", r)
	}
	if len(r.Warnings) == 0 {
		t.Fatalf("expected a warning when declared exceeds provider allowance")
	}
}

func TestReconcile_EqualPrefersDeclared(t *testing.T) {
	r := reconcile(50_000_000, 50_000_000)
	if r.EffectiveUUSD != 50_000_000 || r.Binding != "declared" {
		t.Fatalf("equal should report declared binding: %+v", r)
	}
}

// --- SetBudget cap-value guard tests ---
// The guard fires before any DB call, so a nil-pool Store is sufficient for
// the rejection cases. AllowsDeliberateZero requires a live DB write and uses
// the real pool (skipped when the DB is unavailable).

func TestSetBudget_RejectsSilentZero(t *testing.T) {
	s := &Store{} // nil pool — guard fires before any DB call
	err := s.SetBudget(context.Background(), "tenant-x",
		BudgetScope{Scope: "agent", ScopeID: "00000000-0000-0000-0000-000000000001", MonthlyUSD: 0})
	if err == nil {
		t.Fatal("zero cap without AllowZero must be rejected")
	}
	if !errors.Is(err, ErrInvalidBudget) {
		t.Fatalf("expected ErrInvalidBudget, got %v", err)
	}
}

func TestSetBudget_RejectsNegative(t *testing.T) {
	s := &Store{} // nil pool — guard fires before any DB call
	err := s.SetBudget(context.Background(), "tenant-x",
		BudgetScope{Scope: "agent", ScopeID: "00000000-0000-0000-0000-000000000001", MonthlyUSD: -5})
	if err == nil {
		t.Fatal("negative cap must be rejected")
	}
	if !errors.Is(err, ErrInvalidBudget) {
		t.Fatalf("expected ErrInvalidBudget, got %v", err)
	}
}

func TestSetBudget_AllowsDeliberateZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("DB not reachable: %v", err)
	}
	defer pool.Close()
	s := NewStore(pool)
	const tenant = "00000000-0000-0000-0000-000000000099"
	const agentID = "00000000-0000-0000-0000-000000000098"
	t.Cleanup(func() {
		pool.Exec(context.Background(),
			"DELETE FROM gateway_budgets WHERE tenant_id=$1", tenant)
	})
	if err := s.SetBudget(context.Background(), tenant, BudgetScope{
		Scope:          "agent",
		ScopeID:        agentID,
		MonthlyUSD:     0,
		AllocationMode: "fresh", // fresh mode doesn't require a parent scope
		AllowZero:      true,
	}); err != nil {
		t.Fatalf("deliberate zero (AllowZero=true) should be accepted: %v", err)
	}
}
