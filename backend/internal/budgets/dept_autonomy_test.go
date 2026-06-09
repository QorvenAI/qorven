package budgets

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func autonomyTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("DB not reachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestDepartmentAutonomy_DefaultsAndSet(t *testing.T) {
	pool := autonomyTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000d7"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM departments WHERE tenant_id=$1", tenant) })

	engID, err := s.CreateDepartment(ctx, tenant, "Engineering", "")
	if err != nil {
		t.Fatalf("create eng: %v", err)
	}
	pol, thr, err := s.GetDepartmentAutonomy(ctx, tenant, engID)
	if err != nil {
		t.Fatalf("get eng autonomy: %v", err)
	}
	if pol != PolicyBoth {
		t.Errorf("Engineering default policy: want both, got %q", pol)
	}
	if thr != 25_000_000 {
		t.Errorf("default threshold: want 25000000, got %d", thr)
	}

	mktID, _ := s.CreateDepartment(ctx, tenant, "Marketing", "")
	pol2, _, _ := s.GetDepartmentAutonomy(ctx, tenant, mktID)
	if pol2 != PolicyAuto {
		t.Errorf("Marketing default policy: want auto_within_budget, got %q", pol2)
	}

	if err := s.SetDepartmentAutonomy(ctx, tenant, mktID, PolicyUserApproval, 50_000_000); err != nil {
		t.Fatalf("set: %v", err)
	}
	pol3, thr3, _ := s.GetDepartmentAutonomy(ctx, tenant, mktID)
	if pol3 != PolicyUserApproval || thr3 != 50_000_000 {
		t.Errorf("after set: want user_approval/50000000, got %q/%d", pol3, thr3)
	}
}

func TestProjectDepartmentFeasibility_RunsAndReturnsBreakdown(t *testing.T) {
	pool := autonomyTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000d8"
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM gateway_budgets WHERE tenant_id=$1", tenant)
		pool.Exec(ctx, "DELETE FROM departments WHERE tenant_id=$1", tenant)
	})

	if err := s.SetBudget(ctx, tenant, BudgetScope{Scope: "tenant", MonthlyUSD: 1000, FundingMode: "prepaid_fixed"}); err != nil {
		t.Fatalf("set tenant budget: %v", err)
	}
	deptID, _ := s.CreateDepartment(ctx, tenant, "Marketing", "")

	f, err := s.ProjectDepartmentFeasibility(ctx, tenant, deptID, 100_000_000)
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if f.BudgetUUSD != 1000_000_000 {
		t.Errorf("budget: want 1000000000 µUSD, got %d", f.BudgetUUSD)
	}
	if f.PlanUUSD != 100_000_000 {
		t.Errorf("plan: want 100000000, got %d", f.PlanUUSD)
	}
	if !f.Fits {
		t.Errorf("a $100 plan should fit a $1000 budget with no spend, got available=%d", f.AvailableUUSD)
	}
}

func TestGetDepartmentAutonomy_MissingDeptDefaults(t *testing.T) {
	pool := autonomyTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	// A nonexistent department → defaults, NO error.
	pol, thr, err := s.GetDepartmentAutonomy(ctx, "00000000-0000-0000-0000-0000000000d9", "00000000-0000-0000-0000-0000000000ff")
	if err != nil {
		t.Fatalf("missing dept should not error, got %v", err)
	}
	if pol != PolicyAuto || thr != 25_000_000 {
		t.Errorf("missing dept defaults: want auto/25000000, got %q/%d", pol, thr)
	}
}

func TestSetDepartmentAutonomy_MissingDeptErrors(t *testing.T) {
	pool := autonomyTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	err := s.SetDepartmentAutonomy(ctx, "00000000-0000-0000-0000-0000000000d9", "00000000-0000-0000-0000-0000000000fe", PolicyBoth, 1)
	if err == nil {
		t.Fatalf("setting a nonexistent department should error")
	}
}
