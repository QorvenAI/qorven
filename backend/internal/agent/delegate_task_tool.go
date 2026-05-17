// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
)

const defaultMaxCyclDepth = 20

// DelegateTaskTool lets a can_delegate=true agent create a subtask and assign
// it to another Qor, which wakes immediately via its runtime.
type DelegateTaskTool struct {
	parentTaskID string
	tenantID     string
	taskStore    *tasks.Store
	runtimeMgr   *RuntimeManager
}

// NewDelegateTaskTool constructs the tool for use inside an agent loop.
func NewDelegateTaskTool(parentTaskID, tenantID string, ts *tasks.Store, rm *RuntimeManager) *DelegateTaskTool {
	return &DelegateTaskTool{
		parentTaskID: parentTaskID,
		tenantID:     tenantID,
		taskStore:    ts,
		runtimeMgr:   rm,
	}
}

func (t *DelegateTaskTool) Name() string { return "delegate_task" }

func (t *DelegateTaskTool) Description() string {
	return "Create a subtask and assign it to another Qor agent, which will start working on it immediately."
}

func (t *DelegateTaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":        map[string]any{"type": "string", "description": "Short title for the subtask"},
			"description":  map[string]any{"type": "string", "description": "Detailed description of the work required"},
			"agent_id":     map[string]any{"type": "string", "description": "UUID of the Qor agent to assign to"},
			"budget_cents": map[string]any{"type": "integer", "description": "Optional spend limit in cents (0 = no limit)", "default": 0},
		},
		"required": []string{"title", "description", "agent_id"},
	}
}

// Execute parses args, checks for cycles, creates the subtask, and wakes the
// target agent's runtime.
func (t *DelegateTaskTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	// --- parse required args ---
	title, _ := args["title"].(string)
	description, _ := args["description"].(string)
	agentID, _ := args["agent_id"].(string)

	if title == "" || description == "" || agentID == "" {
		return tools.ErrorResult("delegate_task: title, description, and agent_id are all required")
	}

	// --- optional budget ---
	var budgetCents int
	switch v := args["budget_cents"].(type) {
	case float64:
		budgetCents = int(v)
	case int:
		budgetCents = v
	case string:
		budgetCents, _ = strconv.Atoi(v)
	}

	// --- cycle detection ---
	maxDepth := defaultMaxCyclDepth
	if envMax := os.Getenv("AGENT_MAX_DELEGATION_DEPTH"); envMax != "" {
		if n, err := strconv.Atoi(envMax); err == nil && n > 0 {
			maxDepth = n
		}
	}
	if err := t.checkCycle(ctx, t.parentTaskID, maxDepth); err != nil {
		return tools.ErrorResult(fmt.Sprintf("delegate_task: cycle detected — %v", err))
	}

	// --- create subtask ---
	parentID := t.parentTaskID
	assignedTo := agentID
	subtask := tasks.Task{
		TenantID:    t.tenantID,
		ParentID:    &parentID,
		Title:       title,
		Description: description,
		AssignedTo:  &assignedTo,
		Status:      tasks.StatusAssigned,
		Priority:    3,
		BudgetCents: budgetCents,
	}

	newTaskID, err := t.taskStore.Create(ctx, t.tenantID, subtask)
	if err != nil {
		slog.Error("delegate_task: create failed", "parent", t.parentTaskID, "agent", agentID, "err", err)
		return tools.ErrorResult(fmt.Sprintf("delegate_task: failed to create subtask: %v", err))
	}

	slog.Info("delegate_task: subtask created",
		"task_id", newTaskID, "parent", t.parentTaskID,
		"assigned_to", agentID, "tenant", t.tenantID)

	// --- wake the target agent ---
	sig := WakeupSignal{
		Source:  WakeupAssignment,
		TaskID:  newTaskID,
		Message: fmt.Sprintf("New subtask delegated: %s", title),
		Context: map[string]any{
			"parent_task_id": t.parentTaskID,
			"task_id":        newTaskID,
		},
		Priority: 2,
	}

	woke := t.runtimeMgr.WakeAgent(agentID, sig)
	if !woke {
		// Runtime not yet started — ensure it exists and send again.
		slog.Warn("delegate_task: no runtime for agent, ensuring runtime", "agent", agentID)
		rt := t.runtimeMgr.EnsureRuntime(agentID, t.tenantID)
		rt.Send(sig)
	}

	return tools.SuccessResult(fmt.Sprintf(
		"Subtask created (id=%s) and assigned to agent %s — it will start immediately.",
		newTaskID, agentID,
	))
}

// checkCycle walks the parent_id chain from startID up to maxDepth steps.
// It returns an error if any ancestor equals agentID (which would mean the
// task graph would form a loop). Because task IDs are UUIDs we compare task
// IDs, not agent IDs — the goal is to ensure the new subtask's parent chain
// does not loop back to itself.
func (t *DelegateTaskTool) checkCycle(ctx context.Context, startID string, maxDepth int) error {
	visited := make(map[string]bool, maxDepth)
	currentID := startID

	for depth := 0; depth < maxDepth; depth++ {
		if currentID == "" {
			return nil // reached root — no cycle
		}
		if visited[currentID] {
			return fmt.Errorf("cycle at task %s (depth %d)", currentID, depth)
		}
		visited[currentID] = true

		task, err := t.taskStore.Get(ctx, currentID)
		if err != nil {
			// Can't walk further — assume no cycle (task may not exist yet).
			return nil
		}
		if task.ParentID == nil {
			return nil // root reached
		}
		currentID = *task.ParentID
	}

	return fmt.Errorf("delegation chain exceeds max depth %d — possible cycle", maxDepth)
}
