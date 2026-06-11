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
	"github.com/qorvenai/qorven/internal/governance"
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

		// Kill-switch: cancelled flag set directly on the row (bypasses status FSM).
		if task.Cancelled {
			slog.Info("task_worker: cancelled flag set — stopping", "task", taskID)
			_ = gw.taskStore.Transition(ctx, taskID, tasks.StatusCancelled)
			return
		}

		// Hard iteration cap: stop and block when the task has hit its max turns.
		// TODO(Task 9): also check for a STOP file in the worktree root as a kill-switch.
		if task.MaxIterations > 0 && task.IterationCount >= task.MaxIterations {
			slog.Warn("task_worker: iteration cap reached — blocking", "task", taskID,
				"iteration_count", task.IterationCount, "max_iterations", task.MaxIterations)
			_ = gw.taskStore.Transition(ctx, taskID, tasks.StatusBlocked)
			broadcastTaskEvent(gw, taskID, agentID, realtime.EventTaskBlocked, map[string]any{
				"reason": "iteration_cap",
			})
			if briefID := gw.projectBriefForTask(ctx, taskID); briefID != "" {
				gw.emitProjectEvent(ctx, briefID, "blocked", task.Title, map[string]any{
					"reason": "iteration_cap",
				}, taskID, agentID)
			}
			return
		}

		// Budget guard: stop if cost has exceeded the per-task budget.
		// NOTE: BudgetCents is int; CostCents is int64 — compare correctly.
		if task.BudgetCents > 0 && int(task.CostCents) >= task.BudgetCents {
			slog.Warn("task_worker: budget exceeded", "task", taskID,
				"cost_cents", task.CostCents, "budget_cents", task.BudgetCents)
			_ = gw.taskStore.Transition(ctx, taskID, tasks.StatusBlocked)
			if gw.exceptionStore != nil {
				gw.exceptionStore.Record(ctx, governance.Exception{
					TenantID:    defaultTenant,
					AgentID:     agentID,
					Category:    "budget_overrun",
					Severity:    "warning",
					Description: fmt.Sprintf("Task %s budget exceeded: spent %d of %d cents", taskID, task.CostCents, task.BudgetCents),
					Context:     map[string]any{"task_id": taskID, "cost_cents": task.CostCents, "budget_cents": task.BudgetCents},
				})
			}
			broadcastTaskEvent(gw, taskID, agentID, realtime.EventTaskBlocked, map[string]any{
				"reason": "budget_exceeded",
			})
			// Durable project-scoped event (best-effort).
			if briefID := gw.projectBriefForTask(ctx, taskID); briefID != "" {
				gw.emitProjectEvent(ctx, briefID, "blocked", task.Title, map[string]any{
					"reason": "budget_exceeded",
				}, taskID, agentID)
			}
			return
		}

		// Transition task to in_progress if not already there.
		if task.Status != tasks.StatusInProgress {
			if err := gw.taskStore.Transition(ctx, taskID, tasks.StatusInProgress); err != nil {
				slog.Warn("task_worker: could not transition to in_progress", "task", taskID, "error", err)
			}
			// Durable project-scoped event for task start (best-effort).
			if briefID := gw.projectBriefForTask(ctx, taskID); briefID != "" {
				gw.emitProjectEvent(ctx, briefID, "task_started", task.Title, map[string]any{
					"status": "in_progress",
				}, taskID, agentID)
			}
		}

		// Claim the lease: stamp lease_expires + locked_by so the watchdog knows
		// this worker is alive. Done once per loop turn (not per-iteration) so that
		// even a fast task always has a fresh lease before work starts.
		if gw.db != nil && gw.db.Pool != nil {
			_, _ = gw.db.Pool.Exec(ctx,
				`UPDATE tasks SET lease_expires = NOW() + INTERVAL '3 minutes', locked_by = $2 WHERE id = $1`,
				taskID, agentID)
		}

		// Run one iteration with a per-iteration timeout, keeping the lease alive
		// via a background heartbeat goroutine that ticks every 20 seconds.
		iterCtx, cancel := context.WithTimeout(ctx, taskIterTimeout)

		hbCtx, hbStop := context.WithCancel(ctx)
		go func() {
			t := time.NewTicker(20 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-hbCtx.Done():
					return
				case <-t.C:
					if gw.db != nil && gw.db.Pool != nil {
						_, _ = gw.db.Pool.Exec(context.Background(),
							`UPDATE tasks SET last_heartbeat_at = NOW(), lease_expires = NOW() + INTERVAL '3 minutes', updated_at = NOW() WHERE id = $1`,
							taskID)
					}
				}
			}
		}()

		signal, iterErr := gw.runOneIteration(iterCtx, agentID, task)
		hbStop()
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
				// Durable project-scoped event (best-effort).
				if briefID := gw.projectBriefForTask(ctx, taskID); briefID != "" {
					gw.emitProjectEvent(ctx, briefID, "blocked", task.Title, map[string]any{
						"reason": fmt.Sprintf("max retries exceeded: %v", iterErr),
					}, taskID, agentID)
				}
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
			if gw.taskStateMachine != nil {
				gw.taskStateMachine.Transition(ctx, defaultTenant, taskID, "in_progress", "completed", agentID, "task done")
			}
			if gw.taskCoordinator != nil {
				// Re-fetch to get final state with result populated.
				if finalTask, err := gw.taskStore.Get(ctx, taskID); err == nil {
					gw.taskCoordinator.onTaskComplete(ctx, *finalTask)
				}
			}
			return
		case SignalBlocked:
			slog.Info("task_worker: task blocked by agent", "task", taskID)
			if gw.taskStateMachine != nil {
				gw.taskStateMachine.Transition(ctx, defaultTenant, taskID, "in_progress", "blocked", agentID, "agent reported blocked")
			}
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

	// Inject delegate_task for agents with can_delegate=true (L1→L2→L3 hierarchy).
	if gw.agents != nil && gw.runtimeMgr != nil {
		if a, err := gw.agents.Get(ctx, agentID); err == nil && a.CanDelegate {
			taskTools = append(taskTools, agent.NewDelegateTaskTool(task.ID, defaultTenant, gw.taskStore, gw.runtimeMgr))
		}
	}

	// Build the exec tool scoped to this task's workspace.
	execTool := gw.buildExecToolForTask(task.ID)
	if execTool != nil {
		taskTools = append(taskTools, execTool)
	}

	// Fetch subtask results for synthesis context (empty if no subtasks exist).
	var subtaskResults []tasks.Task
	if gw.taskStore != nil {
		subtaskResults, _ = gw.taskStore.GetSubtasks(ctx, task.ID)
	}

	// Build task execution context (TEC) message.
	tec := buildTEC(task, subtaskResults)

	// Build the run request.
	req := agent.RunRequest{
		AgentID:       agentID,
		UserMessage:   tec,
		TenantID:      defaultTenant,
		Channel:       "task",
		ExtraTools:    taskTools,
		SessionID:     task.OriginSessionID,
		TaskID:        task.ID,
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
func buildTEC(task *tasks.Task, subtaskResults []tasks.Task) string {
	idSnip := task.ID
	if len(idSnip) > 8 {
		idSnip = idSnip[:8]
	}

	var subtaskSection string
	if len(subtaskResults) > 0 {
		subtaskSection = "\n  <subtask_results>\n"
		for _, st := range subtaskResults {
			status := string(st.Status)
			result := st.Result
			if len(result) > 500 {
				result = result[:500] + "…"
			}
			subtaskSection += fmt.Sprintf("    <subtask id=%q title=%q status=%q>%s</subtask>\n",
				st.ID[:8], st.Title, status, result)
		}
		subtaskSection += "  </subtask_results>"
	}

	delegateRule := ""
	if task.ParentID == nil {
		delegateRule = "\n7. Use delegate_task to assign sub-work to specialist agents (if available)."
	}

	return fmt.Sprintf(`<task id=%q status=%q iteration="%d">
  <title>%s</title>
  <description>%s</description>
  <scratchpad>%s</scratchpad>%s
</task>

You are working the above task autonomously. Rules:
1. Use task_continue to save progress and request another iteration.
2. Use task_done when you have fully completed the task — provide a clear result.
3. Use task_blocked when you cannot proceed without human input.
4. Use task_update_scratchpad to checkpoint state mid-iteration.
5. Do not stop without calling one of these tools.
6. Keep the scratchpad up-to-date so interrupted tasks can be resumed.%s`,
		idSnip,
		task.Status,
		task.IterationCount+1,
		task.Title,
		task.Description,
		task.Scratchpad,
		subtaskSection,
		delegateRule,
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
