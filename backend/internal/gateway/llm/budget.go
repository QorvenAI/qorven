// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// agentBudget is the cached per-agent spend state.
type agentBudget struct {
	MonthlyLimitUSD float64
	DailyLimitUSD   float64
	SpentThisMonth  float64
	SpentToday      float64
	LoadedAt        time.Time
}

// BudgetEngine enforces per-agent spend caps against the gateway_budgets
// and gateway_spend tables written by the CostLedger.
type BudgetEngine struct {
	db    *pgxpool.Pool
	mu    sync.Mutex
	cache map[string]*agentBudget // agentID → cached budget (60s TTL)
}

// NewBudgetEngine creates a BudgetEngine backed by the given pool.
func NewBudgetEngine(db *pgxpool.Pool) *BudgetEngine {
	return &BudgetEngine{
		db:    db,
		cache: make(map[string]*agentBudget),
	}
}

// Check returns ErrBudgetExceeded if the agent has hit its monthly or
// daily cap. A missing budget row means "no cap" — returns nil.
func (e *BudgetEngine) Check(ctx context.Context, req GatewayRequest) error {
	if req.AgentID == "" || e.db == nil {
		return nil
	}
	b, err := e.load(ctx, req.TenantID, req.AgentID)
	if err != nil || b == nil {
		return nil // no budget row = uncapped
	}
	if b.MonthlyLimitUSD > 0 && b.SpentThisMonth >= b.MonthlyLimitUSD {
		return ErrBudgetExceeded
	}
	if b.DailyLimitUSD > 0 && b.SpentToday >= b.DailyLimitUSD {
		return ErrBudgetExceeded
	}
	return nil
}

// AddSpend credits costUSD to the agent's current-day spend and invalidates
// the cache so the next Check reads fresh data. Also upserts into org_daily_spend
// so the org finance dashboard has per-agent cost history.
func (e *BudgetEngine) AddSpend(ctx context.Context, tenantID, agentID string, costUSD float64, tokensIn, tokensOut int) {
	if agentID == "" || e.db == nil {
		return
	}
	e.db.Exec(ctx, `
		INSERT INTO gateway_spend (tenant_id, agent_id, cost_usd, period)
		VALUES ($1, $2, $3, CURRENT_DATE)
		ON CONFLICT (tenant_id, agent_id, period)
		DO UPDATE SET cost_usd = gateway_spend.cost_usd + $3
	`, tenantID, agentID, costUSD)

	// Upsert org_daily_spend for CFO/CHRO finance visibility.
	e.db.Exec(ctx, `
		INSERT INTO org_daily_spend (tenant_id, agent_id, cost_usd, tokens_in, tokens_out, date, org_role)
		SELECT $1, $2, $3, $4, $5, CURRENT_DATE, COALESCE(org_role,'')
		FROM agents WHERE id = $2
		ON CONFLICT (tenant_id, agent_id, date)
		DO UPDATE SET
		    cost_usd   = org_daily_spend.cost_usd   + $3,
		    tokens_in  = org_daily_spend.tokens_in  + $4,
		    tokens_out = org_daily_spend.tokens_out + $5
	`, tenantID, agentID, costUSD, tokensIn, tokensOut)

	// Invalidate cache entry so the next Check re-reads from DB.
	e.mu.Lock()
	delete(e.cache, agentID)
	e.mu.Unlock()
}

// load returns the cached budget or fetches from DB. Returns nil, nil when
// no budget row exists for the agent.
func (e *BudgetEngine) load(ctx context.Context, tenantID, agentID string) (*agentBudget, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if b, ok := e.cache[agentID]; ok && time.Since(b.LoadedAt) < 60*time.Second {
		return b, nil
	}

	var b agentBudget
	var monthlyLimit, dailyLimit *float64

	// Load configured caps.
	err := e.db.QueryRow(ctx, `
		SELECT monthly_usd, daily_usd
		FROM   gateway_budgets
		WHERE  tenant_id = $1
		  AND  agent_id  = $2
		LIMIT 1
	`, tenantID, agentID).Scan(&monthlyLimit, &dailyLimit)
	if err != nil {
		return nil, nil // no row = uncapped
	}
	if monthlyLimit != nil {
		b.MonthlyLimitUSD = *monthlyLimit
	}
	if dailyLimit != nil {
		b.DailyLimitUSD = *dailyLimit
	}

	// Load current spend.
	var monthSpend, daySpend *float64
	_ = e.db.QueryRow(ctx, `
		SELECT
		  SUM(cost_usd) FILTER (WHERE period >= date_trunc('month', CURRENT_DATE)) AS month_spend,
		  SUM(cost_usd) FILTER (WHERE period = CURRENT_DATE)                       AS day_spend
		FROM  gateway_spend
		WHERE tenant_id = $1
		  AND agent_id  = $2
	`, tenantID, agentID).Scan(&monthSpend, &daySpend)

	if monthSpend != nil {
		b.SpentThisMonth = *monthSpend
	}
	if daySpend != nil {
		b.SpentToday = *daySpend
	}

	b.LoadedAt = time.Now()
	e.cache[agentID] = &b
	return &b, nil
}
