// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package souldesk

import (
	"context"
	"log/slog"

	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
)

// TaskIntegration creates Kanban tasks when Souls are delegated work.
type TaskIntegration struct {
	taskStore *tasks.Store
	tenantID  string
}

func NewTaskIntegration(taskStore *tasks.Store, tenantID string) *TaskIntegration {
	return &TaskIntegration{taskStore: taskStore, tenantID: tenantID}
}

// CreateDelegationTask creates a task in the Kanban board when work is delegated.
func (ti *TaskIntegration) CreateDelegationTask(ctx context.Context, soulID, soulKey, assignedBy, taskDesc string) (string, error) {
	t := tasks.Task{
		Title:       "Delegated: " + truncate(taskDesc, 80),
		Description: taskDesc,
		AssignedTo:  &soulID,
		AssignedBy:  &assignedBy,
		Priority:    2,
	}

	id, err := ti.taskStore.Create(ctx, ti.tenantID, t)
	if err != nil {
		slog.Warn("souldesk.task.create_failed", "soul", soulKey, "error", err)
		return "", err
	}

	// Move to in_progress
	ti.taskStore.Transition(ctx, id, "in_progress")

	slog.Info("souldesk.task.created", "task_id", id[:8], "soul", soulKey)
	return id, nil
}

// CompleteDelegationTask marks a task as done with the result.
func (ti *TaskIntegration) CompleteDelegationTask(ctx context.Context, taskID, result string, tokensUsed int64) {
	if taskID == "" {
		return
	}
	ti.taskStore.Complete(ctx, taskID, truncate(result, 2000), tokensUsed)
	slog.Info("souldesk.task.completed", "task_id", taskID[:8])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// HeartbeatTaskPickup checks for assigned tasks and returns them for a Soul to work on.
func (ti *TaskIntegration) GetPendingTasks(ctx context.Context, soulID string) ([]tasks.Task, error) {
	return ti.taskStore.ListForAgent(ctx, soulID, "assigned", 5)
}

// PickupTask is a tool that lets Souls check and pick up assigned tasks.
type PickupTaskTool struct {
	ti *TaskIntegration
}

func NewPickupTaskTool(ti *TaskIntegration) *PickupTaskTool {
	return &PickupTaskTool{ti: ti}
}

func (t *PickupTaskTool) Name() string        { return "check_tasks" }
func (t *PickupTaskTool) Description() string { return "Check your assigned tasks from the task board. Returns pending tasks you should work on." }
func (t *PickupTaskTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *PickupTaskTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	agentID := tools.AgentIDFromCtx(ctx)
	if agentID == "" {
		return tools.ErrorResult("no agent context")
	}

	pending, err := t.ti.GetPendingTasks(ctx, agentID)
	if err != nil {
		return tools.ErrorResult(err.Error())
	}

	if len(pending) == 0 {
		return tools.TextResult("No pending tasks assigned to you.")
	}

	var result string
	for _, task := range pending {
		result += "- [P" + string(rune('0'+task.Priority)) + "] " + task.Title + "\n"
		if task.Description != "" {
			result += "  " + truncate(task.Description, 200) + "\n"
		}
	}
	return tools.TextResult(result)
}
