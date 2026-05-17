// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package calendar

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Event struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AgentID     *string   `json:"agent_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	StartAt     time.Time `json:"start_at"`
	EndAt       *time.Time `json:"end_at,omitempty"`
	AllDay      bool      `json:"all_day"`
	Color       string    `json:"color"`
	Location    string    `json:"location,omitempty"`
	Recurrence  string    `json:"recurrence,omitempty"`
	EventType   string    `json:"event_type"`
	SourceID    *string   `json:"source_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Store) List(ctx context.Context, tenantID string, agentID *string, start, end time.Time) ([]Event, error) {
	query := `SELECT id, tenant_id, agent_id, title, COALESCE(description,''), start_at, end_at, all_day, color, COALESCE(location,''), COALESCE(recurrence,''), event_type, source_id, created_at
		FROM calendar_events WHERE tenant_id = $1 AND start_at >= $2 AND start_at <= $3`
	args := []any{tenantID, start, end}
	if agentID != nil {
		query += ` AND agent_id = $4`
		args = append(args, *agentID)
	}
	query += ` ORDER BY start_at`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var e Event
		rows.Scan(&e.ID, &e.TenantID, &e.AgentID, &e.Title, &e.Description, &e.StartAt, &e.EndAt, &e.AllDay, &e.Color, &e.Location, &e.Recurrence, &e.EventType, &e.SourceID, &e.CreatedAt)
		events = append(events, e)
	}
	return events, nil
}

func (s *Store) Create(ctx context.Context, tenantID string, e Event) (*Event, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO calendar_events (tenant_id, agent_id, title, description, start_at, end_at, all_day, color, location, recurrence, event_type, source_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id, created_at`,
		tenantID, e.AgentID, e.Title, e.Description, e.StartAt, e.EndAt, e.AllDay, e.Color, e.Location, e.Recurrence, e.EventType, e.SourceID,
	).Scan(&e.ID, &e.CreatedAt)
	e.TenantID = tenantID
	return &e, err
}

func (s *Store) Update(ctx context.Context, id string, e Event) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE calendar_events SET title=$1, description=$2, start_at=$3, end_at=$4, all_day=$5, color=$6, location=$7, updated_at=now() WHERE id=$8`,
		e.Title, e.Description, e.StartAt, e.EndAt, e.AllDay, e.Color, e.Location, id)
	return err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM calendar_events WHERE id = $1`, id)
	return err
}
