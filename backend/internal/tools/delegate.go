// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// DelegateTool allows an agent (typically Prime) to delegate tasks to specialist agents.
// The specialist runs the task and returns the result.
type DelegateTool struct {
	runAgent func(ctx context.Context, agentKey, message string) (string, error)
	listAgents func(ctx context.Context) ([]map[string]any, error)
}

// NewDelegateTool creates a delegation tool.
func NewDelegateTool(
	runAgent func(ctx context.Context, agentKey, message string) (string, error),
	listAgents func(ctx context.Context) ([]map[string]any, error),
) *DelegateTool {
	return &DelegateTool{runAgent: runAgent, listAgents: listAgents}
}

func (t *DelegateTool) Name() string { return "delegate" }

func (t *DelegateTool) Description() string {
	return "Delegate a task to a specialist agent. The specialist will execute the task and return the result. Use this when the task requires expertise you don't have, or when you want to parallelize work."
}

func (t *DelegateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"description": "The agent key to delegate to (e.g. 'researcher', 'writer', 'analyst')",
			},
			"task": map[string]any{
				"type":        "string",
				"description": "The task description — what the specialist should do",
			},
			"context": map[string]any{
				"type":        "string",
				"description": "Additional context or data the specialist needs",
			},
		},
		"required": []string{"agent", "task"},
	}
}

func (t *DelegateTool) Execute(ctx context.Context, args map[string]any) *Result {
	agentKey, _ := args["agent"].(string)
	task, _ := args["task"].(string)
	extraCtx, _ := args["context"].(string)

	if agentKey == "" || task == "" {
		return ErrorResult("agent and task are required")
	}

	if t.runAgent == nil {
		return ErrorResult("delegation not configured")
	}

	message := task
	if extraCtx != "" {
		message = fmt.Sprintf("%s\n\nContext:\n%s", task, extraCtx)
	}

	slog.Info("delegate.start", "to", agentKey, "task", task[:min(len(task), 80)])

	result, err := t.runAgent(ctx, agentKey, message)
	if err != nil {
		slog.Warn("delegate.failed", "to", agentKey, "error", err)
		return ErrorResult(fmt.Sprintf("Delegation to %s failed: %v", agentKey, err))
	}

	slog.Info("delegate.complete", "to", agentKey, "result_len", len(result))
	return TextResult(fmt.Sprintf("[Result from %s]\n%s", agentKey, result))
}

// ListAgentsTool returns available agents for delegation.
type ListAgentsTool struct {
	listAgents func(ctx context.Context) ([]map[string]any, error)
}

func NewListAgentsTool(listAgents func(ctx context.Context) ([]map[string]any, error)) *ListAgentsTool {
	return &ListAgentsTool{listAgents: listAgents}
}

func (t *ListAgentsTool) Name() string        { return "list_agents" }
func (t *ListAgentsTool) Description() string  { return "List available specialist agents you can delegate tasks to." }
func (t *ListAgentsTool) Parameters() map[string]any { return map[string]any{"type": "object", "properties": map[string]any{}} }

func (t *ListAgentsTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.listAgents == nil {
		return ErrorResult("agent listing not configured")
	}
	agents, err := t.listAgents(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.MarshalIndent(agents, "", "  ")
	return TextResult(string(data))
}

