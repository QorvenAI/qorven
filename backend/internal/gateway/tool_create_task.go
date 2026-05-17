// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
)

type CreateTaskTool struct{ gw *Gateway }

func NewCreateTaskTool(gw *Gateway) *CreateTaskTool { return &CreateTaskTool{gw: gw} }

func (t *CreateTaskTool) Name() string { return "create_task" }

func (t *CreateTaskTool) Description() string {
	return "Create a background task for substantial work that will run in parallel with this conversation. " +
		"Use when the user asks for work requiring many steps or significant time. " +
		"The task runs autonomously — you can continue chatting while it executes. " +
		"Always tell the user you've created the task and what it will do."
}

func (t *CreateTaskTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Short task title, e.g. 'Scrape NTK election results'",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Full goal description with all context the worker needs",
			},
			"assign_to_self": map[string]any{
				"type":        "boolean",
				"description": "Assign this task to the current agent (default true)",
			},
		},
		"required": []string{"title", "description"},
	}
}

func (t *CreateTaskTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	title, _ := args["title"].(string)
	description, _ := args["description"].(string)
	assignToSelf := true
	if v, ok := args["assign_to_self"].(bool); ok {
		assignToSelf = v
	}
	if title == "" {
		return tools.ErrorResult("title is required")
	}
	if description == "" {
		return tools.ErrorResult("description is required")
	}

	agentID := tools.AgentIDFromCtx(ctx)
	tenantID := tools.TenantIDFromCtx(ctx)
	sessionID := tools.SessionIDFromCtx(ctx)
	discussionID := tools.DiscussionIDFromCtx(ctx)

	task := tasks.Task{
		TenantID:        tenantID,
		Title:           title,
		Description:     description,
		Status:          tasks.StatusBacklog,
		OriginSessionID: sessionID,
		DiscussionID:    discussionID,
	}
	if assignToSelf && agentID != "" {
		agentIDStr := agentID
		task.AssignedTo = &agentIDStr
		task.Status = tasks.StatusAssigned
	}

	taskID, err := t.gw.taskStore.Create(ctx, tenantID, task)
	if err != nil {
		slog.Error("create_task.store_failed", "error", err)
		return tools.ErrorResult("failed to create task: " + err.Error())
	}

	if assignToSelf && agentID != "" && t.gw.runtimeMgr != nil {
		t.gw.runtimeMgr.WakeAgent(agentID, agent.WakeupSignal{
			Source: agent.WakeupAssignment,
			TaskID: taskID,
		})
	}

	slog.Info("create_task.created", "id", taskID, "agent", agentID, "title", title)

	out, _ := json.Marshal(map[string]any{
		"task_id": taskID,
		"title":   title,
		"status":  task.Status,
		"message": "Task created. I will work on it in the background.",
	})
	return &tools.Result{ForLLM: string(out)}
}
