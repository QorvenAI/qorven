// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package souldesk

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/qorvenai/qorven/internal/tools"
)

// DispatchTeamTasksTool lets Prime split a plan into parallel subtasks and
// assign each to the right Soul in one call. All tasks start concurrently;
// Prime receives a single batched summary once they are dispatched.
//
// This is the multi-agent task-splitting primitive: brief → subtask list →
// parallel delegation in one LLM turn.
type DispatchTeamTasksTool struct {
	desk *SoulDesk
}

func NewDispatchTeamTasksTool(desk *SoulDesk) *DispatchTeamTasksTool {
	return &DispatchTeamTasksTool{desk: desk}
}

func (t *DispatchTeamTasksTool) Name() string { return "dispatch_team_tasks" }
func (t *DispatchTeamTasksTool) Description() string {
	return `Split a plan into parallel subtasks and assign each to the right Soul simultaneously.
Use this instead of multiple delegate_to_soul calls when you want to kick off
several independent workstreams at once. All tasks start in parallel.

Each item in the tasks array must specify:
- soul_key: which Soul to assign the task to
- task: clear description of what the Soul should do
- context (optional): background or data the Soul needs

Returns immediately with a dispatch summary. Results arrive asynchronously.`
}

func (t *DispatchTeamTasksTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tasks": map[string]any{
				"type":        "array",
				"description": "List of tasks to dispatch in parallel",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"soul_key": map[string]any{
							"type":        "string",
							"description": "The agent_key of the Soul to assign this task to",
						},
						"task": map[string]any{
							"type":        "string",
							"description": "Clear description of what the Soul should do",
						},
						"context": map[string]any{
							"type":        "string",
							"description": "Background info or data the Soul needs",
						},
					},
					"required": []string{"soul_key", "task"},
				},
				"minItems": 1,
				"maxItems": 10,
			},
			"plan_title": map[string]any{
				"type":        "string",
				"description": "Optional label for this batch of work (e.g. 'Customer Portal v1')",
			},
		},
		"required": []string{"tasks"},
	}
}

func (t *DispatchTeamTasksTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	rawTasks, ok := args["tasks"].([]any)
	if !ok || len(rawTasks) == 0 {
		return tools.ErrorResult("tasks array is required and must not be empty")
	}
	planTitle, _ := args["plan_title"].(string)

	type taskSpec struct {
		soulKey string
		task    string
		context string
	}

	primeID := tools.AgentIDFromCtx(ctx)
	sessionID := tools.SessionIDFromCtx(ctx)

	specs := []taskSpec{}
	for _, raw := range rawTasks {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		soulKey, _ := m["soul_key"].(string)
		task, _ := m["task"].(string)
		taskCtx, _ := m["context"].(string)
		if soulKey == "" || task == "" {
			continue
		}
		specs = append(specs, taskSpec{soulKey: soulKey, task: task, context: taskCtx})
	}

	if len(specs) == 0 {
		return tools.ErrorResult("no valid task entries found — each must have soul_key and task")
	}

	// Verify all souls exist before starting anything.
	missing := []string{}
	for _, s := range specs {
		if _, err := t.desk.findSoul(ctx, s.soulKey); err != nil {
			missing = append(missing, "@"+s.soulKey)
		}
	}
	if len(missing) > 0 {
		return tools.ErrorResult(fmt.Sprintf("souls not found: %s — create them first with create_soul", strings.Join(missing, ", ")))
	}

	// Dispatch all tasks in parallel — fire and forget, same as delegate_to_soul.
	var wg sync.WaitGroup
	dispatched := make([]string, 0, len(specs))

	for _, spec := range specs {
		soul, err := t.desk.findSoul(ctx, spec.soulKey)
		if err != nil {
			continue
		}

		taskSpec := spec   // capture loop variable
		soulCopy := soul   // capture loop variable
		wg.Add(1)
		if t.desk.rtHub != nil {
			t.desk.rtHub.BroadcastSoulActivity(soulCopy.ID, taskSpec.soulKey, "working", truncateStr(taskSpec.task, 100))
		}
		go func() {
			defer wg.Done()
			message := taskSpec.task
			if taskSpec.context != "" {
				message = fmt.Sprintf("Task: %s\n\nContext: %s", taskSpec.task, taskSpec.context)
			}
			result, err := t.desk.runSoul(context.Background(), soulCopy, message, "")
			dr := delegationResult{
				SoulKey:  taskSpec.soulKey,
				SoulName: soul.DisplayName,
				Task:     taskSpec.task,
				Err:      err,
			}
			if err != nil {
				dr.Result = "Error: " + err.Error()
			} else {
				dr.Result = result
			}
			if t.desk.rtHub != nil {
				status := "completed"
				if err != nil {
					status = "failed"
				}
				t.desk.rtHub.BroadcastSoulActivity(soulCopy.ID, taskSpec.soulKey, status, dr.Result[:min(len(dr.Result), 200)])
			}
			if primeID != "" {
				var taskID string
				if t.desk.taskInteg != nil {
					taskID, _ = t.desk.taskInteg.CreateDelegationTask(context.Background(), soulCopy.ID, taskSpec.soulKey, primeID, taskSpec.task)
					if taskID != "" {
						t.desk.taskInteg.CompleteDelegationTask(context.Background(), taskID, dr.Result, 0)
					}
				}
				t.desk.deliverResult(context.Background(), primeID, sessionID, taskID, dr)
			}
		}()
		dispatched = append(dispatched, fmt.Sprintf("@%s — %s", spec.soulKey, truncateStr(spec.task, 80)))
	}

	// Don't wait — return immediately so Prime isn't blocked.
	// Results arrive via the announce queue / agent_messages.
	go wg.Wait()

	var sb strings.Builder
	if planTitle != "" {
		sb.WriteString(fmt.Sprintf("Dispatched %d tasks for \"%s\":\n\n", len(dispatched), planTitle))
	} else {
		sb.WriteString(fmt.Sprintf("Dispatched %d tasks in parallel:\n\n", len(dispatched)))
	}
	for i, d := range dispatched {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, d))
	}
	sb.WriteString("\nAll tasks are running concurrently. Use check_updates to see results as they complete.")

	return tools.TextResult(sb.String())
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
