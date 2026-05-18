// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package discussion

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Discussion represents a named conversation thread that groups one or more
// sessions under a single AI-generated or user-overridden label.
type Discussion struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	AgentID      string    `json:"agent_id"`
	AILabel      string    `json:"ai_label"`
	UserLabel    *string   `json:"user_label,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	MessageCount int       `json:"message_count"`
}

// Label returns user_label if set, otherwise ai_label.
func (d *Discussion) Label() string {
	if d.UserLabel != nil && *d.UserLabel != "" {
		return *d.UserLabel
	}
	return d.AILabel
}

// Store provides CRUD operations for discussions.
type Store struct{ pool *pgxpool.Pool }

// NewStore constructs a Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Create inserts a new discussion and returns its generated UUID.
func (s *Store) Create(ctx context.Context, d Discussion) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO discussions (tenant_id, agent_id, ai_label)
         VALUES ($1, $2, $3) RETURNING id`,
		d.TenantID, d.AgentID, d.AILabel,
	).Scan(&id)
	return id, err
}

// Get retrieves a single discussion by its UUID.
func (s *Store) Get(ctx context.Context, id string) (*Discussion, error) {
	d := &Discussion{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, ai_label, user_label, started_at, last_active_at, message_count
         FROM discussions WHERE id = $1`, id,
	).Scan(&d.ID, &d.TenantID, &d.AgentID, &d.AILabel, &d.UserLabel,
		&d.StartedAt, &d.LastActiveAt, &d.MessageCount)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// ListForAgent returns up to limit discussions for the given tenant and agent,
// ordered by last_active_at descending (most recent first).
func (s *Store) ListForAgent(ctx context.Context, tenantID, agentID string, limit int) ([]Discussion, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_id, ai_label, user_label, started_at, last_active_at, message_count
         FROM discussions WHERE tenant_id = $1 AND agent_id = $2 ORDER BY last_active_at DESC LIMIT $3`,
		tenantID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Discussion
	for rows.Next() {
		var d Discussion
		if err := rows.Scan(&d.ID, &d.TenantID, &d.AgentID, &d.AILabel, &d.UserLabel,
			&d.StartedAt, &d.LastActiveAt, &d.MessageCount); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetUserLabel sets (or replaces) the human-readable label on a discussion.
func (s *Store) SetUserLabel(ctx context.Context, tenantID, id, label string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE discussions SET user_label = $1 WHERE id = $2 AND tenant_id = $3`, label, id, tenantID)
	return err
}

// Touch bumps last_active_at to now and increments message_count by 1.
func (s *Store) Touch(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE discussions SET last_active_at = now(), message_count = message_count + 1 WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	return err
}

// AssignSession links an existing session to a discussion.
func (s *Store) AssignSession(ctx context.Context, sessionID, discussionID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET discussion_id = $1 WHERE id = $2`, discussionID, sessionID)
	return err
}
