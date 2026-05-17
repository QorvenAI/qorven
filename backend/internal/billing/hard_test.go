// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package billing

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// hard_test.go — Billing accuracy and budget enforcement tests.

func billingPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestHard_Billing_LogCost(t *testing.T) {
	pool := billingPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Log a cost event
	store.LogCost(ctx, tenant, agentID, "test-session", "deepseek", "deepseek-chat", 100, 50, 0.15, "")

	// Verify it was recorded
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM cost_events WHERE agent_id = $1 AND session_id = 'test-session'", agentID).Scan(&count)
	if count == 0 { t.Error("cost event not recorded") }

	// Clean up
	pool.Exec(ctx, "DELETE FROM cost_events WHERE session_id = 'test-session'")
	t.Log("billing: cost event logged ✓")
}

func TestHard_Billing_GetAgentCosts(t *testing.T) {
	pool := billingPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	costs, err := store.GetAgentCosts(ctx, tenant, time.Now().Add(-24*time.Hour))
	if err != nil { t.Fatal(err) }

	for _, c := range costs {
		if c.AgentID == "" { t.Error("empty agent ID in cost summary") }
		if c.TotalCost < 0 { t.Errorf("negative cost for agent %s", c.AgentID) }
	}
	t.Logf("billing: %d agents with costs ✓", len(costs))
}

func TestHard_Billing_EnforceBudget_NoBudget(t *testing.T) {
	pool := billingPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents WHERE credit_budget_cents = 0 OR credit_budget_cents IS NULL LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents without budget") }

	// Agent with no budget should always pass
	err := store.EnforceBudget(ctx, agentID)
	if err != nil { t.Errorf("agent without budget should pass: %v", err) }
	t.Log("budget: no-budget agent passes ✓")
}

func TestHard_Billing_RecentEvents(t *testing.T) {
	pool := billingPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	events, err := store.RecentEvents(ctx, tenant, 10)
	if err != nil { t.Fatal(err) }

	for _, e := range events {
		if e.InputTokens < 0 { t.Error("negative input tokens") }
		if e.OutputTokens < 0 { t.Error("negative output tokens") }
		if e.CostUSD < 0 { t.Error("negative cost") }
	}
	t.Logf("billing: %d recent events ✓", len(events))
}
