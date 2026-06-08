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
	if !scope.IsOverhead() {
		if cap, spent, warnPct, ok := e.repo.AgentBudget(ctx, scope.TenantID, scope.AgentID); ok && cap > 0 {
			if spent >= cap {
				return ErrBudgetExceeded
			}
			e.maybeWarn("agent:"+scope.AgentID, spent, cap, warnPct)
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
	if spent >= threshold && e.warn != nil {
		e.warn(scopeKey)
	}
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
		WHERE tenant_id = $1 AND agent_id IS NULL AND project_id IS NULL
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
