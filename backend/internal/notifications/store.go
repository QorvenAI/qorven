// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package notifications

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Notification struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	AgentKey  string `json:"agent_key"`
	AgentName string `json:"agent_name"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Highlight string `json:"highlight"`
	Source    string `json:"source"`
	SourceID  string `json:"source_id"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, agentID, agentKey, agentName, nType, title, highlight, source, sourceID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO notifications (agent_id, agent_key, agent_name, type, title, highlight, source, source_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		agentID, agentKey, agentName, nType, title, highlight, source, sourceID).Scan(&id)
	return id, err
}

func (s *Store) List(ctx context.Context, limit int) ([]Notification, int, error) {
	if limit == 0 { limit = 50 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, COALESCE(agent_id,''), COALESCE(agent_key,''), COALESCE(agent_name,''),
		        type, title, COALESCE(highlight,''), COALESCE(source,''), COALESCE(source_id,''),
		        read, created_at::text
		 FROM notifications ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil { return nil, 0, err }
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		rows.Scan(&n.ID, &n.AgentID, &n.AgentKey, &n.AgentName, &n.Type, &n.Title, &n.Highlight, &n.Source, &n.SourceID, &n.Read, &n.CreatedAt)
		out = append(out, n)
	}
	var unread int
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE read = false`).Scan(&unread)
	return out, unread, nil
}

func (s *Store) MarkRead(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE notifications SET read = true WHERE id = $1`, id)
	return err
}

func (s *Store) MarkAllRead(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE notifications SET read = true WHERE read = false`)
	return err
}
