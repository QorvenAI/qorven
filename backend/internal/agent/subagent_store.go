// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SubagentRunRecord struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	ParentID    string     `json:"parent_id"`
	AgentKey    string     `json:"agent_key"`
	Task        string     `json:"task"`
	Status      string     `json:"status"`
	Result      string     `json:"result"`
	Depth       int        `json:"depth"`
	Iterations  int        `json:"iterations"`
	ToolsUsed   []string   `json:"tools_used"`
	TokensIn    int64      `json:"tokens_in"`
	TokensOut   int64      `json:"tokens_out"`
	CostUUSD    int64      `json:"cost_uusd"`
	SessionID   string     `json:"session_id"`
	TraceID     string     `json:"trace_id"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type SubagentRunStore struct {
	db *pgxpool.Pool
}

func NewSubagentRunStore(db *pgxpool.Pool) *SubagentRunStore {
	return &SubagentRunStore{db: db}
}

func (s *SubagentRunStore) Record(ctx context.Context, rec SubagentRunRecord) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO subagent_runs (id, tenant_id, parent_id, agent_key, task, status, result, depth, iterations, tools_used, tokens_in, tokens_out, cost_uusd, session_id, trace_id, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			result = EXCLUDED.result,
			iterations = EXCLUDED.iterations,
			tools_used = EXCLUDED.tools_used,
			tokens_in = EXCLUDED.tokens_in,
			tokens_out = EXCLUDED.tokens_out,
			cost_uusd = EXCLUDED.cost_uusd,
			completed_at = EXCLUDED.completed_at
	`, rec.ID, rec.TenantID, rec.ParentID, rec.AgentKey, rec.Task, rec.Status, rec.Result, rec.Depth, rec.Iterations, rec.ToolsUsed, rec.TokensIn, rec.TokensOut, rec.CostUUSD, rec.SessionID, rec.TraceID, rec.CreatedAt, rec.CompletedAt)
	return err
}

func (s *SubagentRunStore) ListByParent(ctx context.Context, tenantID, parentID string, limit int) ([]SubagentRunRecord, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, parent_id, agent_key, task, status, result, depth, iterations, tools_used, tokens_in, tokens_out, cost_uusd, session_id, COALESCE(trace_id::text,''), created_at, completed_at
		FROM subagent_runs
		WHERE tenant_id = $1 AND parent_id = $2
		ORDER BY created_at DESC
		LIMIT $3
	`, tenantID, parentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SubagentRunRecord
	for rows.Next() {
		var r SubagentRunRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ParentID, &r.AgentKey, &r.Task, &r.Status, &r.Result, &r.Depth, &r.Iterations, &r.ToolsUsed, &r.TokensIn, &r.TokensOut, &r.CostUUSD, &r.SessionID, &r.TraceID, &r.CreatedAt, &r.CompletedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, nil
}

func (s *SubagentRunStore) ListByTenant(ctx context.Context, tenantID string, limit int) ([]SubagentRunRecord, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, parent_id, agent_key, task, status, result, depth, iterations, tools_used, tokens_in, tokens_out, cost_uusd, session_id, COALESCE(trace_id::text,''), created_at, completed_at
		FROM subagent_runs
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SubagentRunRecord
	for rows.Next() {
		var r SubagentRunRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ParentID, &r.AgentKey, &r.Task, &r.Status, &r.Result, &r.Depth, &r.Iterations, &r.ToolsUsed, &r.TokensIn, &r.TokensOut, &r.CostUUSD, &r.SessionID, &r.TraceID, &r.CreatedAt, &r.CompletedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, nil
}

func (s *SubagentRunStore) GetTraceTree(ctx context.Context, tenantID, rootParentID string) ([]SubagentRunRecord, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE tree AS (
			SELECT * FROM subagent_runs WHERE tenant_id = $1 AND parent_id = $2
			UNION ALL
			SELECT sr.* FROM subagent_runs sr
			JOIN tree t ON sr.parent_id = t.id AND sr.tenant_id = $1
		)
		SELECT id, tenant_id, parent_id, agent_key, task, status, result, depth, iterations, tools_used, tokens_in, tokens_out, cost_uusd, session_id, COALESCE(trace_id::text,''), created_at, completed_at
		FROM tree
		ORDER BY depth ASC, created_at ASC
	`, tenantID, rootParentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SubagentRunRecord
	for rows.Next() {
		var r SubagentRunRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ParentID, &r.AgentKey, &r.Task, &r.Status, &r.Result, &r.Depth, &r.Iterations, &r.ToolsUsed, &r.TokensIn, &r.TokensOut, &r.CostUUSD, &r.SessionID, &r.TraceID, &r.CreatedAt, &r.CompletedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, nil
}
