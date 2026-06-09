// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package reachuser

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Escalation is one "reach the user" request, persisted in the escalations table.
type Escalation struct {
	ID          string
	TenantID    string
	UserID      string
	Kind        string
	RefID       string
	Title       string
	Body        string
	Urgency     string
	CurrentRung int
	Status      string
}

// Store persists escalations and their per-delivery audit steps.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Open inserts a new pending escalation with the given next_advance_at.
// A zero nextAdvanceAt is stored as NULL (no further timed advance).
func (s *Store) Open(ctx context.Context, e Escalation, nextAdvanceAt time.Time) (string, error) {
	var nat any
	if !nextAdvanceAt.IsZero() {
		nat = nextAdvanceAt
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO escalations (tenant_id, user_id, kind, ref_id, title, body, urgency, current_rung, status, next_advance_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending',$9) RETURNING id`,
		e.TenantID, e.UserID, e.Kind, e.RefID, e.Title, e.Body, e.Urgency, e.CurrentRung, nat,
	).Scan(&id)
	return id, err
}

// DuePending returns pending escalations whose next_advance_at is at or before `now`.
func (s *Store) DuePending(ctx context.Context, now time.Time) ([]Escalation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, user_id, kind, ref_id, title, body, urgency, current_rung, status
		 FROM escalations
		 WHERE status='pending' AND next_advance_at IS NOT NULL AND next_advance_at <= $1
		 ORDER BY next_advance_at`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Escalation{}
	for rows.Next() {
		var e Escalation
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.Kind, &e.RefID, &e.Title, &e.Body, &e.Urgency, &e.CurrentRung, &e.Status); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Advance sets the new rung and next_advance_at. Pass a zero time to stop further advance.
func (s *Store) Advance(ctx context.Context, id string, newRung int, nextAdvanceAt time.Time) error {
	if nextAdvanceAt.IsZero() {
		_, err := s.pool.Exec(ctx,
			`UPDATE escalations SET current_rung=$2, next_advance_at=NULL WHERE id=$1`, id, newRung)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE escalations SET current_rung=$2, next_advance_at=$3 WHERE id=$1`, id, newRung, nextAdvanceAt)
	return err
}

// Exhaust marks an escalation finished after the last rung (no ack).
func (s *Store) Exhaust(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE escalations SET status='exhausted', next_advance_at=NULL WHERE id=$1`, id)
	return err
}

// Ack marks the matching escalation acknowledged so the ticker stops climbing.
func (s *Store) Ack(ctx context.Context, kind, refID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE escalations SET status='acked', acked_at=now(), next_advance_at=NULL
		 WHERE kind=$1 AND ref_id=$2 AND status='pending'`, kind, refID)
	return err
}

// LogStep appends an audit row for one delivery attempt (best-effort).
func (s *Store) LogStep(ctx context.Context, escalationID string, rung int, channel, outcome, detail string) {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO escalation_steps (escalation_id, rung, channel, outcome, detail) VALUES ($1,$2,$3,$4,$5)`,
		escalationID, rung, channel, outcome, detail); err != nil {
		slog.Warn("reachuser.logstep.failed", "escalation_id", escalationID, "err", err)
	}
}

// get is a test/internal helper to read one escalation.
func (s *Store) get(ctx context.Context, id string) (Escalation, error) {
	var e Escalation
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, user_id, kind, ref_id, title, body, urgency, current_rung, status
		 FROM escalations WHERE id=$1`, id,
	).Scan(&e.ID, &e.TenantID, &e.UserID, &e.Kind, &e.RefID, &e.Title, &e.Body, &e.Urgency, &e.CurrentRung, &e.Status)
	return e, err
}
