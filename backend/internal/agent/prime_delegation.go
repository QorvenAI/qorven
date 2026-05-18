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

// PrimeDelegation manages the Prime Soul → specialist agent delegation flow.
// User talks to Prime → Prime delegates to specialist → specialist reports back → Prime responds.
type PrimeDelegation struct {
	mu          sync.RWMutex
	primeID     string
	agentStore  AgentLookup
	pending     map[string]*DelegatedTask
	onComplete  func(task *DelegatedTask)
	// runAgent executes a specialist agent and returns the response
	runAgent    func(ctx context.Context, agentID, message string) (string, error)
}

// AgentLookup finds agents by ID or key.
type AgentLookup interface {
	Get(ctx context.Context, id string) (*Agent, error)
	GetByKey(ctx context.Context, key string) (*Agent, error)
	List(ctx context.Context, tenantID string) ([]*Agent, error)
}

// DelegatedTask tracks a task delegated from Prime to a specialist.
type DelegatedTask struct {
	ID            string    `json:"id"`
	PrimeID       string    `json:"prime_id"`
	SpecialistID  string    `json:"specialist_id"`
	SpecialistKey string    `json:"specialist_key"`
	UserQuery     string    `json:"user_query"`
	Instructions  string    `json:"instructions"`
	Status        TaskStatus `json:"status"`
	Result        string    `json:"result,omitempty"`
	Error         string    `json:"error,omitempty"`
	OriginChannel string    `json:"origin_channel"`
	OriginChatID  string    `json:"origin_chat_id"`
	CreatedAt     time.Time `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}


const (
)

// NewPrimeDelegation creates a delegation manager for the Prime Soul.
func NewPrimeDelegation(primeID string, store AgentLookup) *PrimeDelegation {
	return &PrimeDelegation{
		primeID:    primeID,
		agentStore: store,
		pending:    make(map[string]*DelegatedTask),
	}
}

// SetOnComplete sets the callback for when a delegated task finishes.
func (pd *PrimeDelegation) SetOnComplete(fn func(task *DelegatedTask)) {
	pd.onComplete = fn
}

// SetRunAgent sets the function that executes a specialist agent.
func (pd *PrimeDelegation) SetRunAgent(fn func(ctx context.Context, agentID, message string) (string, error)) {
	pd.runAgent = fn
}

// Delegate sends a task from Prime to a specialist agent.
// If runAgent is set, executes the specialist in a goroutine and calls Complete/Fail on finish.
func (pd *PrimeDelegation) Delegate(ctx context.Context, specialistKey, instructions, userQuery, channel, chatID string) (*DelegatedTask, error) {
	specialist, err := pd.agentStore.GetByKey(ctx, specialistKey)
	if err != nil || specialist == nil {
		return nil, fmt.Errorf("specialist %q not found", specialistKey)
	}

	task := &DelegatedTask{
		ID:            fmt.Sprintf("del-%d", time.Now().UnixNano()),
		PrimeID:       pd.primeID,
		SpecialistID:  specialist.ID,
		SpecialistKey: specialistKey,
		UserQuery:     userQuery,
		Instructions:  instructions,
		Status:        TaskPending,
		OriginChannel: channel,
		OriginChatID:  chatID,
		CreatedAt:     time.Now(),
	}

	pd.mu.Lock()
	pd.pending[task.ID] = task
	pd.mu.Unlock()

	slog.Info("prime delegated task",
		"task", task.ID, "specialist", specialistKey,
		"query", truncateStr(userQuery, 80))

	// Execute specialist in background if runAgent is set
	if pd.runAgent != nil {
		go func() {
			msg := instructions
			if msg == "" {
				msg = userQuery
			}
			result, err := pd.runAgent(context.Background(), specialist.ID, msg)
			if err != nil {
				pd.Fail(task.ID, err.Error())
			} else {
				pd.Complete(task.ID, result)
			}
		}()
	}

	return task, nil
}

// Complete marks a delegated task as finished.
func (pd *PrimeDelegation) Complete(taskID, result string) {
	pd.mu.Lock()
	task, ok := pd.pending[taskID]
	if !ok {
		pd.mu.Unlock()
		return
	}
	now := time.Now()
	task.Status = TaskCompleted
	task.Result = result
	task.CompletedAt = &now
	pd.mu.Unlock()

	slog.Info("delegation completed", "task", taskID, "specialist", task.SpecialistKey)
	if pd.onComplete != nil {
		pd.onComplete(task)
	}
}

// Fail marks a delegated task as failed.
func (pd *PrimeDelegation) Fail(taskID, reason string) {
	pd.mu.Lock()
	task, ok := pd.pending[taskID]
	if !ok {
		pd.mu.Unlock()
		return
	}
	now := time.Now()
	task.Status = TaskFailed
	task.Error = reason
	task.CompletedAt = &now
	pd.mu.Unlock()

	slog.Warn("delegation failed", "task", taskID, "specialist", task.SpecialistKey, "reason", reason)
	if pd.onComplete != nil {
		pd.onComplete(task)
	}
}

// Pending returns all pending/running delegated tasks.
func (pd *PrimeDelegation) Pending() []*DelegatedTask {
	pd.mu.RLock()
	defer pd.mu.RUnlock()
	var tasks []*DelegatedTask
	for _, t := range pd.pending {
		if t.Status == TaskPending || t.Status == TaskInProgress {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

