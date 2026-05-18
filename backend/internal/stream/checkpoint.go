// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package stream provides SSE stream checkpoint utilities.
// Checkpoints record the last delivered sequence number per session so
// a reconnecting client can request only missed events (not full replay).
//
// Claude Code equivalent: SSETransport.getLastSequenceNum() + from_sequence_num header.
package stream

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Checkpoint tracks the last delivered sequence number for a session.
type Checkpoint struct {
	SessionID string
	seq       atomic.Int64
}

// NewCheckpoint creates a checkpoint starting at seq 0.
func NewCheckpoint(sessionID string) *Checkpoint {
	return &Checkpoint{SessionID: sessionID}
}

// Next increments and returns the next sequence number.
func (c *Checkpoint) Next() int64 {
	return c.seq.Add(1)
}

// Current returns the current (last delivered) sequence number.
func (c *Checkpoint) Current() int64 {
	return c.seq.Load()
}

// CheckpointStore is a DB-backed store for sequence numbers.
// On reconnect, clients send Last-Event-ID and the store returns
// the last known sequence so the server can skip already-delivered events.
type CheckpointStore struct {
	pool  *pgxpool.Pool
	cache sync.Map // sessionID → *Checkpoint (in-memory fast path)
}

// NewCheckpointStore creates a store backed by the given pool.
func NewCheckpointStore(pool *pgxpool.Pool) *CheckpointStore {
	return &CheckpointStore{pool: pool}
}

// Get returns a checkpoint for a session, loading from DB if not cached.
func (s *CheckpointStore) Get(ctx context.Context, sessionID, tenantID string) *Checkpoint {
	if v, ok := s.cache.Load(sessionID); ok {
		return v.(*Checkpoint)
	}
	cp := NewCheckpoint(sessionID)
	// Load last known seq from DB
	if s.pool != nil {
		var seq int64
		err := s.pool.QueryRow(ctx,
			`SELECT last_seq FROM stream_checkpoints WHERE session_id = $1`, sessionID,
		).Scan(&seq)
		if err == nil {
			cp.seq.Store(seq)
		}
	}
	s.cache.Store(sessionID, cp)
	return cp
}

// Save persists the current checkpoint to DB. Called periodically (not every event).
func (s *CheckpointStore) Save(ctx context.Context, cp *Checkpoint, tenantID string) {
	if s.pool == nil {
		return
	}
	seq := cp.Current()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stream_checkpoints (session_id, tenant_id, last_seq, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (session_id) DO UPDATE SET last_seq = $3, updated_at = NOW()`,
		cp.SessionID, tenantID, seq)
	if err != nil {
		slog.Warn("stream.checkpoint.save_failed", "session", cp.SessionID, "seq", seq, "error", err)
	}
}

// SavePeriodic saves the checkpoint every interval in a background goroutine.
// Returns a cancel function to stop saving.
func (s *CheckpointStore) SavePeriodic(ctx context.Context, cp *Checkpoint, tenantID string, interval time.Duration) func() {
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx2.Done():
				// Final save on exit
				s.Save(context.Background(), cp, tenantID)
				return
			case <-ticker.C:
				s.Save(ctx2, cp, tenantID)
			}
		}
	}()
	return cancel
}

// Evict removes a session's checkpoint from the in-memory cache (not from DB).
func (s *CheckpointStore) Evict(sessionID string) {
	s.cache.Delete(sessionID)
}
