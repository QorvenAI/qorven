// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package goals

import (
	"context"
	"time"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Goal struct {
	ID string `json:"id"`; AgentID string `json:"agent_id,omitempty"`; TenantID string `json:"tenant_id"`
	Title string `json:"title"`; Description string `json:"description"`; Status string `json:"status"`
	DueDate *time.Time `json:"due_date,omitempty"`; CreatedAt time.Time `json:"created_at"`
}

type Store struct{ pool *pgxpool.Pool }
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, g Goal) (*Goal, error) {
	g.ID = uuid.New().String(); g.Status = "active"; g.CreatedAt = time.Now()
	_, err := s.pool.Exec(ctx, `INSERT INTO goals (id,agent_id,tenant_id,title,description,status,due_date,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		g.ID, g.AgentID, g.TenantID, g.Title, g.Description, g.Status, g.DueDate, g.CreatedAt)
	return &g, err
}

func (s *Store) List(ctx context.Context, tenantID string) ([]Goal, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,agent_id,tenant_id,title,description,status,due_date,created_at FROM goals WHERE tenant_id=$1 ORDER BY created_at DESC`, tenantID)
	if err != nil { return nil, err }; defer rows.Close()
	out := []Goal{}
	for rows.Next() { var g Goal; rows.Scan(&g.ID, &g.AgentID, &g.TenantID, &g.Title, &g.Description, &g.Status, &g.DueDate, &g.CreatedAt); out = append(out, g) }
	return out, nil
}

func (s *Store) ListForAgent(ctx context.Context, agentID string) ([]Goal, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,agent_id,tenant_id,title,description,status,due_date,created_at FROM goals WHERE agent_id=$1 AND status='active'`, agentID)
	if err != nil { return nil, err }; defer rows.Close()
	out := []Goal{}
	for rows.Next() { var g Goal; rows.Scan(&g.ID, &g.AgentID, &g.TenantID, &g.Title, &g.Description, &g.Status, &g.DueDate, &g.CreatedAt); out = append(out, g) }
	return out, nil
}

func (s *Store) Update(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE goals SET status=$1 WHERE id=$2`, status, id); return err
}

var Templates = []Goal{
	{Title: "Research my niche", Description: "Analyze competitors, trends, and opportunities"},
	{Title: "Manage marketing", Description: "Create and schedule social media content"},
	{Title: "Monitor competitors", Description: "Track competitor releases and strategies"},
	{Title: "Daily standup", Description: "Summarize progress, plan, and blockers"},
	{Title: "Customer support", Description: "Handle queries, escalate complex issues"},
	{Title: "Code review", Description: "Review PRs, suggest improvements"},
}
