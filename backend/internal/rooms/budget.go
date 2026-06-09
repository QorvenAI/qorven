// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package rooms

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Default per-room agent-turn budget: at most DefaultTurnCap automated agent
// responses per DefaultWindow, so a room can't loop or overspend.
const (
	DefaultTurnCap = 20
	DefaultWindow  = 10 * time.Minute
)

// BudgetAllows reports whether another agent turn is permitted given how many
// turns already happened in the window and the limit. A limit <= 0 means unlimited.
// Pure.
func BudgetAllows(turnsInWindow, limit int) bool {
	if limit <= 0 {
		return true
	}
	return turnsInWindow < limit
}

// BudgetStore records and counts per-room agent turns.
type BudgetStore struct{ pool *pgxpool.Pool }

func NewBudgetStore(pool *pgxpool.Pool) *BudgetStore { return &BudgetStore{pool: pool} }

// RecordTurn logs one agent turn in a room.
func (s *BudgetStore) RecordTurn(ctx context.Context, tenantID, roomID, agentID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO room_run_budgets (tenant_id, room_id, agent_id) VALUES ($1,$2,$3)`,
		tenantID, roomID, agentID)
	return err
}

// TurnsInWindow counts agent turns in a room within the trailing `window`.
func (s *BudgetStore) TurnsInWindow(ctx context.Context, roomID string, window time.Duration) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM room_run_budgets
		 WHERE room_id=$1 AND created_at >= now() - make_interval(secs => $2)`,
		roomID, window.Seconds()).Scan(&n)
	return n, err
}
