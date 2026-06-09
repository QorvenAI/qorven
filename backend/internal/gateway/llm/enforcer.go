// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
)

// budgetRepo reads caps + current spend in integer µUSD. Abstracted so the
// enforcement logic is unit-testable without Postgres.
type budgetRepo interface {
	AgentBudget(ctx context.Context, tenantID, agentID string) (capUUSD, spentUUSD int64, warnPct int, ok bool)
	TenantBudget(ctx context.Context, tenantID string) (capUUSD, spentUUSD int64, warnPct int, ok bool)
	ProjectBudget(ctx context.Context, tenantID, projectID string) (capUUSD, spentUUSD int64, warnPct int, ok bool)
	DepartmentBudget(ctx context.Context, tenantID, departmentID string) (capUUSD, spentUUSD int64, warnPct int, ok bool)
	TaskBudget(ctx context.Context, tenantID, taskID string) (capUUSD, spentUUSD int64, warnPct int, ok bool)
}

// DBEnforcer is the single budget enforcement engine. It evaluates the agent
// cap and the tenant (overall/overhead) cap, both in µUSD, and blocks when
// either is met. Scope-aware: department/project caps come in a later
// subsystem by extending budgetRepo — the Check flow already evaluates
// parent scopes.
type DBEnforcer struct {
	repo budgetRepo
	warn func(scopeKey string)

	mu    sync.Mutex
	cache map[string]cachedVerdict
	ttl   time.Duration
}

type cachedVerdict struct {
	err    error
	loaded time.Time
}

// NewDBEnforcer builds the enforcer over a pgx pool. db may be nil in tests
// (then set .repo directly).
func NewDBEnforcer(db *pgxpool.Pool) *DBEnforcer {
	e := &DBEnforcer{
		cache: make(map[string]cachedVerdict),
		ttl:   60 * time.Second,
	}
	if db != nil {
		e.repo = &pgBudgetRepo{db: db}
	}
	e.warn = func(scopeKey string) {
		slog.Warn("budget.warn_threshold_reached", "scope", scopeKey)
	}
	return e
}

// Check enforces the agent cap and the tenant/overhead cap in µUSD.
func (e *DBEnforcer) Check(ctx context.Context, scope providers.MeterScope) error {
	if e.repo == nil {
		return nil
	}
	if scope.TaskID != "" {
		if cap, spent, warnPct, ok := e.repo.TaskBudget(ctx, scope.TenantID, scope.TaskID); ok && cap > 0 {
			if spent >= cap {
				return ErrBudgetExceeded
			}
			e.maybeWarn("task:"+scope.TaskID, spent, cap, warnPct)
		}
	}
	if !scope.IsOverhead() {
		if cap, spent, warnPct, ok := e.repo.AgentBudget(ctx, scope.TenantID, scope.AgentID); ok && cap > 0 {
			if spent >= cap {
				return ErrBudgetExceeded
			}
			e.maybeWarn("agent:"+scope.AgentID, spent, cap, warnPct)
		}
	}
	if scope.ProjectID != "" {
		if cap, spent, warnPct, ok := e.repo.ProjectBudget(ctx, scope.TenantID, scope.ProjectID); ok && cap > 0 {
			if spent >= cap {
				return ErrBudgetExceeded
			}
			e.maybeWarn("project:"+scope.ProjectID, spent, cap, warnPct)
		}
	}
	if scope.DepartmentID != "" {
		if cap, spent, warnPct, ok := e.repo.DepartmentBudget(ctx, scope.TenantID, scope.DepartmentID); ok && cap > 0 {
			if spent >= cap {
				return ErrBudgetExceeded
			}
			e.maybeWarn("department:"+scope.DepartmentID, spent, cap, warnPct)
		}
	}
	if cap, spent, warnPct, ok := e.repo.TenantBudget(ctx, scope.TenantID); ok && cap > 0 {
		if spent >= cap {
			return ErrBudgetExceeded
		}
		e.maybeWarn("tenant:"+scope.TenantID, spent, cap, warnPct)
	}
	return nil
}

func (e *DBEnforcer) maybeWarn(scopeKey string, spent, cap int64, warnPct int) {
	if warnPct <= 0 || warnPct >= 100 || cap <= 0 {
		return
	}
	threshold := cap * int64(warnPct) / 100
	if spent < threshold || e.warn == nil {
		return
	}
	// Dedup: warn at most once per scope per ttl window to avoid log storms
	// when an agent keeps calling after crossing the threshold.
	e.mu.Lock()
	if e.cache != nil {
		if v, ok := e.cache[scopeKey]; ok && e.ttl > 0 && time.Since(v.loaded) < e.ttl {
			e.mu.Unlock()
			return
		}
		e.cache[scopeKey] = cachedVerdict{loaded: time.Now()}
	}
	e.mu.Unlock()
	e.warn(scopeKey)
}

// pgBudgetRepo reads gateway_budgets caps and gateway_spend.cost_total_uusd.
type pgBudgetRepo struct{ db *pgxpool.Pool }

