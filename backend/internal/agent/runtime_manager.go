// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"sync"
)

// RuntimeManager holds all active per-agent runtimes.
type RuntimeManager struct {
	runtimes map[string]*AgentRuntime // agentID → runtime
	mu       sync.RWMutex
	runFn    RuntimeRunFn
	ctx      context.Context
}

// NewRuntimeManager creates a manager. runFn is called by every runtime on wakeup.
func NewRuntimeManager(ctx context.Context, runFn RuntimeRunFn) *RuntimeManager {
	return &RuntimeManager{
		runtimes: make(map[string]*AgentRuntime),
		runFn:    runFn,
		ctx:      ctx,
	}
}

// StartAll boots one runtime goroutine for each persistent agent.
func (m *RuntimeManager) StartAll(agents []AgentBootEntry) {
	for _, a := range agents {
		if a.RuntimeMode != "persistent" {
			continue
		}
		m.EnsureRuntime(a.AgentID, a.TenantID)
	}
	slog.Info("runtime_manager.started_all", "count", len(m.runtimes))
}

// EnsureRuntime starts a runtime for agentID if one isn't already running.
func (m *RuntimeManager) EnsureRuntime(agentID, tenantID string) *AgentRuntime {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.runtimes[agentID]; ok {
		return rt
	}
	rt := NewAgentRuntime(agentID, tenantID, m.runFn)
	m.runtimes[agentID] = rt
	go rt.Run(m.ctx)
	slog.Info("runtime_manager.runtime_started", "agent", agentID)
	return rt
}

// Get returns the runtime for agentID, or nil if not running.
func (m *RuntimeManager) Get(agentID string) *AgentRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtimes[agentID]
}

// WakeAgent sends a wakeup signal to agentID's runtime.
func (m *RuntimeManager) WakeAgent(agentID string, sig WakeupSignal) bool {
	rt := m.Get(agentID)
	if rt == nil {
		return false
	}
	rt.Send(sig)
	return true
}

// Suspend sets agentID's runtime to suspended state.
func (m *RuntimeManager) Suspend(agentID string) bool {
	rt := m.Get(agentID)
	if rt == nil {
		return false
	}
	rt.Suspend()
	return true
}

// Resume unblocks agentID's suspended runtime.
func (m *RuntimeManager) Resume(agentID string) bool {
	rt := m.Get(agentID)
	if rt == nil {
		return false
	}
	rt.Resume()
	return true
}

// Override injects a message into agentID's current iteration.
func (m *RuntimeManager) Override(agentID, message string) bool {
	rt := m.Get(agentID)
	if rt == nil {
		return false
	}
	rt.Override(message)
	return true
}

// States returns a snapshot of all runtime states keyed by agentID.
func (m *RuntimeManager) States() map[string]RuntimeState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]RuntimeState, len(m.runtimes))
	for id, rt := range m.runtimes {
		out[id] = rt.State()
	}
	return out
}

// AgentBootEntry is used by StartAll to know which agents need persistent runtimes.
type AgentBootEntry struct {
	AgentID     string
	TenantID    string
	RuntimeMode string // 'persistent' | 'oneshot'
}
