// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// ToolExecResult is the result of a parallel tool execution.
type ToolExecResult struct {
	Name    string
	Content string
	Error   string
}

// AtomicPickupTask picks up a pending task with row-level locking.
func (l *Loop) AtomicPickupTask(ctx context.Context, agentID string) (string, error) {
	if l.agentStore == nil || l.agentStore.Pool() == nil {
		return "", nil
	}
	var taskID string
	err := l.agentStore.Pool().QueryRow(ctx,
		`UPDATE tasks SET status = 'in_progress', assigned_to = $1, updated_at = now()
		 WHERE id = (SELECT id FROM tasks WHERE status = 'pending' AND (assigned_to IS NULL OR assigned_to = $1)
		 ORDER BY priority DESC, created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED)
		 RETURNING id`, agentID).Scan(&taskID)
	if err != nil {
		return "", nil // no pending tasks
	}
	slog.Info("task.picked_up", "agent", agentID, "task", taskID)
	return taskID, nil
}

// ExecuteToolsParallel runs independent tool calls concurrently.
func ExecuteToolsParallel(ctx context.Context, calls []providers.ToolCall, execFn func(context.Context, providers.ToolCall) (string, error)) []ToolExecResult {
	results := make([]ToolExecResult, len(calls))
	var wg sync.WaitGroup
	for i, tc := range calls {
		wg.Add(1)
		go func(idx int, call providers.ToolCall) {
			defer wg.Done()
			content, err := execFn(ctx, call)
			results[idx] = ToolExecResult{Name: call.Name, Content: content}
			if err != nil { results[idx].Error = err.Error() }
		}(i, tc)
	}
	wg.Wait()
	return results
}

// InterruptManager tracks interrupted sessions.
type InterruptManager struct {
	mu      sync.Mutex
	pending map[string]bool
}

func NewInterruptManager() *InterruptManager {
	return &InterruptManager{pending: make(map[string]bool)}
}

func (im *InterruptManager) Interrupt(sessionID string) {
	im.mu.Lock(); defer im.mu.Unlock()
	im.pending[sessionID] = true
}

func (im *InterruptManager) Check(sessionID string) bool {
	im.mu.Lock(); defer im.mu.Unlock()
	if im.pending[sessionID] { delete(im.pending, sessionID); return true }
	return false
}

// ShouldNudgeSkills returns true if agent should be reminded about skills.
func ShouldNudgeSkills(itersSinceSkill int, hasSkills bool) bool {
	return hasSkills && itersSinceSkill > 5
}

// RiskyTools require approval before execution.
var RiskyTools = map[string]string{
	"shell_exec": "Shell commands require approval",
	"file_delete": "File deletion requires approval",
	"git_push":   "Git push requires approval",
	"deploy":     "Deployment requires approval",
	"send_email": "Sending emails requires approval",
}

func NeedsApproval(toolName string) (bool, string) {
	reason, ok := RiskyTools[toolName]
	return ok, reason
}

// EventBus for domain events.
type EventType string

const (
	EvtIssueCreated   EventType = "issue.created"
	EvtHeartbeatFired  EventType = "heartbeat.fired"
	EvtArtifactCreated EventType = "artifact.created"
	EvtBudgetExceeded  EventType = "budget.exceeded"
	EvtToolCalled      EventType = "tool.called"
)

type DomainEvent struct {
	Type      EventType      `json:"type"`
	AgentID   string         `json:"agent_id"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
}

type EventBus struct {
	mu   sync.RWMutex
	subs map[EventType][]func(DomainEvent)
}

func NewEventBus() *EventBus { return &EventBus{subs: make(map[EventType][]func(DomainEvent))} }

func (eb *EventBus) Subscribe(t EventType, fn func(DomainEvent)) {
	eb.mu.Lock(); defer eb.mu.Unlock()
	eb.subs[t] = append(eb.subs[t], fn)
}

func (eb *EventBus) Publish(evt DomainEvent) {
	eb.mu.RLock(); defer eb.mu.RUnlock()
	evt.Timestamp = time.Now()
	for _, fn := range eb.subs[evt.Type] {
		go func(f func(DomainEvent)) { defer func() { recover() }(); f(evt) }(fn)
	}
	slog.Debug("event.published", "type", evt.Type, "agent", evt.AgentID)
}
