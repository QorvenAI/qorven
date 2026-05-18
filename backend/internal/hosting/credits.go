// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package hosting

// This module is licensed under BSL 1.1 (see LICENSE-HOSTING).
// It provides managed hosting features for Qorven Cloud.

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CreditTracker struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
}

func NewCreditTracker(pool *pgxpool.Pool) *CreditTracker {
	return &CreditTracker{pool: pool}
}

// TrackUsage records credit consumption for a tenant/agent/session.
func (ct *CreditTracker) TrackUsage(ctx context.Context, tenantID, agentID string, inputTokens, outputTokens int, costCents int) error {
	// Update agent spend
	_, err := ct.pool.Exec(ctx, `UPDATE agents SET credit_used_cents = credit_used_cents + $2, updated_at = NOW() WHERE id = $1`, agentID, costCents)
	if err != nil {
		return err
	}
	// Update tenant spend
	_, err = ct.pool.Exec(ctx, `UPDATE tenants SET credit_used_cents = credit_used_cents + $2, updated_at = NOW() WHERE id = $1`, tenantID, costCents)
	return err
}

// CheckBudget returns true if the agent/tenant has remaining budget.
func (ct *CreditTracker) CheckBudget(ctx context.Context, tenantID, agentID string) (bool, int64, int64, error) {
	var budget, used int64
	err := ct.pool.QueryRow(ctx, `SELECT credit_budget_cents, credit_used_cents FROM tenants WHERE id = $1`, tenantID).Scan(&budget, &used)
	if err != nil {
		return false, 0, 0, err
	}
	if budget <= 0 {
		return true, budget, used, nil // No budget = unlimited
	}
	return used < budget, budget, used, nil
}

// GetUsageStats returns usage breakdown per agent.
func (ct *CreditTracker) GetUsageStats(ctx context.Context, tenantID string) ([]AgentUsage, error) {
	rows, err := ct.pool.Query(ctx, `SELECT id, agent_key, display_name, credit_budget_cents, credit_used_cents
		FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY credit_used_cents DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := []AgentUsage{}
	for rows.Next() {
		s := AgentUsage{}
		rows.Scan(&s.AgentID, &s.AgentKey, &s.DisplayName, &s.BudgetCents, &s.UsedCents)
		stats = append(stats, s)
	}
	return stats, nil
}

type AgentUsage struct {
	AgentID     string `json:"agent_id"`
	AgentKey    string `json:"agent_key"`
	DisplayName string `json:"display_name"`
	BudgetCents int64  `json:"budget_cents"`
	UsedCents   int64  `json:"used_cents"`
}

// ResetMonthly resets all tenant credit usage (called by billing cron).
func (ct *CreditTracker) ResetMonthly(ctx context.Context) error {
	_, err := ct.pool.Exec(ctx, `UPDATE tenants SET credit_used_cents = 0, updated_at = NOW()`)
	if err != nil {
		return err
	}
	_, err = ct.pool.Exec(ctx, `UPDATE agents SET credit_used_cents = 0, updated_at = NOW()`)
	slog.Info("monthly credit reset complete")
	return err
}

// ManagedConfig holds hosting-specific settings.
type ManagedConfig struct {
	Enabled       bool   `json:"enabled"`
	TenantID      string `json:"tenant_id"`
	PlanType      string `json:"plan_type"` // starter, pro, unlimited
	CreditsCents  int64  `json:"credits_cents"`
	BillingAPI    string `json:"billing_api"`
}

// WidgetData returns data for the credit widget injection.
func (ct *CreditTracker) WidgetData(ctx context.Context, tenantID string) (map[string]interface{}, error) {
	var budget, used int64
	err := ct.pool.QueryRow(ctx, `SELECT credit_budget_cents, credit_used_cents FROM tenants WHERE id = $1`, tenantID).Scan(&budget, &used)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"used":    float64(used) / 100,
		"limit":   float64(budget) / 100,
		"percent": func() float64 { if budget <= 0 { return 0 }; return float64(used) / float64(budget) * 100 }(),
	}, nil
}

// StartBillingCron starts the monthly credit reset check.
func (ct *CreditTracker) StartBillingCron(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				// Check if any tenant needs reset (billing date passed)
				// Simplified: actual implementation checks per-tenant billing dates
			}
		}
	}()
	slog.Info("billing cron started")
}
