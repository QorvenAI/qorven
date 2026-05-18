// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"log/slog"
	"sync"
)

// ModelSwitchQueue prevents model changes during active runs.
type ModelSwitchQueue struct {
	mu       sync.Mutex
	busy     map[string]bool   // agentID → is running
	pending  map[string]string // agentID → pending model
}

func NewModelSwitchQueue() *ModelSwitchQueue {
	return &ModelSwitchQueue{busy: make(map[string]bool), pending: make(map[string]string)}
}

// MarkBusy marks an agent as running. Returns the effective model to use.
func (q *ModelSwitchQueue) MarkBusy(agentID, currentModel string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.busy[agentID] = true
	// Apply any pending model switch
	if m, ok := q.pending[agentID]; ok {
		delete(q.pending, agentID)
		slog.Info("model_switch.applied", "agent", agentID, "from", currentModel, "to", m)
		return m
	}
	return currentModel
}

// MarkDone marks an agent as idle.
func (q *ModelSwitchQueue) MarkDone(agentID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.busy, agentID)
}

// SwitchModel queues a model change. If agent is busy, defers until next run.
func (q *ModelSwitchQueue) SwitchModel(agentID, newModel string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.busy[agentID] {
		q.pending[agentID] = newModel
		slog.Info("model_switch.queued", "agent", agentID, "model", newModel)
		return false // queued, not applied yet
	}
	return true // safe to apply immediately
}
