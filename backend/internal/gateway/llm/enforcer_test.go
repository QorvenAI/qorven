// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

type fakeBudgetRepo struct {
	agentCapUUSD    int64
	agentSpentUUSD  int64
	tenantCapUUSD   int64
	tenantSpentUUSD int64
	warnPercent     int

	projectCapUUSD, projectSpentUUSD int64
	deptCapUUSD, deptSpentUUSD       int64
	taskCapUUSD, taskSpentUUSD       int64
}

func (f *fakeBudgetRepo) AgentBudget(ctx context.Context, tenantID, agentID string) (int64, int64, int, bool) {
	if f.agentCapUUSD == 0 && f.agentSpentUUSD == 0 {
		return 0, 0, 0, false
	}
	return f.agentCapUUSD, f.agentSpentUUSD, f.warnPercent, true
}
func (f *fakeBudgetRepo) TenantBudget(ctx context.Context, tenantID string) (int64, int64, int, bool) {
	if f.tenantCapUUSD == 0 {
		return 0, 0, 0, false
	}
	return f.tenantCapUUSD, f.tenantSpentUUSD, f.warnPercent, true
}

func newTestEnforcer(repo budgetRepo) *DBEnforcer {
	e := NewDBEnforcer(nil)
	e.repo = repo
	e.warn = func(scopeKey string) {}
	return e
}

func TestEnforcer_NoBudgetRow_Allows(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a"}); err != nil {
		t.Fatalf("uncapped agent should pass, got %v", err)
	}
}

func TestEnforcer_AgentUnderCap_Allows(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{agentCapUUSD: 10_000_000, agentSpentUUSD: 5_000_000})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a"}); err != nil {
		t.Fatalf("under cap should pass, got %v", err)
	}
}

func TestEnforcer_AgentAtCap_Blocks(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{agentCapUUSD: 10_000_000, agentSpentUUSD: 10_000_000})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a"}); err != ErrBudgetExceeded {
		t.Fatalf("at cap should return ErrBudgetExceeded, got %v", err)
	}
}

func TestEnforcer_TenantCapBlocks_EvenWhenAgentUnder(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{
		agentCapUUSD: 100_000_000, agentSpentUUSD: 1_000_000,
		tenantCapUUSD: 20_000_000, tenantSpentUUSD: 20_000_000,
	})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a"}); err != ErrBudgetExceeded {
		t.Fatalf("tenant cap should block even when agent is under, got %v", err)
	}
}

func TestEnforcer_Overhead_ChecksTenantOnly(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{tenantCapUUSD: 5_000_000, tenantSpentUUSD: 5_000_000})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t"}); err != ErrBudgetExceeded {
		t.Fatalf("overhead over tenant cap should block, got %v", err)
	}
}

func TestEnforcer_WarnThresholdFires(t *testing.T) {
	var warned []string
	e := newTestEnforcer(&fakeBudgetRepo{agentCapUUSD: 10_000_000, agentSpentUUSD: 8_500_000, warnPercent: 80})
	e.warn = func(scopeKey string) { warned = append(warned, scopeKey) }
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a"}); err != nil {
		t.Fatalf("85%% should still pass, got %v", err)
	}
	if len(warned) == 0 {
		t.Fatalf("expected a warn callback at 85%% of cap with warn_percent=80")
	}
}

func TestEnforcer_WarnDedupedWithinTTL(t *testing.T) {
	var warned int
	e := newTestEnforcer(&fakeBudgetRepo{agentCapUUSD: 10_000_000, agentSpentUUSD: 8_500_000, warnPercent: 80})
	e.warn = func(scopeKey string) { warned++ }
	scope := providers.MeterScope{TenantID: "t", AgentID: "a"}
	// Three checks in quick succession, all above the 80% warn threshold.
	_ = e.Check(context.Background(), scope)
	_ = e.Check(context.Background(), scope)
	_ = e.Check(context.Background(), scope)
	if warned != 1 {
		t.Fatalf("expected exactly 1 warn within TTL window, got %d", warned)
	}
}

