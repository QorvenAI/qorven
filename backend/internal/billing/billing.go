// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package billing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CostEvent struct {
	ID                int64     `json:"id"`
	TenantID          string    `json:"tenant_id"`
	AgentID           string    `json:"agent_id"`
	SessionID         string    `json:"session_id"`
	Provider          string    `json:"provider"`
	Model             string    `json:"model"`
	InputTokens       int       `json:"input_tokens"`
	OutputTokens      int       `json:"output_tokens"`
	CostUSD           float64   `json:"cost_usd"`
	ToolName          string    `json:"tool_name"`
	TraceID           string    `json:"trace_id,omitempty"`
	LatencyMS         int64     `json:"latency_ms,omitempty"`
	CacheReadTokens   int       `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens  int       `json:"cache_write_tokens,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// CostEventParams groups all parameters for LogCost.
type CostEventParams struct {
	TenantID         string
	AgentID          string
	SessionID        string
	Provider         string
	Model            string
	InputTokens      int
	OutputTokens     int
	CostUSD          float64
	ToolName         string
	TraceID          string
	LatencyMS        int64
	CacheReadTokens  int
	CacheWriteTokens int
}

type AgentCostSummary struct {
	AgentID      string  `json:"agent_id"`
	AgentName    string  `json:"agent_name"`
	TotalCost    float64 `json:"total_cost_cents"`
	TotalInput   int     `json:"total_input_tokens"`
	TotalOutput  int     `json:"total_output_tokens"`
	CallCount    int     `json:"call_count"`
	BudgetCents  int64   `json:"budget_cents"`
	UsedCents    int64   `json:"used_cents"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// LogCost records a cost event for an LLM call. costUSD is in US dollars (e.g. 0.00045).
// Deprecated: prefer LogCostP for new call sites.
func (s *Store) LogCost(ctx context.Context, tenantID, agentID, sessionID, provider, model string, inputTokens, outputTokens int, costUSD float64, toolName string) {
	s.LogCostP(ctx, CostEventParams{
		TenantID: tenantID, AgentID: agentID, SessionID: sessionID,
		Provider: provider, Model: model,
		InputTokens: inputTokens, OutputTokens: outputTokens,
		CostUSD: costUSD, ToolName: toolName,
	})
}

// LogCostP records a cost event with full observability metadata.
func (s *Store) LogCostP(ctx context.Context, p CostEventParams) {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO cost_events (tenant_id, agent_id, session_id, provider, model, input_tokens, output_tokens, cost_cents, tool_name, trace_id, latency_ms, cache_read_tokens, cache_write_tokens)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		p.TenantID, p.AgentID, p.SessionID, p.Provider, p.Model,
		p.InputTokens, p.OutputTokens, p.CostUSD*100, p.ToolName,
		p.TraceID, p.LatencyMS, p.CacheReadTokens, p.CacheWriteTokens,
	); err != nil {
		slog.Error("billing.log_cost.insert", "err", err)
	}

	if _, err := s.pool.Exec(ctx,
		"UPDATE agents SET credit_used_cents = credit_used_cents + $1 WHERE id = $2",
		int64(p.CostUSD*100), p.AgentID,
	); err != nil {
		slog.Error("billing.log_cost.update_budget", "err", err)
	}
}

// GetAgentCosts returns cost summary per agent for a tenant.
func (s *Store) GetAgentCosts(ctx context.Context, tenantID string, since time.Time) ([]AgentCostSummary, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.agent_id, COALESCE(a.display_name,''), SUM(c.cost_cents), SUM(c.input_tokens), SUM(c.output_tokens), COUNT(*),
		        COALESCE(a.credit_budget_cents,0), COALESCE(a.credit_used_cents,0)
		 FROM cost_events c LEFT JOIN agents a ON a.id = c.agent_id::uuid
		 WHERE c.tenant_id=$1 AND c.created_at >= $2
		 GROUP BY c.agent_id, a.display_name, a.credit_budget_cents, a.credit_used_cents
		 ORDER BY SUM(c.cost_cents) DESC`,
		tenantID, since)
	if err != nil { return nil, err }
	defer rows.Close()

	out := []AgentCostSummary{}
	for rows.Next() {
		var s AgentCostSummary
		rows.Scan(&s.AgentID, &s.AgentName, &s.TotalCost, &s.TotalInput, &s.TotalOutput, &s.CallCount, &s.BudgetCents, &s.UsedCents)
		out = append(out, s)
	}
	return out, nil
}

// GetTotalCost returns total cost for a tenant in a time range.
func (s *Store) GetTotalCost(ctx context.Context, tenantID string, since time.Time) (float64, int, error) {
	var total float64
	var count int
	err := s.pool.QueryRow(ctx,
		"SELECT COALESCE(SUM(cost_cents),0), COUNT(*) FROM cost_events WHERE tenant_id=$1 AND created_at >= $2",
		tenantID, since).Scan(&total, &count)
	return total, count, err
}

// CheckBudget returns true if the agent is within budget, false if exceeded.
func (s *Store) CheckBudget(ctx context.Context, agentID string) (bool, error) {
	var budget, used int64
	err := s.pool.QueryRow(ctx,
		"SELECT COALESCE(credit_budget_cents,0), COALESCE(credit_used_cents,0) FROM agents WHERE id=$1",
		agentID).Scan(&budget, &used)
	if err != nil { return true, err }
	if budget <= 0 { return true, nil } // no budget = unlimited
	return used < budget, nil
}

// EnforceBudget checks budget and returns error if exceeded.
func (s *Store) EnforceBudget(ctx context.Context, agentID string) error {
	ok, err := s.CheckBudget(ctx, agentID)
	if err != nil { return err }
	if !ok {
		return fmt.Errorf("agent budget exceeded — paused until budget is increased")
	}
	return nil
}

// RecentEvents returns recent cost events for a tenant.
func (s *Store) RecentEvents(ctx context.Context, tenantID string, limit int) ([]CostEvent, error) {
	if limit <= 0 { limit = 50 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_id, session_id, provider, model, input_tokens, output_tokens, cost_cents, tool_name,
		        COALESCE(trace_id,''), COALESCE(latency_ms,0), COALESCE(cache_read_tokens,0), COALESCE(cache_write_tokens,0), created_at
		 FROM cost_events WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`,
		tenantID, limit)
	if err != nil { return nil, err }
	defer rows.Close()

	out := []CostEvent{}
	for rows.Next() {
		var e CostEvent
		var costCents float64
		rows.Scan(&e.ID, &e.TenantID, &e.AgentID, &e.SessionID, &e.Provider, &e.Model,
			&e.InputTokens, &e.OutputTokens, &costCents, &e.ToolName,
			&e.TraceID, &e.LatencyMS, &e.CacheReadTokens, &e.CacheWriteTokens, &e.CreatedAt)
		e.CostUSD = costCents / 100
		out = append(out, e)
	}
	return out, nil
}
