// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tracing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Trace struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	SessionID string    `json:"session_id"`
	Model     string    `json:"model"`
	Status    string    `json:"status"`
	InputTok  int       `json:"input_tokens"`
	OutputTok int       `json:"output_tokens"`
	CostUSD   float64   `json:"cost_usd"`
	Duration  float64   `json:"duration_ms"`
	CreatedAt time.Time `json:"created_at"`
}

type Span struct {
	ID       string  `json:"id"`
	TraceID  string  `json:"trace_id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"` // llm, tool, agent
	Duration float64 `json:"duration_ms"`
	Status   string  `json:"status"`
	Meta     string  `json:"meta"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) CreateTrace(ctx context.Context, t Trace) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO traces (agent_id, session_id, model, status, input_tokens, output_tokens, cost_usd, duration_ms, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW()) RETURNING id`,
		t.AgentID, t.SessionID, t.Model, t.Status, t.InputTok, t.OutputTok, t.CostUSD, t.Duration).Scan(&id)
	return id, err
}

func (s *Store) UpdateTrace(ctx context.Context, traceID, status string, inputTok, outputTok int, costUSD, durationMS float64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE traces SET status=$1, input_tokens=$2, output_tokens=$3, cost_usd=$4, duration_ms=$5 WHERE id=$6`,
		status, inputTok, outputTok, costUSD, durationMS, traceID)
	return err
}

func (s *Store) GetTrace(ctx context.Context, traceID string) (*Trace, error) {
	var t Trace
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, session_id, model, status, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		 FROM traces WHERE id = $1`, traceID).Scan(
		&t.ID, &t.AgentID, &t.SessionID, &t.Model, &t.Status, &t.InputTok, &t.OutputTok, &t.CostUSD, &t.Duration, &t.CreatedAt)
	if err != nil { return nil, err }
	return &t, nil
}

func (s *Store) ListTraces(ctx context.Context, agentID string, limit, offset int) ([]Trace, error) {
	if limit <= 0 { limit = 50 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, session_id, model, status, input_tokens, output_tokens, cost_usd, duration_ms, created_at
		 FROM traces WHERE agent_id = $1 OR $1 = '' ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		agentID, limit, offset)
	if err != nil { return nil, err }
	defer rows.Close()
	traces := []Trace{}
	for rows.Next() {
		var t Trace
		rows.Scan(&t.ID, &t.AgentID, &t.SessionID, &t.Model, &t.Status, &t.InputTok, &t.OutputTok, &t.CostUSD, &t.Duration, &t.CreatedAt)
		traces = append(traces, t)
	}
	return traces, nil
}

func (s *Store) CreateSpan(ctx context.Context, sp Span) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO trace_spans (trace_id, name, kind, duration_ms, status, meta) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		sp.TraceID, sp.Name, sp.Kind, sp.Duration, sp.Status, sp.Meta).Scan(&id)
	return id, err
}

func (s *Store) GetTraceSpans(ctx context.Context, traceID string) ([]Span, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, trace_id, name, kind, duration_ms, status, COALESCE(meta,'') FROM trace_spans WHERE trace_id = $1 ORDER BY id`, traceID)
	if err != nil { return nil, err }
	defer rows.Close()
	spans := []Span{}
	for rows.Next() {
		var sp Span
		rows.Scan(&sp.ID, &sp.TraceID, &sp.Name, &sp.Kind, &sp.Duration, &sp.Status, &sp.Meta)
		spans = append(spans, sp)
	}
	return spans, nil
}

func (s *Store) GetMonthlyAgentCost(ctx context.Context, agentID string) (float64, error) {
	var cost float64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM traces
		 WHERE agent_id = $1 AND created_at >= date_trunc('month', NOW())`, agentID).Scan(&cost)
	return cost, err
}

func (s *Store) GetCostSummary(ctx context.Context, tenantID string) (map[string]float64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT agent_id, COALESCE(SUM(cost_usd), 0) FROM traces
		 WHERE created_at >= date_trunc('month', NOW()) GROUP BY agent_id`)
	if err != nil { return nil, err }
	defer rows.Close()
	costs := map[string]float64{}
	for rows.Next() {
		var agentID string
		var cost float64
		rows.Scan(&agentID, &cost)
		costs[agentID] = cost
	}
	return costs, nil
}

func (s *Store) DeleteTracesOlderThan(ctx context.Context, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM traces WHERE created_at < $1`, before)
	if err != nil { return 0, err }
	return tag.RowsAffected(), nil
}
