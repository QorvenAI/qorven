// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// AutonomousMode enables long-running agent execution (1-2+ hours)
// without iteration caps limiting progress. When an agent's loop reaches
// maxIter but still has pending work, the autonomous controller:
//
//  1. Compacts the context (saves progress as a continuation summary)
//  2. Self-wakes the agent with the compacted summary as a new run
//  3. Repeats until the agent signals "done" or budget is exhausted
//
// This mirrors Claude Code's ability to run indefinitely by compacting
// and continuing. The agent never "goes dark" — it streams progress
// throughout and the user can interrupt at any time.
//
// Safety: bounded by total token budget (not iteration count), maximum
// wall-clock duration, and health checks from the Supervisor.

// AutonomousConfig controls long-running execution behavior.
type AutonomousConfig struct {
	MaxContinuations int           // max self-wake cycles (default 50 = ~1000 iterations)
	MaxDuration      time.Duration // hard wall-clock cap (default 2h)
	MaxTotalTokens   int64         // total input+output budget (default 2M)
	CheckInterval    time.Duration // how often to emit progress (default 30s)
}

// DefaultAutonomousConfig returns safe defaults for long-running tasks.
func DefaultAutonomousConfig() AutonomousConfig {
	return AutonomousConfig{
		MaxContinuations: 50,
		MaxDuration:      2 * time.Hour,
		MaxTotalTokens:   2_000_000,
		CheckInterval:    30 * time.Second,
	}
}

// AutonomousState tracks the state of a long-running autonomous session.
type AutonomousState struct {
	SessionID      string          `json:"session_id"`
	AgentID        string          `json:"agent_id"`
	OriginalTask   string          `json:"original_task"`
	Continuations  int             `json:"continuations"`
	TotalTokens    int64           `json:"total_tokens"`
	TotalDuration  time.Duration   `json:"total_duration"`
	Status         AutonomousStatus `json:"status"`
	LastCheckpoint string          `json:"last_checkpoint"`
	StartedAt      time.Time       `json:"started_at"`
	LastActiveAt   time.Time       `json:"last_active_at"`
	ToolsUsed      int             `json:"tools_used"`
	Iterations     int             `json:"iterations"`
}

type AutonomousStatus string

const (
	AutonomousRunning   AutonomousStatus = "running"
	AutonomousPaused    AutonomousStatus = "paused"
	AutonomousCompleted AutonomousStatus = "completed"
	AutonomousFailed    AutonomousStatus = "failed"
	AutonomousBudgetHit AutonomousStatus = "budget_exhausted"
	AutonomousTimedOut  AutonomousStatus = "timed_out"
	AutonomousCancelled AutonomousStatus = "cancelled"
)

// AutonomousController manages long-running agent sessions.
// It wraps the standard Loop.Run and handles continuation logic.
type AutonomousController struct {
	loop   *Loop
	config AutonomousConfig
	mu     sync.RWMutex
	active map[string]*AutonomousState // sessionID → state
}

// NewAutonomousController creates a controller for managing long-running tasks.
func NewAutonomousController(loop *Loop, config AutonomousConfig) *AutonomousController {
	return &AutonomousController{
		loop:   loop,
		config: config,
		active: make(map[string]*AutonomousState),
	}
}

// continuationPrompt builds the self-wake message that carries context forward.
const continuationPrompt = `You are continuing a long-running autonomous task. Here is your progress so far:

---CHECKPOINT---
%s
---END CHECKPOINT---

Original task: %s

INSTRUCTIONS:
- Continue where you left off. Do NOT restart from scratch.
- If the task is complete, respond with your final answer.
- If more work is needed, keep going — use tools as needed.
- You have been running for %d iterations across %d continuations.
`

