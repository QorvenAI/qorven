// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
)

// TaskSignal is the outcome an agent signals when it calls a task_* tool.
type TaskSignal string

const (
	SignalContinue TaskSignal = "continue"
	SignalDone     TaskSignal = "done"
	SignalBlocked  TaskSignal = "blocked"
)

// taskSignalMsg carries the signal and its optional payload (result or reason).
type taskSignalMsg struct {
	Signal  TaskSignal
	Payload string
}

// taskSignalCh is the channel used by task tools to communicate back to the
// iteration loop that drives the agent run.
type taskSignalCh chan taskSignalMsg

// buildTaskTools returns the four task lifecycle tools that are injected into
// an agent's tool set while it is working on an autonomous task.
//
// The signalCh is drained by the caller (typically the iteration loop in the
// task worker) — it must be buffered by at least 1 so the Execute call does
// not block when the loop isn't actively waiting.
func (gw *Gateway) buildTaskTools(taskID, agentID string, signalCh taskSignalCh) []tools.Tool {
	return []tools.Tool{
		&taskContinueTool{gw: gw, taskID: taskID, agentID: agentID, signalCh: signalCh},
		&taskDoneTool{gw: gw, taskID: taskID, agentID: agentID, signalCh: signalCh},
		&taskBlockedTool{gw: gw, taskID: taskID, agentID: agentID, signalCh: signalCh},
		&taskUpdateScratchpadTool{gw: gw, taskID: taskID, agentID: agentID},
	}
}

// ─── task_continue ────────────────────────────────────────────────────────────

type taskContinueTool struct {
	gw       *Gateway
	taskID   string
	agentID  string
	signalCh taskSignalCh
}

func (t *taskContinueTool) Name() string        { return "task_continue" }
func (t *taskContinueTool) Description() string {
	return "Save the current scratchpad and signal that more work remains. " +
		"The runtime will schedule the next iteration immediately."
}
func (t *taskContinueTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"scratchpad": map[string]any{
				"type":        "string",
				"description": "Current working notes / state to persist before the next iteration.",
			},
		},
		"required": []string{"scratchpad"},
	}
}
func (t *taskContinueTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	scratchpad, _ := args["scratchpad"].(string)

	if t.gw.taskStore != nil && scratchpad != "" {
		if err := t.gw.taskStore.UpdateScratchpad(ctx, t.taskID, scratchpad); err != nil {
			slog.Warn("task_continue: scratchpad save failed", "task", t.taskID, "error", err)
		}
	}

	// Log audit event
	logTaskEvent(ctx, t.gw.taskStore, t.taskID, t.agentID, "continue", map[string]any{
		"scratchpad_len": len(scratchpad),
	})

	// Broadcast progress
	broadcastTaskEvent(t.gw, t.taskID, t.agentID, realtime.EventTaskProgress, map[string]any{
		"status": "continuing",
	})

	// Signal the iteration loop
	select {
	case t.signalCh <- taskSignalMsg{Signal: SignalContinue, Payload: scratchpad}:
	default:
		slog.Warn("task_continue: signal channel full", "task", t.taskID)
	}

	return tools.TextResult("continuing — next iteration scheduled")
}

// ─── task_done ────────────────────────────────────────────────────────────────

type taskDoneTool struct {
	gw       *Gateway
	taskID   string
	agentID  string
	signalCh taskSignalCh
}

func (t *taskDoneTool) Name() string        { return "task_done" }
func (t *taskDoneTool) Description() string {
	return "Mark the task as complete and deliver the final result."
}
func (t *taskDoneTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"result": map[string]any{
				"type":        "string",
				"description": "The final result or summary to store against the task.",
			},
		},
		"required": []string{"result"},
	}
}
func (t *taskDoneTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	result, _ := args["result"].(string)

	if t.gw.taskStore != nil {
		if err := t.gw.taskStore.Complete(ctx, t.taskID, result, 0); err != nil {
			slog.Error("task_done: Complete failed", "task", t.taskID, "error", err)
			return tools.ErrorResult(fmt.Sprintf("failed to mark task done: %v", err))
		}
	}

	// Log audit event
	logTaskEvent(ctx, t.gw.taskStore, t.taskID, t.agentID, "done", map[string]any{
		"result_len": len(result),
	})

	// Broadcast completion
	broadcastTaskEvent(t.gw, t.taskID, t.agentID, realtime.EventTaskDone, map[string]any{
		"result": result,
	})

	// Signal the iteration loop — non-blocking; loop may already be closing
	select {
	case t.signalCh <- taskSignalMsg{Signal: SignalDone, Payload: result}:
	default:
		slog.Warn("task_done: signal channel full", "task", t.taskID)
	}

	return tools.TextResult("task marked as done")
}

