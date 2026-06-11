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

// startTaskWatchdog periodically reclaims tasks whose worker stopped
// heartbeating (dead or stuck process) so they never hang 'in_progress'
// forever. Runs every 60 seconds; each tick gets its own 30-second deadline.
func (gw *Gateway) startTaskWatchdog() {
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				gw.recoverInflightTasks(ctx)
			}()
		}
	}()
}