func (f *fakeBudgetRepo) ProjectBudget(ctx context.Context, tenantID, projectID string) (int64, int64, int, bool) {
	if f.projectCapUUSD == 0 && f.projectSpentUUSD == 0 {
		return 0, 0, 0, false
	}
	return f.projectCapUUSD, f.projectSpentUUSD, f.warnPercent, true
}
func (f *fakeBudgetRepo) DepartmentBudget(ctx context.Context, tenantID, deptID string) (int64, int64, int, bool) {
	if f.deptCapUUSD == 0 && f.deptSpentUUSD == 0 {
		return 0, 0, 0, false
	}
	return f.deptCapUUSD, f.deptSpentUUSD, f.warnPercent, true
}
func (f *fakeBudgetRepo) TaskBudget(ctx context.Context, tenantID, taskID string) (int64, int64, int, bool) {
	if f.taskCapUUSD == 0 && f.taskSpentUUSD == 0 {
		return 0, 0, 0, false
	}
	return f.taskCapUUSD, f.taskSpentUUSD, f.warnPercent, true
}

func TestEnforcer_ProjectCapBlocks(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{projectCapUUSD: 5_000_000, projectSpentUUSD: 5_000_000})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a", ProjectID: "p"}); err != ErrBudgetExceeded {
		t.Fatalf("project at cap must block, got %v", err)
	}
}
func TestEnforcer_DepartmentCapBlocks(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{deptCapUUSD: 5_000_000, deptSpentUUSD: 5_000_000})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a", DepartmentID: "d"}); err != ErrBudgetExceeded {
		t.Fatalf("department at cap must block, got %v", err)
	}
}
func TestEnforcer_TaskCapBlocks(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{taskCapUUSD: 1_000_000, taskSpentUUSD: 1_000_000})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a", TaskID: "k"}); err != ErrBudgetExceeded {
		t.Fatalf("task at cap must block, got %v", err)
	}
}
func TestEnforcer_AllScopesUnderCap_Passes(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{
		agentCapUUSD: 10_000_000, agentSpentUUSD: 1_000_000,
		projectCapUUSD: 10_000_000, projectSpentUUSD: 1_000_000,
		deptCapUUSD: 10_000_000, deptSpentUUSD: 1_000_000,
		taskCapUUSD: 10_000_000, taskSpentUUSD: 1_000_000,
		tenantCapUUSD: 10_000_000, tenantSpentUUSD: 1_000_000,
	})
	s := providers.MeterScope{TenantID: "t", AgentID: "a", ProjectID: "p", DepartmentID: "d", TaskID: "k"}
	if err := e.Check(context.Background(), s); err != nil {
		t.Fatalf("all under cap must pass, got %v", err)
	}
}
func TestEnforcer_NoHierarchyIDs_OnlyAgentTenantChecked(t *testing.T) {
	e := newTestEnforcer(&fakeBudgetRepo{
		projectCapUUSD: 1, projectSpentUUSD: 1, deptCapUUSD: 1, deptSpentUUSD: 1, taskCapUUSD: 1, taskSpentUUSD: 1,
	})
	if err := e.Check(context.Background(), providers.MeterScope{TenantID: "t", AgentID: "a"}); err != nil {
		t.Fatalf("with no hierarchy ids on scope, hierarchy caps must not apply, got %v", err)
	}
}

func TestBudgetEngine_DelegatesToEnforcer(t *testing.T) {
	enf := newTestEnforcer(&fakeBudgetRepo{agentCapUUSD: 1_000_000, agentSpentUUSD: 1_000_000}) // maxed
	be := NewBudgetEngine(nil)
	be.SetEnforcer(enf)
	err := be.Check(context.Background(), GatewayRequest{TenantID: "t", AgentID: "a"})
	if err != ErrBudgetExceeded {
		t.Fatalf("BudgetEngine.Check must delegate and block, got %v", err)
	}
}