// ─── task_blocked ─────────────────────────────────────────────────────────────

type taskBlockedTool struct {
	gw       *Gateway
	taskID   string
	agentID  string
	signalCh taskSignalCh
}

func (t *taskBlockedTool) Name() string        { return "task_blocked" }
func (t *taskBlockedTool) Description() string {
	return "Mark the task as blocked — use when you cannot proceed without human input or an external dependency."
}
func (t *taskBlockedTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"reason": map[string]any{
				"type":        "string",
				"description": "Clear explanation of what is blocking progress.",
			},
		},
		"required": []string{"reason"},
	}
}
func (t *taskBlockedTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	reason, _ := args["reason"].(string)

	if t.gw.taskStore != nil {
		if err := t.gw.taskStore.Transition(ctx, t.taskID, tasks.StatusBlocked); err != nil {
			slog.Error("task_blocked: Transition failed", "task", t.taskID, "error", err)
			return tools.ErrorResult(fmt.Sprintf("failed to mark task blocked: %v", err))
		}
	}

	// Log audit event
	logTaskEvent(ctx, t.gw.taskStore, t.taskID, t.agentID, "blocked", map[string]any{
		"reason": reason,
	})

	// Broadcast blocked state
	broadcastTaskEvent(t.gw, t.taskID, t.agentID, realtime.EventTaskBlocked, map[string]any{
		"reason": reason,
	})

	// Signal the iteration loop
	select {
	case t.signalCh <- taskSignalMsg{Signal: SignalBlocked, Payload: reason}:
	default:
		slog.Warn("task_blocked: signal channel full", "task", t.taskID)
	}

	return tools.TextResult("task marked as blocked — awaiting unblock")
}

// ─── task_update_scratchpad ───────────────────────────────────────────────────

type taskUpdateScratchpadTool struct {
	gw      *Gateway
	taskID  string
	agentID string
}

func (t *taskUpdateScratchpadTool) Name() string        { return "task_update_scratchpad" }
func (t *taskUpdateScratchpadTool) Description() string {
	return "Checkpoint the scratchpad mid-iteration without ending the run. " +
		"Use this to persist progress so it survives unexpected interruptions."
}
func (t *taskUpdateScratchpadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"scratchpad": map[string]any{
				"type":        "string",
				"description": "Current working notes / state to persist.",
			},
		},
		"required": []string{"scratchpad"},
	}
}
func (t *taskUpdateScratchpadTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	scratchpad, _ := args["scratchpad"].(string)

	if t.gw.taskStore == nil {
		return tools.ErrorResult("task store unavailable")
	}

	if err := t.gw.taskStore.UpdateScratchpad(ctx, t.taskID, scratchpad); err != nil {
		slog.Error("task_update_scratchpad: failed", "task", t.taskID, "error", err)
		return tools.ErrorResult(fmt.Sprintf("scratchpad update failed: %v", err))
	}

	// Broadcast progress heartbeat
	broadcastTaskEvent(t.gw, t.taskID, t.agentID, realtime.EventTaskProgress, map[string]any{
		"scratchpad_len": len(scratchpad),
		"checkpoint":     true,
	})

	return tools.TextResult(fmt.Sprintf("scratchpad updated (%d bytes)", len(scratchpad)))
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// broadcastTaskEvent fans out a realtime event via the hub when one is available.
func broadcastTaskEvent(gw *Gateway, taskID, agentID, eventType string, data map[string]any) {
	if gw.rtHub == nil {
		return
	}
	payload := make(map[string]any, len(data)+2)
	for k, v := range data {
		payload[k] = v
	}
	payload["task_id"] = taskID
	payload["agent_id"] = agentID

	gw.rtHub.Broadcast(realtime.Event{
		Type:      eventType,
		AgentID:   agentID,
		Data:      payload,
		Timestamp: time.Now().UnixMilli(),
	})
}

// logTaskEvent writes a non-blocking audit entry to task_events; any error is
// only logged — it must not abort the tool call.
func logTaskEvent(ctx context.Context, store *tasks.Store, taskID, agentID, eventType string, payload map[string]any) {
	if store == nil {
		return
	}
	if err := store.LogEvent(ctx, tasks.TaskEvent{
		TaskID:    taskID,
		AgentID:   agentID,
		EventType: eventType,
		Payload:   payload,
	}); err != nil {
		slog.Warn("task.logEvent failed", "task", taskID, "event", eventType, "error", err)
	}
}
