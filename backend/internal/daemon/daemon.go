// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package daemon

import (
	"context"
	"log/slog"
	"time"
)

// Daemon ties a Registry and an optional WorkspaceManager together and runs
// background maintenance goroutines:
//
//   - Stale-agent reaper: marks agents offline when their heartbeat is
//     missing for >staleTimeout, then re-queues their in-progress task.
//   - Pending-task retrier: when an agent becomes idle, any queued tasks
//     that were stuck waiting for an agent are retried.
//
// Usage:
//
//	d, _ := NewDaemon("/path/to/repo")
//	d.Start(ctx)
//	defer d.Shutdown()
//
// If repoRoot is empty, workspace isolation is disabled (tasks share the
// working directory).
type Daemon struct {
	Reg       *Registry
	Workspace *WorkspaceManager // nil if repoRoot was empty

	staleTimeout time.Duration
	retryTick    time.Duration
}

// NewDaemon creates a Daemon. If repoRoot is non-empty, a WorkspaceManager
// is created for git worktree isolation.
func NewDaemon(repoRoot string) (*Daemon, error) {
	reg := New()
	d := &Daemon{
		Reg:          reg,
		staleTimeout: 60 * time.Second,
		retryTick:    15 * time.Second,
	}
	if repoRoot != "" {
		wm, err := NewWorkspaceManager(repoRoot)
		if err != nil {
			return nil, err
		}
		d.Workspace = wm
	}
	return d, nil
}

// Start launches background goroutines. ctx cancellation stops them.
func (d *Daemon) Start(ctx context.Context) {
	go d.runStaleReaper(ctx)
	go d.runPendingRetrier(ctx)
	slog.Info("daemon.started", "stale_timeout", d.staleTimeout, "retry_tick", d.retryTick)
}

// Shutdown cleans up all workspaces. Call after the context is cancelled.
func (d *Daemon) Shutdown() {
	if d.Workspace != nil {
		d.Workspace.CleanupAll()
	}
	slog.Info("daemon.shutdown")
}

// AllocWorkspace creates a git worktree for the given task and injects its
// path into the task's context. No-op if workspace isolation is disabled.
func (d *Daemon) AllocWorkspace(taskID string) (string, error) {
	if d.Workspace == nil {
		return "", nil
	}
	path, err := d.Workspace.Create(taskID)
	if err != nil {
		return "", err
	}
	// Annotate the task context so the dispatched agent knows where to work.
	d.Reg.mu.Lock()
	if t, ok := d.Reg.tasks[taskID]; ok {
		if t.Context == nil {
			t.Context = make(map[string]any)
		}
		t.Context["workspace_path"] = path
	}
	d.Reg.mu.Unlock()
	return path, nil
}

// FreeWorkspace removes the worktree when a task completes or fails.
func (d *Daemon) FreeWorkspace(taskID string) {
	if d.Workspace == nil {
		return
	}
	if err := d.Workspace.Cleanup(taskID); err != nil {
		slog.Warn("daemon.workspace.free_err", "task", taskID, "err", err)
	}
}

// ─── Background goroutines ────────────────────────────────────────────────────

// runStaleReaper marks agents offline when their heartbeat has been silent
// for longer than staleTimeout. In-progress tasks are re-queued.
func (d *Daemon) runStaleReaper(ctx context.Context) {
	ticker := time.NewTicker(d.staleTimeout / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.reapStaleAgents()
		}
	}
}

func (d *Daemon) reapStaleAgents() {
	d.Reg.mu.Lock()
	defer d.Reg.mu.Unlock()

	cutoff := time.Now().Add(-d.staleTimeout)
	for id, inst := range d.Reg.agents {
		if inst.Status == StatusOffline {
			continue
		}
		if inst.LastSeenAt.Before(cutoff) {
			slog.Warn("daemon.agent.stale", "id", id, "last_seen", inst.LastSeenAt)
			// Re-queue in-progress task.
			if inst.CurrentTask != "" {
				if t, ok := d.Reg.tasks[inst.CurrentTask]; ok && t.Status == TaskInProgress {
					t.Status = TaskQueued
					t.Owner = ""
					slog.Info("daemon.task.requeued", "task", t.ID, "reason", "agent_stale")
				}
			}
			inst.Status = StatusOffline
			inst.CurrentTask = ""
			d.Reg.broadcast(Event{Type: EvtAgentStatus, Data: map[string]any{
				"id": id, "status": StatusOffline,
			}})
		}
	}
}

// runPendingRetrier re-dispatches queued tasks that have no owner yet.
// This covers the case where a task was created but no agent was idle at the time.
func (d *Daemon) runPendingRetrier(ctx context.Context) {
	ticker := time.NewTicker(d.retryTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.retryPending()
		}
	}
}

func (d *Daemon) retryPending() {
	d.Reg.mu.RLock()
	var unowned []string
	for id, t := range d.Reg.tasks {
		if t.Status == TaskQueued && t.Owner == "" {
			unowned = append(unowned, id)
		}
	}
	d.Reg.mu.RUnlock()

	for _, taskID := range unowned {
		go d.Reg.autoDispatch(context2Background(), taskID)
	}
	if len(unowned) > 0 {
		slog.Info("daemon.retry.pending", "count", len(unowned))
	}
}
