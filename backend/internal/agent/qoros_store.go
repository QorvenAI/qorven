// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"time"
)

// QorosStore implements QorosStateDB on top of the agent Store's pgxpool.
// All state is scoped to (tenant_id, agent_id) — safe for multi-tenant use.

// Verify at compile time that *Store satisfies the QorosStateDB interface.
var _ QorosStateDB = (*Store)(nil)

// SaveQorosState upserts the QOROS runtime state for an agent.
func (s *Store) SaveQorosState(ctx context.Context, agentID, tenantID string, tickCount int, sleeping bool, sleepUntil *time.Time, sleepReason string, lastTickAt *time.Time) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO qoros_state
			(agent_id, tenant_id, active, tick_count, sleeping, sleep_until, sleep_reason, last_tick_at, updated_at)
		 VALUES ($1, $2, true, $3, $4, $5, $6, $7, NOW())
		 ON CONFLICT (agent_id) DO UPDATE SET
			active       = true,
			tick_count   = $3,
			sleeping     = $4,
			sleep_until  = $5,
			sleep_reason = $6,
			last_tick_at = $7,
			updated_at   = NOW()`,
		agentID, tenantID, tickCount, sleeping, sleepUntil, sleepReason, lastTickAt)
	return err
}

// LoadQorosState fetches the last persisted QOROS state for an agent.
func (s *Store) LoadQorosState(ctx context.Context, agentID string) (found bool, tickCount int, sleeping bool, sleepUntil time.Time, err error) {
	if s.pool == nil {
		return false, 0, false, time.Time{}, nil
	}
	var sleepUntilPtr *time.Time
	qErr := s.pool.QueryRow(ctx,
		`SELECT tick_count, sleeping, sleep_until FROM qoros_state WHERE agent_id = $1`, agentID,
	).Scan(&tickCount, &sleeping, &sleepUntilPtr)
	if qErr != nil {
		return false, 0, false, time.Time{}, nil // not found is fine
	}
	if sleepUntilPtr != nil {
		sleepUntil = *sleepUntilPtr
	}
	return true, tickCount, sleeping, sleepUntil, nil
}

// MarkQorosActive sets whether a QOROS instance is running.
// Called on Start() and Stop() so the restart-recovery logic knows which agents to revive.
func (s *Store) MarkQorosActive(ctx context.Context, agentID, tenantID string, active bool) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO qoros_state (agent_id, tenant_id, active, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (agent_id) DO UPDATE SET active = $3, updated_at = NOW()`,
		agentID, tenantID, active)
	return err
}

// ListActiveQoros returns agent IDs whose QOROS was active before the last restart.
func (s *Store) ListActiveQoros(ctx context.Context, tenantID string) ([]string, error) {
	if s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT agent_id FROM qoros_state WHERE tenant_id = $1 AND active = true`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// AppendDailyLogDB writes a single log entry to the DB.
// This is the durable backup — filesystem is still the primary read path.
func (s *Store) AppendDailyLogDB(ctx context.Context, agentID, tenantID string, logDate time.Time, entryTime time.Time, content string) error {
	if s.pool == nil {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO qoros_daily_logs (agent_id, tenant_id, log_date, entry_time, content)
		 VALUES ($1, $2, $3, $4, $5)`,
		agentID, tenantID, logDate.Format("2006-01-02"), entryTime, content)
	return err
}

// ReadDailyLogDB fetches all log entries for a given date, ordered by time.
// Returns raw content strings (not yet formatted as markdown).
func (s *Store) ReadDailyLogDB(ctx context.Context, agentID string, logDate time.Time) ([]string, error) {
	if s.pool == nil {
		return nil, fmt.Errorf("no db")
	}
	rows, err := s.pool.Query(ctx,
		`SELECT content FROM qoros_daily_logs
		 WHERE agent_id = $1 AND log_date = $2
		 ORDER BY entry_time ASC`,
		agentID, logDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []string
	for rows.Next() {
		var content string
		if rows.Scan(&content) == nil {
			entries = append(entries, content)
		}
	}
	return entries, nil
}
