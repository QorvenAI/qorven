// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
)

const (
	taskMaxRetries   = 3
	taskRetryBackoff = 30 * time.Second
	taskIterTimeout  = 10 * time.Minute
)

// processTask is the top-level driver for autonomous task execution.
// It loops through iterations until the task reaches a terminal state
// (done, blocked, cancelled) or exhausts retries.
func (gw *Gateway) processTask(ctx context.Context, agentID string, taskID string) {
	if gw.taskStore == nil || gw.agentLoop == nil {
		slog.Warn("task_worker: store or loop unavailable", "task", taskID)
		return
	}

	retries := 0
	for {
		// Abort if context is done.
		if ctx.Err() != nil {
			slog.Info("task_worker: context cancelled", "task", taskID)
			return
		}

		// Fetch current task state.
		task, err := gw.taskStore.Get(ctx, taskID)
		if err != nil {
			slog.Error("task_worker: failed to fetch task", "task", taskID, "error", err)
			return
		}

		// Terminal state checks — stop the loop.
		switch task.Status {
		case tasks.StatusDone:
			slog.Info("task_worker: task already done", "task", taskID)
			// TODO(071): trigger synthesis when TaskCoordinator is available
			if gw.taskCoordinator != nil {
				gw.taskCoordinator.onTaskComplete(ctx, *task)
			}
			return
		case tasks.StatusCancelled:
			slog.Info("task_worker: task cancelled", "task", taskID)
			return
		case tasks.StatusBlocked:
			slog.Info("task_worker: task blocked — stopping iteration", "task", taskID)
			return
		}

		// Budget guard: stop if cost has exceeded the per-task budget.
		// NOTE: BudgetCents is int; CostCents is int64 — compare correctly.
		if task.BudgetCents > 0 && int(task.CostCents) >= task.BudgetCents {
			slog.Warn("task_worker: budget exceeded", "task", taskID,
				"cost_cents", task.CostCents, "budget_cents", task.BudgetCents)
			_ = gw.taskStore.Transition(ctx, taskID, tasks.StatusBlocked)
			broadcastTaskEvent(gw, taskID, agentID, realtime.EventTaskBlocked, map[string]any{
				"reason": "budget_exceeded",
			})
			return
		}

		// Transition task to in_progress if not already there.
		if task.Status != tasks.StatusInProgress {
			if err := gw.taskStore.Transition(ctx, taskID, tasks.StatusInProgress); err != nil {
				slog.Warn("task_worker: could not transition to in_progress", "task", taskID, "error", err)
			}
		}

		// Run one iteration with a per-iteration timeout.
		iterCtx, cancel := context.WithTimeout(ctx, taskIterTimeout)
		signal, iterErr := gw.runOneIteration(iterCtx, agentID, task)
		cancel()

		if iterErr != nil {
			retries++
			slog.Warn("task_worker: iteration error", "task", taskID,
				"error", iterErr, "retries", retries, "max", taskMaxRetries)
			if retries >= taskMaxRetries {
				slog.Error("task_worker: max retries exceeded — blocking task", "task", taskID)
				_ = gw.taskStore.Transition(ctx, taskID, tasks.StatusBlocked)
				broadcastTaskEvent(gw, taskID, agentID, realtime.EventTaskBlocked, map[string]any{
					"reason": fmt.Sprintf("max retries exceeded: %v", iterErr),
				})
				return
			}
			// Back off before retrying.
			select {
			case <-ctx.Done():
				return
			case <-time.After(taskRetryBackoff):
			}
			continue
		}

		// Reset retry counter on a clean iteration.
		retries = 0

		switch signal {
		case SignalDone:
			slog.Info("task_worker: task completed", "task", taskID)
			// TODO(071): trigger synthesis when TaskCoordinator is available
			if gw.taskCoordinator != nil {
				// Re-fetch to get final state with result populated.
				if finalTask, err := gw.taskStore.Get(ctx, taskID); err == nil {
					gw.taskCoordinator.onTaskComplete(ctx, *finalTask)
				}
			}
			return
		case SignalBlocked:
			slog.Info("task_worker: task blocked by agent", "task", taskID)
			return
		case SignalContinue:
			// Loop immediately — next iteration.
			slog.Debug("task_worker: continuing to next iteration", "task", taskID)
			continue
		default:
			// No signal received (agent finished without calling a task tool).
			// Treat as continue — agent may have done work but forgot to signal.
			slog.Warn("task_worker: no task signal received, treating as continue", "task", taskID, "signal", signal)
			continue
		}
	}
}

