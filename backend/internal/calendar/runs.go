// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package calendar

import (
	"context"
	"time"
)

const (
	RunStatusRunning = "running"
	RunStatusOK      = "ok"
	RunStatusError   = "error"
)

// Run is one recorded scheduled execution.
type Run struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	AgentID       *string    `json:"agent_id"`
	Source        string     `json:"source"`
	SourceID      string     `json:"source_id"`
	Title         string     `json:"title"`
	ScheduledFor  *time.Time `json:"scheduled_for,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Status        string     `json:"status"`
	ResultSnippet string     `json:"result_snippet"`
	Tokens        int64      `json:"tokens"`
	CostCents     int64      `json:"cost_cents"`
	Error         string     `json:"error,omitempty"`
}

// StartRun inserts a 'running' row and returns its id. agentID may be "" (→ NULL).
func (s *Store) StartRun(ctx context.Context, tenantID, agentID, source, sourceID, title string, scheduledFor *time.Time) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO scheduled_runs (tenant_id, agent_id, source, source_id, title, scheduled_for, status)
		 VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6, 'running') RETURNING id`,
		tenantID, agentID, source, sourceID, title, scheduledFor,
	).Scan(&id)
	return id, err
}

// FinishRun marks a run ok/error with its result snippet and usage.
func (s *Store) FinishRun(ctx context.Context, runID, status, resultSnippet, errMsg string, tokens, costCents int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE scheduled_runs
		 SET status=$1, result_snippet=$2, error=$3, tokens=$4, cost_cents=$5, finished_at=now()
		 WHERE id=$6`,
		status, resultSnippet, errMsg, tokens, costCents, runID)
	return err
}

// GetRun returns a single run by id, tenant-scoped.
func (s *Store) GetRun(ctx context.Context, tenantID, runID string) (*Run, error) {
	var r Run
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, source, source_id, title, scheduled_for,
		        started_at, finished_at, status, result_snippet, tokens, cost_cents, error
		 FROM scheduled_runs WHERE tenant_id=$1 AND id=$2`,
		tenantID, runID,
	).Scan(&r.ID, &r.TenantID, &r.AgentID, &r.Source, &r.SourceID, &r.Title, &r.ScheduledFor,
		&r.StartedAt, &r.FinishedAt, &r.Status, &r.ResultSnippet, &r.Tokens, &r.CostCents, &r.Error)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRuns returns runs in a window, optionally filtered by agent, newest first.
func (s *Store) ListRuns(ctx context.Context, tenantID string, agentID *string, start, end time.Time) ([]Run, error) {
	query := `SELECT id, tenant_id, agent_id, source, source_id, title, scheduled_for,
	                 started_at, finished_at, status, result_snippet, tokens, cost_cents, error
	          FROM scheduled_runs WHERE tenant_id=$1 AND started_at >= $2 AND started_at <= $3`
	args := []any{tenantID, start, end}
	if agentID != nil {
		query += ` AND agent_id = $4`
		args = append(args, *agentID)
	}
	query += ` ORDER BY started_at DESC LIMIT 500`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.TenantID, &r.AgentID, &r.Source, &r.SourceID, &r.Title, &r.ScheduledFor,
			&r.StartedAt, &r.FinishedAt, &r.Status, &r.ResultSnippet, &r.Tokens, &r.CostCents, &r.Error); err != nil {
			continue
		}
		runs = append(runs, r)
	}
	return runs, nil
}