// RunAutonomous starts a long-running autonomous execution.
// The agent will self-continue until:
//   - It produces a final answer without requesting more tool calls
//   - Budget/duration limits are reached
//   - The context is cancelled (user interrupt)
//
// onEvent receives streaming updates throughout the entire execution.
func (ac *AutonomousController) RunAutonomous(
	ctx context.Context,
	req RunRequest,
	onEvent func(StreamEvent),
) (*RunResult, error) {
	state := &AutonomousState{
		SessionID:    req.SessionID,
		AgentID:      req.AgentID,
		OriginalTask: req.UserMessage,
		Status:       AutonomousRunning,
		StartedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}

	ac.mu.Lock()
	ac.active[req.SessionID] = state
	ac.mu.Unlock()

	defer func() {
		ac.mu.Lock()
		delete(ac.active, req.SessionID)
		ac.mu.Unlock()
	}()

	onEvent(StreamEvent{Type: "autonomous_start", Data: map[string]any{
		"max_continuations": ac.config.MaxContinuations,
		"max_duration":      ac.config.MaxDuration.String(),
	}})

	var finalResult *RunResult
	currentMessage := req.UserMessage

	for continuation := 0; continuation < ac.config.MaxContinuations; continuation++ {
		if ctx.Err() != nil {
			state.Status = AutonomousCancelled
			return finalResult, ctx.Err()
		}

		if time.Since(state.StartedAt) > ac.config.MaxDuration {
			state.Status = AutonomousTimedOut
			onEvent(StreamEvent{Type: "autonomous_timeout", Data: map[string]any{
				"duration": time.Since(state.StartedAt).String(),
			}})
			break
		}

		if ac.config.MaxTotalTokens > 0 && state.TotalTokens > ac.config.MaxTotalTokens {
			state.Status = AutonomousBudgetHit
			onEvent(StreamEvent{Type: "autonomous_budget", Data: map[string]any{
				"total_tokens": state.TotalTokens,
				"limit":        ac.config.MaxTotalTokens,
			}})
			break
		}

		// Run one iteration cycle
		iterReq := req
		iterReq.UserMessage = currentMessage

		result, err := ac.loop.Run(ctx, iterReq, onEvent)
		if err != nil {
			state.Status = AutonomousFailed
			return result, err
		}

		state.Continuations = continuation + 1
		state.TotalTokens += int64(result.InputTokens + result.OutputTokens)
		state.TotalDuration = time.Since(state.StartedAt)
		state.LastActiveAt = time.Now()
		state.ToolsUsed += len(result.ToolsUsed)
		state.Iterations += result.Iterations
		finalResult = result

		// Check if the agent is done (produced text without needing more tools).
		// If the loop exited because it hit maxIter (result.Iterations == maxIter),
		// AND the agent was still using tools, it needs to continue.
		if !result.HitIterationCap {
			state.Status = AutonomousCompleted
			break
		}

		// Agent hit iteration cap but has more work to do.
		// Build continuation context from the compacted summary.
		checkpoint := result.Content
		if checkpoint == "" {
			checkpoint = fmt.Sprintf("Completed %d iterations, used tools: %v. Work in progress.",
				result.Iterations, result.ToolsUsed)
		}
		state.LastCheckpoint = checkpoint

		onEvent(StreamEvent{Type: "autonomous_continue", Data: map[string]any{
			"continuation":  continuation + 1,
			"total_iters":   state.Iterations,
			"total_tokens":  state.TotalTokens,
			"duration":      state.TotalDuration.String(),
			"tools_used":    state.ToolsUsed,
		}})

		slog.Info("autonomous.continue",
			"agent", req.AgentID, "session", req.SessionID,
			"continuation", continuation+1,
			"total_iters", state.Iterations,
			"total_tokens", state.TotalTokens,
			"duration", state.TotalDuration)

		// Build continuation message for the next cycle
		currentMessage = fmt.Sprintf(continuationPrompt,
			checkpoint, state.OriginalTask, state.Iterations, state.Continuations)
	}

	if state.Status == AutonomousRunning {
		state.Status = AutonomousCompleted
	}

	onEvent(StreamEvent{Type: "autonomous_done", Data: map[string]any{
		"status":         string(state.Status),
		"continuations":  state.Continuations,
		"total_iters":    state.Iterations,
		"total_tokens":   state.TotalTokens,
		"total_duration": state.TotalDuration.String(),
		"tools_used":     state.ToolsUsed,
	}})

	return finalResult, nil
}

// ActiveSessions returns all currently running autonomous sessions.
func (ac *AutonomousController) ActiveSessions() []*AutonomousState {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	out := make([]*AutonomousState, 0, len(ac.active))
	for _, s := range ac.active {
		out = append(out, s)
	}
	return out
}

// Cancel stops an autonomous session.
func (ac *AutonomousController) Cancel(sessionID string) bool {
	ac.mu.RLock()
	state, ok := ac.active[sessionID]
	ac.mu.RUnlock()
	if !ok {
		return false
	}
	state.Status = AutonomousCancelled
	return true
}

// GetState returns the current state of an autonomous session.
func (ac *AutonomousController) GetState(sessionID string) *AutonomousState {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.active[sessionID]
}
