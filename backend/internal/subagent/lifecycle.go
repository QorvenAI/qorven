// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package subagent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunStatus tracks the lifecycle of a subagent delegation.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusCompleted RunStatus = "completed"
	StatusFailed    RunStatus = "failed"
	StatusTimedOut  RunStatus = "timed_out"
	StatusCancelled RunStatus = "cancelled"
)

// Run is a tracked subagent execution with full lifecycle.
type Run struct {
	ID            string    `json:"id"`
	ParentAgentID string    `json:"parent_agent_id"`
	ChildAgentID  string    `json:"child_agent_id"`
	Task          string    `json:"task"`
	Status        RunStatus `json:"status"`
	Result        string    `json:"result,omitempty"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	EndedAt       time.Time `json:"ended_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// LifecycleManager tracks and controls subagent runs.
type LifecycleManager struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	active  map[string]*activeRun
}

type activeRun struct {
	run    *Run
	cancel context.CancelFunc
}

func NewLifecycleManager(pool *pgxpool.Pool) *LifecycleManager {
	return &LifecycleManager{pool: pool, active: make(map[string]*activeRun)}
}

// Start creates a tracked run and returns a cancellable context.
func (lm *LifecycleManager) Start(ctx context.Context, parentID, childID, task string) (string, context.Context, context.CancelFunc) {
	runID := uuid.New().String()
	now := time.Now()

	run := &Run{
		ID: runID, ParentAgentID: parentID, ChildAgentID: childID,
		Task: task, Status: StatusRunning, StartedAt: now, CreatedAt: now,
	}

	runCtx, cancel := context.WithTimeout(ctx, Timeout)

	lm.mu.Lock()
	lm.active[runID] = &activeRun{run: run, cancel: cancel}
	lm.mu.Unlock()

	// Persist
	if lm.pool != nil {
		lm.pool.Exec(ctx,
			`INSERT INTO subagent_runs (id, parent_agent_id, child_agent_id, task, status, started_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			runID, parentID, childID, task, StatusRunning, now)
	}

	slog.Info("subagent.started", "run_id", runID, "parent", parentID, "child", childID)
	return runID, runCtx, cancel
}

// Complete marks a run as finished.
func (lm *LifecycleManager) Complete(ctx context.Context, runID, result string) {
	lm.mu.Lock()
	ar, ok := lm.active[runID]
	if ok {
		ar.run.Status = StatusCompleted
		ar.run.Result = result
		ar.run.EndedAt = time.Now()
		delete(lm.active, runID)
	}
	lm.mu.Unlock()

	if lm.pool != nil {
		lm.pool.Exec(ctx,
			`UPDATE subagent_runs SET status = $1, result = $2, ended_at = $3 WHERE id = $4`,
			StatusCompleted, result, time.Now(), runID)
	}
	slog.Info("subagent.completed", "run_id", runID)
}

// Fail marks a run as failed.
func (lm *LifecycleManager) Fail(ctx context.Context, runID, errMsg string) {
	lm.mu.Lock()
	ar, ok := lm.active[runID]
	if ok {
		ar.run.Status = StatusFailed
		ar.run.Error = errMsg
		ar.run.EndedAt = time.Now()
		delete(lm.active, runID)
	}
	lm.mu.Unlock()

	if lm.pool != nil {
		lm.pool.Exec(ctx,
			`UPDATE subagent_runs SET status = $1, error = $2, ended_at = $3 WHERE id = $4`,
			StatusFailed, errMsg, time.Now(), runID)
	}
	slog.Warn("subagent.failed", "run_id", runID, "error", errMsg)
}

// Kill cancels a running subagent.
func (lm *LifecycleManager) Kill(runID string) bool {
	lm.mu.Lock()
	ar, ok := lm.active[runID]
	if ok {
		ar.cancel()
		ar.run.Status = StatusCancelled
		ar.run.EndedAt = time.Now()
		delete(lm.active, runID)
	}
	lm.mu.Unlock()

	if ok && lm.pool != nil {
		lm.pool.Exec(context.Background(),
			`UPDATE subagent_runs SET status = $1, ended_at = $2 WHERE id = $3`,
			StatusCancelled, time.Now(), runID)
		slog.Info("subagent.killed", "run_id", runID)
	}
	return ok
}

// List returns all active runs.
func (lm *LifecycleManager) ListActive() []*Run {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	runs := make([]*Run, 0, len(lm.active))
	for _, ar := range lm.active {
		runs = append(runs, ar.run)
	}
	return runs
}

// ListAll returns recent runs from DB.
func (lm *LifecycleManager) ListAll(ctx context.Context, limit int) ([]*Run, error) {
	rows, err := lm.pool.Query(ctx,
		`SELECT id, parent_agent_id, child_agent_id, task, status, COALESCE(result,''), COALESCE(error,''), started_at, COALESCE(ended_at, now()), created_at
		 FROM subagent_runs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []*Run{}
	for rows.Next() {
		r := &Run{}
		rows.Scan(&r.ID, &r.ParentAgentID, &r.ChildAgentID, &r.Task, &r.Status, &r.Result, &r.Error, &r.StartedAt, &r.EndedAt, &r.CreatedAt)
		runs = append(runs, r)
	}
	return runs, nil
}
