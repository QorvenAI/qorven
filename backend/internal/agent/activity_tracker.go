// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"sync"
	"time"
)

// ActivityTracker replaces wall-clock timeouts with smart activity tracking.
// Long-running tasks that are actively working will never be killed —
// only truly idle agents time out.
type ActivityTracker struct {
	mu           sync.Mutex
	lastActivity time.Time
	toolActive   bool      // true while a tool is executing
	toolStart    time.Time // when the current tool started
	idleTimeout  time.Duration
}

func NewActivityTracker(idleTimeout time.Duration) *ActivityTracker {
	return &ActivityTracker{
		lastActivity: time.Now(),
		idleTimeout:  idleTimeout,
	}
}

// Touch records activity — resets the idle timer.
func (at *ActivityTracker) Touch() {
	at.mu.Lock()
	at.lastActivity = time.Now()
	at.mu.Unlock()
}

// ToolStart marks the beginning of a tool execution.
// While a tool is active, the agent is never considered idle.
func (at *ActivityTracker) ToolStart() {
	at.mu.Lock()
	at.toolActive = true
	at.toolStart = time.Now()
	at.lastActivity = time.Now()
	at.mu.Unlock()
}

// ToolEnd marks the end of a tool execution.
func (at *ActivityTracker) ToolEnd() {
	at.mu.Lock()
	at.toolActive = false
	at.lastActivity = time.Now()
	at.mu.Unlock()
}

// IsIdle returns true if the agent has been idle (no tool activity, no LLM calls)
// for longer than the idle timeout. Active tool executions are never idle.
func (at *ActivityTracker) IsIdle() bool {
	at.mu.Lock()
	defer at.mu.Unlock()
	if at.toolActive {
		return false // actively working — never idle
	}
	return time.Since(at.lastActivity) > at.idleTimeout
}

// IdleDuration returns how long the agent has been idle.
// Returns 0 if a tool is currently active.
func (at *ActivityTracker) IdleDuration() time.Duration {
	at.mu.Lock()
	defer at.mu.Unlock()
	if at.toolActive {
		return 0
	}
	return time.Since(at.lastActivity)
}

// ActiveDuration returns how long the current tool has been running.
// Returns 0 if no tool is active.
func (at *ActivityTracker) ActiveDuration() time.Duration {
	at.mu.Lock()
	defer at.mu.Unlock()
	if !at.toolActive {
		return 0
	}
	return time.Since(at.toolStart)
}
