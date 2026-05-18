// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// RuntimeState is the observable lifecycle state of a persistent runtime.
type RuntimeState string

const (
	RuntimeIdle      RuntimeState = "idle"
	RuntimeWorking   RuntimeState = "working"
	RuntimeSuspended RuntimeState = "suspended"
)

// WakeupSignal is sent into a runtime to trigger work.
type WakeupSignal struct {
	Source   WakeupSource
	TaskID   string         // set for task-based wakeups
	Message  string         // set for channel_message wakeups
	Context  map[string]any // arbitrary metadata (subtask_results, etc.)
	Priority int
}

// RuntimeRunFn is the callback the runtime calls for each wakeup.
type RuntimeRunFn func(ctx context.Context, agentID string, sig WakeupSignal)

// AgentRuntime is a persistent goroutine that processes wakeup signals for one agent.
type AgentRuntime struct {
	agentID  string
	tenantID string

	wakeupCh   chan WakeupSignal // buffered 32 — callers never block
	overrideCh chan string       // buffered 4 — inject message mid-iteration
	resumeCh   chan struct{}     // buffered 1 — unblocks Suspend

	state      RuntimeState
	lastActive time.Time
	mu         sync.RWMutex

	runFn RuntimeRunFn
}

// NewAgentRuntime creates a runtime for agentID. runFn is called for each signal.
func NewAgentRuntime(agentID, tenantID string, runFn RuntimeRunFn) *AgentRuntime {
	return &AgentRuntime{
		agentID:    agentID,
		tenantID:   tenantID,
		wakeupCh:   make(chan WakeupSignal, 32),
		overrideCh: make(chan string, 4),
		resumeCh:   make(chan struct{}, 1),
		state:      RuntimeIdle,
		runFn:      runFn,
	}
}

// Run starts the runtime loop. Call in a dedicated goroutine. Exits when ctx is cancelled.
func (r *AgentRuntime) Run(ctx context.Context) {
	r.setState(RuntimeIdle)
	slog.Info("runtime.started", "agent", r.agentID)

	for {
		select {
		case <-ctx.Done():
			slog.Info("runtime.stopped", "agent", r.agentID)
			return
		case sig := <-r.wakeupCh:
			if r.State() == RuntimeSuspended {
				slog.Info("runtime.suspended.queued", "agent", r.agentID, "source", sig.Source)
				select {
				case <-ctx.Done():
					return
				case <-r.resumeCh:
				}
			}

			r.setState(RuntimeWorking)
			slog.Info("runtime.wakeup", "agent", r.agentID, "source", sig.Source, "task", sig.TaskID)
			r.runFn(ctx, r.agentID, sig)
			r.setState(RuntimeIdle)

			r.drainAndDispatch(ctx)
		}
	}
}

// drainAndDispatch processes signals already in the buffer without blocking.
func (r *AgentRuntime) drainAndDispatch(ctx context.Context) {
	for {
		select {
		case sig := <-r.wakeupCh:
			if ctx.Err() != nil {
				return
			}
			r.setState(RuntimeWorking)
			slog.Info("runtime.drain.wakeup", "agent", r.agentID, "source", sig.Source)
			r.runFn(ctx, r.agentID, sig)
			r.setState(RuntimeIdle)
		default:
			return
		}
	}
}

// Send enqueues a wakeup signal. Non-blocking: drops if buffer full.
func (r *AgentRuntime) Send(sig WakeupSignal) {
	select {
	case r.wakeupCh <- sig:
	default:
		slog.Warn("runtime.wakeup.dropped", "agent", r.agentID, "source", sig.Source)
	}
}

// Override injects a message into the current iteration. Non-blocking.
func (r *AgentRuntime) Override(message string) {
	select {
	case r.overrideCh <- message:
	default:
		slog.Warn("runtime.override.dropped", "agent", r.agentID)
	}
}

// NextOverride returns a pending override message, or "" if none.
func (r *AgentRuntime) NextOverride() string {
	select {
	case msg := <-r.overrideCh:
		return msg
	default:
		return ""
	}
}

// Suspend marks the runtime as suspended.
func (r *AgentRuntime) Suspend() {
	// Drain any stale resume token so a future Resume() is authoritative.
	select {
	case <-r.resumeCh:
	default:
	}
	r.setState(RuntimeSuspended)
	slog.Info("runtime.suspended", "agent", r.agentID)
}

// Resume unblocks a suspended runtime.
func (r *AgentRuntime) Resume() {
	if r.State() != RuntimeSuspended {
		return
	}
	r.setState(RuntimeIdle)
	select {
	case r.resumeCh <- struct{}{}:
	default:
	}
	slog.Info("runtime.resumed", "agent", r.agentID)
}

// State returns the current runtime state (safe for concurrent reads).
func (r *AgentRuntime) State() RuntimeState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *AgentRuntime) setState(s RuntimeState) {
	r.mu.Lock()
	r.state = s
	if s == RuntimeWorking {
		r.lastActive = time.Now()
	}
	r.mu.Unlock()
}

// AgentID returns the agent this runtime serves.
func (r *AgentRuntime) AgentID() string { return r.agentID }

// TenantID returns the tenant.
func (r *AgentRuntime) TenantID() string { return r.tenantID }

// LastActive returns when the runtime last entered the working state.
func (r *AgentRuntime) LastActive() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastActive
}
