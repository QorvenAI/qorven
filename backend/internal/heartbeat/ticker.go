// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package heartbeat

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/tasks"
)

const pollInterval = 30 * time.Second

// Ticker polls for due heartbeats and runs them.
type Ticker struct {
	store     *Store
	taskStore *tasks.Store
	tenantID  string
	runFn     func(ctx context.Context, cfg Config) RunResult
	onEvent   func(Event)           // lifecycle events
	wakeCh    chan string           // immediate wake trigger
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// Event represents a heartbeat lifecycle event.
type Event struct {
	Action   string `json:"action"`   // running, completed, suppressed, error, skipped
	AgentID  string `json:"agent_id"`
	Status   string `json:"status,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Error    string `json:"error,omitempty"`
}

func NewTicker(store *Store, taskStore *tasks.Store, tenantID string, runFn func(ctx context.Context, cfg Config) RunResult) *Ticker {
	return &Ticker{
		store:     store,
		taskStore: taskStore,
		tenantID:  tenantID,
		runFn:     runFn,
		wakeCh:    make(chan string, 16),
		stopCh:    make(chan struct{}),
	}
}

// SetOnEvent sets the event callback for lifecycle events.
func (t *Ticker) SetOnEvent(fn func(Event)) { t.onEvent = fn }

func (t *Ticker) emitEvent(e Event) {
	if t.onEvent != nil { t.onEvent(e) }
}

// Wake triggers an immediate heartbeat run for a specific agent.
func (t *Ticker) Wake(agentID string) {
	select {
	case t.wakeCh <- agentID:
	default: // channel full, skip
	}
}

// Start begins the background poll loop.
func (t *Ticker) Start() {
	t.wg.Add(1)
	go t.loop()
	slog.Info("heartbeat ticker started", "poll_interval", pollInterval)
}

// Stop signals the poll loop to exit.
func (t *Ticker) Stop() {
	close(t.stopCh)
	t.wg.Wait()
	slog.Info("heartbeat ticker stopped")
}

func (t *Ticker) loop() {
	defer t.wg.Done()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.tick()
		case agentID := <-t.wakeCh:
			go t.runOneByAgentID(agentID)
		case <-t.stopCh:
			return
		}
	}
}

func (t *Ticker) tick() {
	ctx := context.Background()
	now := time.Now()

	due, err := t.store.ListDue(ctx, now)
	if err != nil {
		slog.Warn("heartbeat: failed to list due", "error", err)
		return
	}

	for _, cfg := range due {
		// Check active hours
		if !IsWithinActiveHours(&cfg) {
			t.logSkipped(ctx, cfg, "active_hours")
			t.advanceNextRun(ctx, cfg)
			continue
		}

		// Run heartbeat in goroutine (don't block the ticker)
		go t.runOne(ctx, cfg)
	}
}

func (t *Ticker) runOneByAgentID(agentID string) {
	ctx := context.Background()
	cfg, err := t.store.Get(ctx, agentID)
	if err != nil {
		slog.Warn("heartbeat.wake_get_failed", "agent_id", agentID, "error", err)
		return
	}
	t.runOne(ctx, *cfg)
}

func (t *Ticker) runOne(ctx context.Context, cfg Config) {
	start := time.Now()
	slog.Info("heartbeat: running", "agent", cfg.AgentID, "state", cfg.CurrentState)

	t.emitEvent(Event{Action: "running", AgentID: cfg.AgentID})

	// Execute the 5-phase heartbeat
	result := t.runFn(ctx, cfg)
	result.Duration = time.Since(start)

	// Smart suppression: check for HEARTBEAT_OK token
	if result.Summary != "" && strings.Contains(result.Summary, "HEARTBEAT_OK") {
		result.Status = "suppressed"
		result.Summary = "" // don't deliver
	}

	// Update state in DB
	next := time.Now().Add(time.Duration(cfg.IntervalSec)*time.Second + StaggerOffset(cfg.AgentID, cfg.IntervalSec))
	lastErr := result.Error
	t.store.UpdateState(ctx, cfg.ID, result.PolicyState, cfg.ConsecFailures, cfg.ConsecPasses, cfg.RunCount+1, next, result.Status, lastErr)

	// Log the run
	t.store.LogRun(ctx, t.tenantID, result, cfg.ID, cfg.AgentID)

	// Emit completion event
	t.emitEvent(Event{
		Action:  result.Status,
		AgentID: cfg.AgentID,
		Status:  result.Status,
		Error:   result.Error,
	})

	slog.Info("heartbeat: complete",
		"agent", cfg.AgentID,
		"status", result.Status,
		"state", result.PolicyState,
		"changed", result.StateChanged,
		"llm", result.LLMCalled,
		"phase", result.PhaseReached,
		"duration_ms", result.Duration.Milliseconds())
}

func (t *Ticker) logSkipped(ctx context.Context, cfg Config, reason string) {
	t.emitEvent(Event{Action: "skipped", AgentID: cfg.AgentID, Reason: reason})
	slog.Debug("heartbeat: skipped", "agent", cfg.AgentID, "reason", reason)
}

func (t *Ticker) advanceNextRun(ctx context.Context, cfg Config) {
	next := time.Now().Add(time.Duration(cfg.IntervalSec)*time.Second + StaggerOffset(cfg.AgentID, cfg.IntervalSec))
	t.store.UpdateState(ctx, cfg.ID, cfg.CurrentState, cfg.ConsecFailures, cfg.ConsecPasses, cfg.RunCount, next, "skipped", "")
}

// RunHeartbeat executes the full 5-phase heartbeat for a config.
// This is the function injected into the ticker as runFn.
func RunHeartbeat(ctx context.Context, cfg Config, taskStore *tasks.Store) RunResult {
	result := RunResult{PolicyState: cfg.CurrentState}

	// Deterministic probes
	result.PhaseReached = 1
	result.ProbeResults = RunProbes(ctx, cfg.Probes)

	// Policy engine (state machine)
	result.PhaseReached = 2
	newState, changed := EvaluatePolicy(&cfg, result.ProbeResults)
	result.PolicyState = newState
	result.StateChanged = changed

	// Check pending tasks
	result.PhaseReached = 3
	pendingTasks := 0
	if taskStore != nil {
		tasks, err := taskStore.ListForAgent(ctx, cfg.AgentID, "", 10)
		if err == nil {
			pendingTasks = len(tasks)
		}
	}

	// LLM escalation (only if needed)
	result.PhaseReached = 4
	if ShouldEscalateToLLM(&cfg, result.ProbeResults, changed, pendingTasks) {
		result.LLMCalled = true
		// LLM call would happen here via the agent loop
		// For now, generate a summary from probe results
		result.Summary = buildProbeSummary(result.ProbeResults, newState, pendingTasks)
	}

	// Report
	result.PhaseReached = 5
	if result.LLMCalled && result.Summary != "" {
		result.Status = "completed"
	} else {
		result.Status = "suppressed" // no delivery needed
	}

	return result
}

func buildProbeSummary(probes []ProbeResult, state string, pendingTasks int) string {
	parts := []string{}
	parts = append(parts, "State: "+state)
	for _, p := range probes {
		status := "✅"
		if !p.OK {
			status = "❌"
		}
		parts = append(parts, status+" "+p.Name+": "+p.Value)
		if p.Error != "" {
			parts = append(parts, "  Error: "+p.Error)
		}
	}
	if pendingTasks > 0 {
		parts = append(parts, "📋 Pending tasks: "+string(rune('0'+pendingTasks)))
	}
	return strings.Join(parts, "\n")
}

// ProcessResponse implements smart suppression.
// If response contains HEARTBEAT_OK, suppress delivery.
func ProcessResponse(response string) (deliver bool, cleaned string) {
	if strings.Contains(response, "HEARTBEAT_OK") {
		return false, ""
	}
	return true, response
}