func (r *pgBudgetRepo) AgentBudget(ctx context.Context, tenantID, agentID string) (int64, int64, int, bool) {
	var monthlyUSD *float64
	var warnPct *int
	err := r.db.QueryRow(ctx, `
		SELECT monthly_usd, warn_percent
		FROM gateway_budgets
		WHERE tenant_id = $1 AND agent_id = $2
		LIMIT 1
	`, tenantID, agentID).Scan(&monthlyUSD, &warnPct)
	if err != nil || monthlyUSD == nil {
		return 0, 0, 0, false
	}
	capUUSD := int64(*monthlyUSD * float64(uusdPerUSD))
	var spent *int64
	_ = r.db.QueryRow(ctx, `
		SELECT SUM(cost_total_uusd) FILTER (WHERE period >= date_trunc('month', CURRENT_DATE))
		FROM gateway_spend WHERE tenant_id = $1 AND agent_id = $2
	`, tenantID, agentID).Scan(&spent)
	wp := 80
	if warnPct != nil {
		wp = *warnPct
	}
	if spent == nil {
		return capUUSD, 0, wp, true
	}
	return capUUSD, *spent, wp, true
}

func (r *pgBudgetRepo) TenantBudget(ctx context.Context, tenantID string) (int64, int64, int, bool) {
	var monthlyUSD *float64
	var warnPct *int
	err := r.db.QueryRow(ctx, `
		SELECT monthly_usd, warn_percent
		FROM gateway_budgets
		WHERE tenant_id = $1 AND scope = 'tenant'
		LIMIT 1
	`, tenantID).Scan(&monthlyUSD, &warnPct)
	if err != nil || monthlyUSD == nil {
		return 0, 0, 0, false
	}
	capUUSD := int64(*monthlyUSD * float64(uusdPerUSD))
	var spent *int64
	_ = r.db.QueryRow(ctx, `
		SELECT SUM(cost_total_uusd) FILTER (WHERE period >= date_trunc('month', CURRENT_DATE))
		FROM gateway_spend WHERE tenant_id = $1
	`, tenantID).Scan(&spent)
	wp := 80
	if warnPct != nil {
		wp = *warnPct
	}
	if spent == nil {
		return capUUSD, 0, wp, true
	}
	return capUUSD, *spent, wp, true
}

func (r *pgBudgetRepo) ProjectBudget(ctx context.Context, tenantID, projectID string) (int64, int64, int, bool) {
	return r.scopedBudget(ctx, tenantID, "project_id", projectID, "project")
}

func (r *pgBudgetRepo) DepartmentBudget(ctx context.Context, tenantID, departmentID string) (int64, int64, int, bool) {
	return r.scopedBudget(ctx, tenantID, "department_id", departmentID, "department")
}

// scopedBudget reads the cap for a department/project scope row and sums its
// month-to-date spend from the raw ledger (which carries department_id /
// project_id after migration 030). col is a hardcoded literal (never user
// input) so concatenating it is safe; ids are bound params.
func (r *pgBudgetRepo) scopedBudget(ctx context.Context, tenantID, col, id, scope string) (int64, int64, int, bool) {
	var monthlyUSD *float64
	var warnPct *int
	err := r.db.QueryRow(ctx,
		`SELECT monthly_usd, warn_percent FROM gateway_budgets
		 WHERE tenant_id = $1 AND scope = $2 AND `+col+` = $3 LIMIT 1`,
		tenantID, scope, id).Scan(&monthlyUSD, &warnPct)
	if err != nil || monthlyUSD == nil {
		return 0, 0, 0, false
	}
	capUUSD := int64(*monthlyUSD * float64(uusdPerUSD))
	var spent *int64
	_ = r.db.QueryRow(ctx,
		`SELECT SUM(cost_total_uusd) FILTER (WHERE created_at >= date_trunc('month', CURRENT_DATE))
		 FROM gateway_spend_raw WHERE tenant_id = $1 AND `+col+` = $2`,
		tenantID, id).Scan(&spent)
	wp := 80
	if warnPct != nil {
		wp = *warnPct
	}
	if spent == nil {
		return capUUSD, 0, wp, true
	}
	return capUUSD, *spent, wp, true
}

// TaskBudget reads tasks.budget_cents (cents → µUSD) and sums task spend from
// the raw ledger (only place carrying task_id).
func (r *pgBudgetRepo) TaskBudget(ctx context.Context, tenantID, taskID string) (int64, int64, int, bool) {
	var budgetCents *int64
	err := r.db.QueryRow(ctx,
		`SELECT NULLIF(budget_cents,0) FROM tasks WHERE id = $1 AND tenant_id = $2 LIMIT 1`,
		taskID, tenantID).Scan(&budgetCents)
	if err != nil || budgetCents == nil {
		return 0, 0, 0, false
	}
	capUUSD := *budgetCents * 10_000 // cents → µUSD (1 cent = 10,000 µUSD)
	var spent *int64
	_ = r.db.QueryRow(ctx,
		`SELECT SUM(cost_total_uusd) FROM gateway_spend_raw WHERE tenant_id = $1 AND task_id = $2`,
		tenantID, taskID).Scan(&spent)
	if spent == nil {
		return capUUSD, 0, 80, true
	}
	return capUUSD, *spent, 80, true
}
