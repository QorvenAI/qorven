// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/tasks"
)

// recoverInflightTasks reclaims any task left 'in_progress' whose lease has
// expired — meaning the worker goroutine died or became permanently stuck.
// Active tasks are protected: task_worker renews lease_expires every ~20 s
// via the heartbeat goroutine, keeping it 3 minutes in the future.  A task
// whose lease_expires is NULL (pre-045 row) or in the past is considered dead
// and is moved back to 'assigned' so the agent is re-woken from its last
// scratchpad checkpoint.  Idempotent: safe to call on every boot and watchdog tick.
func (gw *Gateway) recoverInflightTasks(ctx context.Context) {
	if gw.db == nil || gw.db.Pool == nil {
		return
	}

	rows, err := gw.db.Pool.Query(ctx,
		`UPDATE tasks
		    SET status = $1, locked_by = NULL, updated_at = NOW()
		  WHERE status = $2
		    AND (lease_expires IS NULL OR lease_expires < NOW())
		  RETURNING id::text, COALESCE(assigned_to::text, '')`,
		tasks.StatusAssigned, tasks.StatusInProgress,
	)
	if err != nil {
		slog.Warn("recovery.sweep_failed", "err", err)
		return
	}
	defer rows.Close()

	type reclaim struct{ taskID, agentID string }
	var list []reclaim
	for rows.Next() {
		var taskID, agentID string
		if err := rows.Scan(&taskID, &agentID); err == nil && agentID != "" {
			list = append(list, reclaim{taskID, agentID})
		}
	}

	for _, r := range list {
		slog.Info("recovery.requeue", "task", r.taskID, "agent", r.agentID)
		if gw.runtimeMgr != nil {
			gw.runtimeMgr.WakeAgent(r.agentID, agent.WakeupSignal{
				Source: agent.WakeupAssignment,
				TaskID: r.taskID,
			})
		}
	}
}

// recoverStuckMerges resets merge_queue rows stranded in 'merging' — a process
// death (or a cancelled context) between claiming a row and writing its final
// status would otherwise leave it 'merging' forever, and the queue's
// NOT EXISTS('merging') serialization guard would then deadlock every future
// merge for that project. A row whose updated_at is older than 5 minutes (well
// past the 20 s merge call) is presumed stranded and returned to 'queued'.
func (gw *Gateway) recoverStuckMerges(ctx context.Context) {
	if gw.db == nil || gw.db.Pool == nil {
		return
	}
	tag, err := gw.db.Pool.Exec(ctx,
		`UPDATE merge_queue
		    SET status = 'queued', updated_at = NOW()
		  WHERE status = 'merging' AND updated_at < NOW() - INTERVAL '5 minutes'`)
	if err != nil {
		slog.Warn("recovery.merge_sweep_failed", "err", err)
		return
	}
	if n := tag.RowsAffected(); n > 0 {
		slog.Info("recovery.merge_requeued", "count", n)
	}
}

// startTaskWatchdog periodically reclaims tasks whose worker stopped
// heartbeating (dead or stuck process) so they never hang 'in_progress'
// forever, and frees merge_queue rows stranded in 'merging'. Runs every 60
// seconds; each tick gets its own 30-second deadline.
func (gw *Gateway) startTaskWatchdog() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				gw.recoverInflightTasks(ctx)
				gw.recoverStuckMerges(ctx)
			}()
		}
	}()
}
