// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/tasks"
)

// taskPresenceChecker is a narrow interface to avoid a direct import-cycle
// between the gateway and presence packages.
type taskPresenceChecker interface {
	IsOnline(ctx context.Context, tenantID string) (bool, error)
	LastChannel(ctx context.Context, userID string) (string, error)
}

// TaskCoordinator triggers parent-task synthesis once all subtasks are terminal.
// It is called by the task worker whenever a task transitions to done or blocked.
// The coordinator performs an atomic CAS update (synthesis_triggered_at) to
// ensure exactly-once dispatch even if multiple subtasks complete concurrently.
type TaskCoordinator struct {
	taskStore     *tasks.Store
	runtimeMgr    *agent.RuntimeManager
	rtHub         *realtime.Hub
	presenceStore taskPresenceChecker
}

// NewTaskCoordinator returns a ready-to-use coordinator.
func NewTaskCoordinator(ts *tasks.Store, rm *agent.RuntimeManager, hub *realtime.Hub) *TaskCoordinator {
	return &TaskCoordinator{taskStore: ts, runtimeMgr: rm, rtHub: hub}
}

// SetPresence wires an optional presence checker. Safe to call with nil.
func (c *TaskCoordinator) SetPresence(p taskPresenceChecker) { c.presenceStore = p }

// onTaskComplete is called by the iteration loop when a task transitions to done.
// It checks whether all siblings are terminal, then atomically marks the parent
// as synthesis-triggered and wakes the parent agent for synthesis.
func (c *TaskCoordinator) onTaskComplete(ctx context.Context, task tasks.Task) {
	// Step 1 — for top-level tasks, escalate if the user is offline.
	if task.ParentID == nil {
		go c.escalateIfOffline(ctx, task)
		return
	}
	parentID := *task.ParentID

	// Step 2 — check whether every sibling is in a terminal state.
	allTerminal, err := c.taskStore.SiblingStatus(ctx, parentID)
	if err != nil {
		slog.Warn("task_coordinator: SiblingStatus error", "parent", parentID, "error", err)
		return
	}
	if !allTerminal {
		return
	}

	// Step 3 — fetch the parent and skip if synthesis already triggered.
	parent, err := c.taskStore.Get(ctx, parentID)
	if err != nil {
		slog.Warn("task_coordinator: failed to fetch parent task", "parent", parentID, "error", err)
		return
	}
	if parent.SynthesisTriggeredAt != nil {
		// Another subtask completion already claimed synthesis for this parent.
		return
	}

	// Step 4 — atomic CAS: set synthesis_triggered_at only if it is still NULL.
	// Uses the raw pool so we can inspect RowsAffected without a round-trip SELECT.
	tag, err := c.taskStore.Pool().Exec(ctx,
		`UPDATE tasks SET synthesis_triggered_at = NOW()
		 WHERE id = $1 AND synthesis_triggered_at IS NULL`,
		parentID,
	)
	if err != nil {
		slog.Warn("task_coordinator: CAS update failed", "parent", parentID, "error", err)
		return
	}
	if tag.RowsAffected() == 0 {
		// Another goroutine won the race; bail out silently.
		return
	}

	// Step 5 — collect all subtask results.
	subtasks, err := c.taskStore.GetSubtasks(ctx, parentID)
	if err != nil {
		slog.Warn("task_coordinator: GetSubtasks failed", "parent", parentID, "error", err)
		return
	}

	// Step 6 — build a result map for the synthesis context.
	resultMap := make([]map[string]any, 0, len(subtasks))
	for _, st := range subtasks {
		resultMap = append(resultMap, map[string]any{
			"id":     st.ID,
			"title":  st.Title,
			"status": st.Status,
			"result": st.Result,
		})
	}

	// Step 7 — broadcast to all connected clients.
	if c.rtHub != nil {
		c.rtHub.Broadcast(realtime.Event{
			Type:    realtime.EventSynthesisTriggered,
			AgentID: func() string {
				if parent.AssignedTo != nil {
					return *parent.AssignedTo
				}
				return ""
			}(),
			Data: map[string]any{
				"parent_task_id":  parent.ID,
				"subtask_results": resultMap,
			},
			Timestamp: time.Now().UnixMilli(),
		})
	}

	// Step 8 — guard: parent must have an assigned agent.
	if parent.AssignedTo == nil {
		slog.Warn("task_coordinator: parent task has no assigned agent — cannot wake for synthesis",
			"parent", parentID)
		return
	}

	// Step 9 — wake the parent agent for synthesis.
	slog.Info("task_coordinator: triggering synthesis", "parent", parentID, "agent", *parent.AssignedTo)
	c.runtimeMgr.WakeAgent(*parent.AssignedTo, agent.WakeupSignal{
		Source: agent.WakeupSynthesis,
		TaskID: parent.ID,
		Context: map[string]any{
			"subtask_results": resultMap,
			"mode":            "synthesis",
		},
	})
}

// escalateIfOffline checks whether the tenant is currently online and, if not,
// logs the offline escalation event. Full channel-based nudge delivery can be
// added here later when TaskCoordinator gains a reference to the gateway's send
// functions.
func (c *TaskCoordinator) escalateIfOffline(ctx context.Context, task tasks.Task) {
	if c.presenceStore == nil {
		return
	}
	online, err := c.presenceStore.IsOnline(ctx, task.TenantID)
	if err != nil {
		slog.Warn("escalation.presence_error", "task", task.ID, "error", err)
		return
	}
	if online {
		slog.Info("escalation.user_online_skip", "task", task.ID)
		return
	}
	// User is offline — log for now; full channel-based nudge can be added later.
	slog.Info("escalation.user_offline",
		"task", task.ID,
		"tenant", task.TenantID,
		"title", task.Title,
		"status", task.Status,
	)
}