// runOneIteration executes a single agent iteration for the task.
// It injects task-lifecycle tools, builds the TEC prompt, runs the agent,
// and returns the TaskSignal the agent emitted (or an error).
func (gw *Gateway) runOneIteration(ctx context.Context, agentID string, task *tasks.Task) (TaskSignal, error) {
	// Increment iteration counter.
	iterNum, err := gw.taskStore.IncrementIteration(ctx, task.ID)
	if err != nil {
		slog.Warn("task_worker: could not increment iteration", "task", task.ID, "error", err)
		iterNum = task.IterationCount + 1
	}

	slog.Info("task_worker: starting iteration", "task", task.ID, "agent", agentID, "iteration", iterNum)

	// Emit iteration-start event so the UI can show live progress.
	broadcastTaskEvent(gw, task.ID, agentID, realtime.EventTaskIterationStart, map[string]any{
		"iteration": iterNum,
	})

	// Build the signal channel (buffered so tool Execute doesn't block).
	signalCh := make(taskSignalCh, 1)

	// Build per-task lifecycle tools.
	taskTools := gw.buildTaskTools(task.ID, agentID, signalCh)

	// Build the exec tool scoped to this task's workspace.
	execTool := gw.buildExecToolForTask(task.ID)
	if execTool != nil {
		taskTools = append(taskTools, execTool)
	}

	// Build task execution context (TEC) message.
	tec := buildTEC(task)

	// Build the run request.
	req := agent.RunRequest{
		AgentID:       agentID,
		UserMessage:   tec,
		TenantID:      defaultTenant,
		Channel:       "task",
		ExtraTools:    taskTools,
		SessionID:     task.OriginSessionID,
		NoPersist:     task.OriginSessionID == "",
		DiscussionID:  task.DiscussionID,
		SourceChannel: "task",
	}

	// Track tool activity for signal detection.
	var lastSignal TaskSignal

	_, runErr := gw.agentLoop.Run(ctx, req, func(event agent.StreamEvent) {
		// Broadcast tool-start events to the realtime hub.
		if event.Type == agent.EventTypeToolStart && gw.rtHub != nil {
			gw.rtHub.Broadcast(realtime.Event{
				Type:    realtime.EventTaskToolCall,
				AgentID: agentID,
				Data: map[string]any{
					"task_id":   task.ID,
					"tool":      event.Tool,
					"iteration": iterNum,
				},
				Timestamp: time.Now().UnixMilli(),
			})
		}
	})

	if runErr != nil {
		return "", fmt.Errorf("agent run failed (iteration %d): %w", iterNum, runErr)
	}

	// Drain the signal channel — the task tool fires it during Execute.
	select {
	case msg := <-signalCh:
		lastSignal = msg.Signal
	default:
		// No signal — agent didn't call a task lifecycle tool.
		lastSignal = SignalContinue
	}

	return lastSignal, nil
}

// buildTEC constructs the XML task execution context that is prepended
// to every iteration's user message so the agent knows its context.
func buildTEC(task *tasks.Task) string {
	idSnip := task.ID
	if len(idSnip) > 8 {
		idSnip = idSnip[:8]
	}
	return fmt.Sprintf(`<task id=%q status=%q iteration="%d">
  <title>%s</title>
  <description>%s</description>
  <scratchpad>%s</scratchpad>
</task>

You are working the above task autonomously. Rules:
1. Use task_continue to save progress and request another iteration.
2. Use task_done when you have fully completed the task — provide a clear result.
3. Use task_blocked when you cannot proceed without human input.
4. Use task_update_scratchpad to checkpoint state mid-iteration.
5. Do not stop without calling one of these tools.
6. Keep the scratchpad up-to-date so interrupted tasks can be resumed.`,
		idSnip,
		task.Status,
		task.IterationCount+1,
		task.Title,
		task.Description,
		task.Scratchpad,
	)
}

// buildExecToolForTask returns an exec tool scoped to <WorkspaceRoot>/task-<id[:8]>/.
// The workspace directory is created on demand.
func (gw *Gateway) buildExecToolForTask(taskID string) tools.Tool {
	idSnip := taskID
	if len(idSnip) > 8 {
		idSnip = idSnip[:8]
	}
	workspace := fmt.Sprintf("%s/task-%s", tools.WorkspaceRoot(), idSnip)
	if err := os.MkdirAll(workspace, 0o750); err != nil {
		slog.Warn("task_worker: could not create workspace", "workspace", workspace, "error", err)
		return nil
	}
	// restrict=true: commands are limited to the task workspace.
	return tools.NewExecTool(workspace, true)
}
