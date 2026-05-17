// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// startTaskLockRenewal extends the task lock every 5 min while the agent works.
// Prevents the task recovery ticker from reclaiming active tasks as stale.
// Returns a stop func — call it when the run finishes.
func startTaskLockRenewal(ctx context.Context, pool *pgxpool.Pool, taskID string) (stop func()) {
	if pool == nil || taskID == "" {
		return func() {}
	}
	ch := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, err := pool.Exec(ctx,
					`UPDATE tasks SET updated_at = NOW() WHERE id = $1 AND status = 'in_progress'`, taskID)
				if err != nil {
					slog.Warn("task.lock_renewal_failed", "task_id", taskID, "error", err)
					return
				}
				slog.Debug("task.lock_renewed", "task_id", taskID)
			case <-ch:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { close(ch) }
}

// resolveTaskOutcome determines the correct task lifecycle action after an agent run.
// error → fail, silent/empty → fail, success → complete.
func resolveTaskOutcome(ctx context.Context, pool *pgxpool.Pool, taskID string, content string, err error) {
	if pool == nil || taskID == "" {
		return
	}

	if err != nil {
		// Agent errored → auto-fail
		errMsg := err.Error()
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
		pool.Exec(ctx,
			`UPDATE tasks SET status = 'failed', result = $2, updated_at = NOW() WHERE id = $1`,
			taskID, errMsg)
		slog.Info("task.auto_failed", "task_id", taskID, "error", errMsg)
		return
	}

	if content == "" || isSilentReply(content) {
		// Empty/silent result → fail
		pool.Exec(ctx,
			`UPDATE tasks SET status = 'failed', result = 'Agent returned empty result', updated_at = NOW() WHERE id = $1`,
			taskID)
		slog.Info("task.auto_failed_empty", "task_id", taskID)
		return
	}

	// Success → auto-complete
	result := content
	if len(result) > 100000 {
		result = result[:100000] + "\n[truncated]"
	}
	pool.Exec(ctx,
		`UPDATE tasks SET status = 'completed', result = $2, completed_at = NOW(), updated_at = NOW() WHERE id = $1`,
		taskID, result)
	slog.Info("task.auto_completed", "task_id", taskID)

	// Dispatch unblocked tasks — tasks that were blocked_by this one
	dispatchUnblockedTasks(ctx, pool, taskID)
}

// dispatchUnblockedTasks finds tasks blocked by the completed task and moves them to pending.
func dispatchUnblockedTasks(ctx context.Context, pool *pgxpool.Pool, completedTaskID string) {
	if pool == nil {
		return
	}
	// Find tasks that have this task in their blocked_by array and all blockers are now completed
	tag, err := pool.Exec(ctx, `
		UPDATE tasks SET status = 'pending', updated_at = NOW()
		WHERE status = 'blocked'
		AND $1 = ANY(blocked_by)
		AND NOT EXISTS (
			SELECT 1 FROM unnest(blocked_by) AS bid
			JOIN tasks t2 ON t2.id = bid::uuid
			WHERE t2.status != 'completed'
		)`, completedTaskID)
	if err != nil {
		slog.Warn("task.dispatch_unblocked_failed", "completed", completedTaskID, "error", err)
		return
	}
	if tag.RowsAffected() > 0 {
		slog.Info("task.unblocked", "completed", completedTaskID, "unblocked", tag.RowsAffected())
	}
}

// generateTaskID creates a new UUID for a task.
func generateTaskID() string {
	return uuid.New().String()
}
