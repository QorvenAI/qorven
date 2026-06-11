// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/tasks"
)

// recoverInflightTasks reclaims any task left 'in_progress' whose lock-renewal
// heartbeat (updated_at) has gone stale — meaning the worker goroutine died
// with the process or became permanently stuck. Those tasks are moved back to
// 'assigned' and the owning agent is re-woken from its last checkpoint
// (scratchpad). Idempotent: safe to call on every boot and on every watchdog
// tick. Active tasks are protected: task_worker calls startTaskLockRenewal
// which bumps updated_at every 5 minutes, so only genuinely dead tasks (silent
// for >10 minutes) are reclaimed.
func (gw *Gateway) recoverInflightTasks(ctx context.Context) {
	if gw.db == nil || gw.db.Pool == nil {
		return
	}

	rows, err := gw.db.Pool.Query(ctx,
		`UPDATE tasks
		    SET status = $1, locked_by = NULL, updated_at = NOW()
		  WHERE status = $2
		    AND updated_at < NOW() - INTERVAL '10 minutes'
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
