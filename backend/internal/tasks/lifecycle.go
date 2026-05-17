// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TaskState represents the lifecycle state of a task.
type TaskState string

const (
	StateBacklog    TaskState = "backlog"
	StateTodo       TaskState = "todo"
	StateInProgress TaskState = "in_progress"
	StateReview     TaskState = "review"
	StateDone       TaskState = "done"
	StateCancelled  TaskState = "cancelled"
)

// ValidTransitions defines allowed state transitions.
var ValidTransitions = map[TaskState][]TaskState{
	StateBacklog:    {StateTodo, StateCancelled},
	StateTodo:       {StateInProgress, StateBacklog, StateCancelled},
	StateInProgress: {StateReview, StateTodo, StateCancelled},
	StateReview:     {StateDone, StateInProgress, StateCancelled},
	StateDone:       {}, // terminal
	StateCancelled:  {StateBacklog}, // can reopen
}

// CanTransition checks if a state transition is valid.
func CanTransition(from, to TaskState) bool {
	for _, t := range ValidTransitions[from] {
		if t == to { return true }
	}
	return false
}

// Lifecycle manages task state transitions.
type Lifecycle struct{ pool *pgxpool.Pool }

func NewLifecycle(pool *pgxpool.Pool) *Lifecycle { return &Lifecycle{pool: pool} }

// Transition moves a task to a new state with validation.
func (lc *Lifecycle) Transition(ctx context.Context, taskID string, newState TaskState, actorID string) error {
	var currentState TaskState
	err := lc.pool.QueryRow(ctx, "SELECT state FROM tasks WHERE id = $1", taskID).Scan(&currentState)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}
	if !CanTransition(currentState, newState) {
		return fmt.Errorf("invalid transition: %s → %s", currentState, newState)
	}

	now := time.Now()
	switch newState {
	case StateInProgress:
		_, err = lc.pool.Exec(ctx,
			"UPDATE tasks SET state=$1, started_at=$2, locked_by=$3, locked_at=$2, updated_at=$2 WHERE id=$4",
			newState, now, actorID, taskID)
	case StateDone:
		_, err = lc.pool.Exec(ctx,
			"UPDATE tasks SET state=$1, completed_at=$2, locked_by=NULL, locked_at=NULL, updated_at=$2 WHERE id=$3",
			newState, now, taskID)
	case StateCancelled:
		_, err = lc.pool.Exec(ctx,
			"UPDATE tasks SET state=$1, locked_by=NULL, locked_at=NULL, updated_at=$2 WHERE id=$3",
			newState, now, taskID)
	default:
		_, err = lc.pool.Exec(ctx,
			"UPDATE tasks SET state=$1, updated_at=$2 WHERE id=$3",
			newState, now, taskID)
	}
	return err
}

// Assign assigns a task to an agent.
func (lc *Lifecycle) Assign(ctx context.Context, taskID, agentID string) error {
	_, err := lc.pool.Exec(ctx,
		"UPDATE tasks SET assigned_agent_id=$1, updated_at=NOW() WHERE id=$2",
		agentID, taskID)
	return err
}

// CreateSubtask creates a child task linked to a parent.
func (lc *Lifecycle) CreateSubtask(ctx context.Context, parentID, tenantID, title, description, agentID string) (string, error) {
	var id string
	err := lc.pool.QueryRow(ctx,
		`INSERT INTO tasks (tenant_id, title, description, assigned_agent_id, parent_task_id, state)
		 VALUES ($1, $2, $3, $4, $5, 'todo') RETURNING id`,
		tenantID, title, description, agentID, parentID).Scan(&id)
	return id, err
}

// ListByState returns tasks in a given state for a tenant.
func (lc *Lifecycle) ListByState(ctx context.Context, tenantID string, state TaskState, limit int) ([]map[string]any, error) {
	if limit <= 0 { limit = 50 }
	rows, err := lc.pool.Query(ctx,
		`SELECT id, title, description, state, assigned_agent_id, parent_task_id, created_at, started_at, completed_at
		 FROM tasks WHERE tenant_id=$1 AND state=$2 ORDER BY created_at DESC LIMIT $3`,
		tenantID, state, limit)
	if err != nil { return nil, err }
	defer rows.Close()

	tasks := []map[string]any{}
	for rows.Next() {
		var id, title, desc, st string
		var assignedAgent, parentTask *string
		var createdAt time.Time
		var startedAt, completedAt *time.Time
		rows.Scan(&id, &title, &desc, &st, &assignedAgent, &parentTask, &createdAt, &startedAt, &completedAt)
		tasks = append(tasks, map[string]any{
			"id": id, "title": title, "description": desc, "state": st,
			"assigned_agent_id": assignedAgent, "parent_task_id": parentTask,
			"created_at": createdAt, "started_at": startedAt, "completed_at": completedAt,
		})
	}
	return tasks, nil
}

// Lock acquires a lock on a task for an agent (prevents concurrent work).
func (lc *Lifecycle) Lock(ctx context.Context, taskID, agentID string) error {
	result, err := lc.pool.Exec(ctx,
		"UPDATE tasks SET locked_by=$1, locked_at=NOW() WHERE id=$2 AND (locked_by IS NULL OR locked_by=$1)",
		agentID, taskID)
	if err != nil { return err }
	if result.RowsAffected() == 0 {
		return fmt.Errorf("task %s is locked by another agent", taskID)
	}
	return nil
}

// Unlock releases a lock on a task.
func (lc *Lifecycle) Unlock(ctx context.Context, taskID string) error {
	_, err := lc.pool.Exec(ctx, "UPDATE tasks SET locked_by=NULL, locked_at=NULL WHERE id=$1", taskID)
	return err
}
