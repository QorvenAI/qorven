// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Standard task lifecycle states.
const (
	StateDraft           = "draft"
	StateSubmitted       = "submitted"
	StateRouted          = "routed"
	StateInProgress      = "in_progress"
	StateWaitingApproval = "waiting_approval"
	StateBlocked         = "blocked"
	StateCompleted       = "completed"
	StateRejected        = "rejected"
	StateArchived        = "archived"
)

// validTransitions defines legal state transitions.
var validTransitions = map[string][]string{
	StateDraft:           {StateSubmitted, StateArchived},
	StateSubmitted:       {StateRouted, StateRejected, StateArchived},
	StateRouted:          {StateInProgress, StateBlocked, StateRejected},
	StateInProgress:      {StateCompleted, StateBlocked, StateWaitingApproval},
	StateWaitingApproval: {StateInProgress, StateRejected, StateBlocked},
	StateBlocked:         {StateInProgress, StateRejected, StateArchived},
	StateCompleted:       {StateArchived},
	StateRejected:        {StateDraft, StateArchived},
	StateArchived:        {},
}

type TaskTransition struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	FromState string    `json:"from_state"`
	ToState   string    `json:"to_state"`
	ChangedBy string    `json:"changed_by"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskStateMachine struct {
	db *pgxpool.Pool
}

func NewTaskStateMachine(db *pgxpool.Pool) *TaskStateMachine {
	return &TaskStateMachine{db: db}
}

func (m *TaskStateMachine) Transition(ctx context.Context, tenantID, taskID, fromState, toState, changedBy, reason string) error {
	if !isValidTransition(fromState, toState) {
		return fmt.Errorf("invalid transition: %s → %s", fromState, toState)
	}

	tx, err := m.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Record the transition
	_, err = tx.Exec(ctx, `
		INSERT INTO task_state_transitions (tenant_id, task_id, from_state, to_state, changed_by, reason)
		VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid,$6)
	`, tenantID, taskID, fromState, toState, changedBy, reason)
	if err != nil {
		return err
	}

	// Update the task (may fail if tasks table not owned — graceful)
	_, _ = tx.Exec(ctx, `
		UPDATE tasks SET workflow_state=$1, state_changed_at=now(), state_changed_by=NULLIF($2,'')::uuid
		WHERE id=$3
	`, toState, changedBy, taskID)

	return tx.Commit(ctx)
}

func (m *TaskStateMachine) History(ctx context.Context, tenantID, taskID string) ([]TaskTransition, error) {
	rows, err := m.db.Query(ctx, `
		SELECT id, task_id, from_state, to_state, COALESCE(changed_by::text,''), COALESCE(reason,''), created_at
		FROM task_state_transitions WHERE tenant_id = $1 AND task_id = $2
		ORDER BY created_at ASC
	`, tenantID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaskTransition
	for rows.Next() {
		var t TaskTransition
		if err := rows.Scan(&t.ID, &t.TaskID, &t.FromState, &t.ToState, &t.ChangedBy, &t.Reason, &t.CreatedAt); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func isValidTransition(from, to string) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
