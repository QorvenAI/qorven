// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Status constants for task lifecycle.
const (
	StatusBacklog    = "backlog"
	StatusAssigned   = "assigned"
	StatusInProgress = "in_progress"
	StatusReview     = "review"
	StatusDone       = "done"
	StatusBlocked    = "blocked"
	StatusCancelled  = "cancelled"
	StatusPaused     = "paused"
)

// Task represents a unit of work assigned to an agent.
type Task struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	ParentID    *string    `json:"parent_id,omitempty"`
	TicketID    *string    `json:"ticket_id,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Context     string     `json:"context,omitempty"`     // full goal ancestry
	AssignedTo  *string    `json:"assigned_to,omitempty"`
	AssignedBy  *string    `json:"assigned_by,omitempty"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Result      string     `json:"result,omitempty"`
	TokensUsed  int64      `json:"tokens_used"`
	CostCents   int64      `json:"cost_cents"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	// Autonomous runtime fields (migration 071)
	Scratchpad           string     `json:"scratchpad,omitempty"`
	LastHeartbeatAt      *time.Time `json:"last_heartbeat_at,omitempty"`
	BudgetCents          int        `json:"budget_cents,omitempty"`
	IterationCount       int        `json:"iteration_count,omitempty"`
	SynthesisTriggeredAt *time.Time `json:"synthesis_triggered_at,omitempty"`
	Permissions          string     `json:"permissions,omitempty"` // JSONB stored as string
	AllowedDomains       []string   `json:"allowed_domains,omitempty"`

	// Origin linkage (migration 073)
	DiscussionID    string `json:"discussion_id,omitempty"`
	OriginSessionID string `json:"origin_session_id,omitempty"`
}

// Store handles task CRUD with assignment and status transitions.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool returns the underlying pgxpool so callers can run atomic one-off queries.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Create creates a new task. If assignedTo is set, status becomes "assigned".
func (s *Store) Create(ctx context.Context, tenantID string, t Task) (string, error) {
	if t.Status == "" {
		t.Status = StatusBacklog
	}
	if t.AssignedTo != nil && t.Status == StatusBacklog {
		t.Status = StatusAssigned
	}
	if t.Priority < 1 || t.Priority > 5 {
		t.Priority = 3
	}

	// Build goal context from parent chain
	if t.ParentID != nil && t.Context == "" {
		t.Context = s.buildContext(ctx, *t.ParentID)
	}

	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tasks (tenant_id, parent_id, title, description, context, assigned_to, assigned_by, status, priority, due_at, discussion_id, origin_session_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, '')) RETURNING id`,
		tenantID, t.ParentID, t.Title, t.Description, t.Context,
		t.AssignedTo, t.AssignedBy, t.Status, t.Priority, t.DueAt,
		t.DiscussionID, t.OriginSessionID,
	).Scan(&id)
	return id, err
}

// Get returns a task by ID.
func (s *Store) Get(ctx context.Context, id string) (*Task, error) {
	t := &Task{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, parent_id, ticket_id, title, COALESCE(description,''), COALESCE(context,''),
		        assigned_to, assigned_by,
		        status, priority, COALESCE(result,''), tokens_used, cost_cents,
		        due_at, started_at, completed_at, created_at, updated_at,
		        COALESCE(scratchpad,''), last_heartbeat_at, budget_cents, iteration_count, synthesis_triggered_at
		 FROM tasks WHERE id = $1`, id,
	).Scan(&t.ID, &t.TenantID, &t.ParentID, &t.TicketID, &t.Title, &t.Description, &t.Context,
		&t.AssignedTo, &t.AssignedBy, &t.Status, &t.Priority, &t.Result,
		&t.TokensUsed, &t.CostCents, &t.DueAt, &t.StartedAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt,
		&t.Scratchpad, &t.LastHeartbeatAt, &t.BudgetCents, &t.IterationCount, &t.SynthesisTriggeredAt)
	if err != nil {
		return nil, fmt.Errorf("task not found: %s", id)
	}
	return t, nil
}

// ListForAgent returns tasks assigned to an agent, optionally filtered by status.
func (s *Store) ListForAgent(ctx context.Context, agentID string, status string, limit int) ([]Task, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows interface {
		Close()
		Next() bool
		Scan(...any) error
	}
	var err error

	if status != "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id, tenant_id, parent_id, title, description, status, priority, assigned_by, created_at, updated_at
			 FROM tasks WHERE assigned_to = $1 AND status = $2 ORDER BY priority, created_at LIMIT $3`,
			agentID, status, limit)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, tenant_id, parent_id, title, description, status, priority, assigned_by, created_at, updated_at
			 FROM tasks WHERE assigned_to = $1 AND status NOT IN ('done','cancelled')
			 ORDER BY priority, created_at LIMIT $2`,
			agentID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TenantID, &t.ParentID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedBy, &t.CreatedAt, &t.UpdatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListAll returns all tasks for a tenant, optionally filtered by status.
func (s *Store) ListAll(ctx context.Context, tenantID, status string, limit int) ([]Task, error) {
	if limit <= 0 { limit = 100 }
	query := `SELECT id, tenant_id, parent_id, ticket_id, title, description, status, priority, assigned_to, assigned_by, result, created_at, updated_at
		 FROM tasks WHERE tenant_id = $1`
	args := []any{tenantID}
	if status != "" && status != "all" {
		query += ` AND status = $2 ORDER BY created_at DESC LIMIT $3`
		args = append(args, status, limit)
	} else {
		query += ` ORDER BY created_at DESC LIMIT $2`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TenantID, &t.ParentID, &t.TicketID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedTo, &t.AssignedBy, &t.Result, &t.CreatedAt, &t.UpdatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// Search returns tasks whose title or description match the query string.
func (s *Store) Search(ctx context.Context, tenantID, q string, limit int) ([]Task, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, parent_id, ticket_id, title, description, status, priority,
		        assigned_to, assigned_by, result, created_at, updated_at
		 FROM tasks
		 WHERE tenant_id = $1
		   AND (title ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		 ORDER BY created_at DESC LIMIT $3`,
		tenantID, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TenantID, &t.ParentID, &t.TicketID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedTo, &t.AssignedBy, &t.Result, &t.CreatedAt, &t.UpdatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListSubtasks returns child tasks of a parent.
func (s *Store) ListSubtasks(ctx context.Context, parentID string) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, status, priority, assigned_to, created_at
		 FROM tasks WHERE parent_id = $1 ORDER BY priority, created_at`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.Title, &t.Status, &t.Priority, &t.AssignedTo, &t.CreatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// Transition moves a task to a new status with validation.
func (s *Store) Transition(ctx context.Context, id, newStatus string) error {
	// Validate transition
	t, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !isValidTransition(t.Status, newStatus) {
		return fmt.Errorf("invalid transition: %s → %s", t.Status, newStatus)
	}

	updates := map[string]any{"status": newStatus, "updated_at": time.Now()}
	switch newStatus {
	case StatusInProgress:
		now := time.Now()
		updates["started_at"] = now
	case StatusDone:
		now := time.Now()
		updates["completed_at"] = now
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE tasks SET status = $2, started_at = COALESCE($3, started_at),
		 completed_at = COALESCE($4, completed_at), updated_at = NOW()
		 WHERE id = $1`,
		id, newStatus, updates["started_at"], updates["completed_at"])
	return err
}

// Complete marks a task as done with a result.
func (s *Store) Complete(ctx context.Context, id, result string, tokensUsed int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tasks SET status = 'done', result = $2, tokens_used = tokens_used + $3,
		 completed_at = NOW(), updated_at = NOW() WHERE id = $1`,
		id, result, tokensUsed)
	return err
}

// Assign assigns a task to an agent.
func (s *Store) Assign(ctx context.Context, taskID, agentID, assignedBy string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tasks SET assigned_to = $2, assigned_by = $3, status = 'assigned', updated_at = NOW()
		 WHERE id = $1`, taskID, agentID, assignedBy)
	return err
}

// buildContext walks up the parent chain to build goal ancestry.
func (s *Store) buildContext(ctx context.Context, parentID string) string {
	chain := []string{}
	current := parentID
	for i := 0; i < 10; i++ { // max depth 10
		var title string
		var nextParent *string
		err := s.pool.QueryRow(ctx,
			`SELECT title, parent_id FROM tasks WHERE id = $1`, current,
		).Scan(&title, &nextParent)
		if err != nil {
			break
		}
		chain = append([]string{title}, chain...)
		if nextParent == nil {
			break
		}
		current = *nextParent
	}
	result := ""
	for i, t := range chain {
		if i > 0 {
			result += " → "
		}
		result += t
	}
	return result
}

// Valid status transitions.
func isValidTransition(from, to string) bool {
	valid := map[string][]string{
		StatusBacklog:    {StatusAssigned, StatusCancelled},
		StatusAssigned:   {StatusInProgress, StatusBacklog, StatusCancelled},
		StatusInProgress: {StatusReview, StatusDone, StatusBlocked, StatusPaused, StatusCancelled},
		StatusReview:     {StatusDone, StatusInProgress, StatusCancelled},
		StatusBlocked:    {StatusInProgress, StatusCancelled},
		StatusPaused:     {StatusInProgress, StatusCancelled},
		StatusDone:       {}, // terminal
		StatusCancelled:  {}, // terminal
	}
	for _, v := range valid[from] {
		if v == to {
			return true
		}
	}
	return false
}

// RecoverStale resets tasks stuck in "in_progress" for more than 30 minutes back to "assigned".
func (s *Store) RecoverStale(ctx context.Context) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tasks SET status = 'assigned', updated_at = NOW()
		 WHERE status = 'in_progress'
		   AND (last_heartbeat_at IS NULL OR last_heartbeat_at < NOW() - INTERVAL '30 minutes')
		   AND assigned_to IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// UpdateScratchpad persists the agent's scratchpad and updates the heartbeat timestamp.
func (s *Store) UpdateScratchpad(ctx context.Context, taskID, scratchpad string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE tasks
		 SET scratchpad = $2, last_heartbeat_at = NOW(), updated_at = NOW()
		 WHERE id = $1`,
		taskID, scratchpad)
	return err
}

// IncrementIteration atomically increments iteration_count and returns the new value.
func (s *Store) IncrementIteration(ctx context.Context, taskID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`UPDATE tasks SET iteration_count = iteration_count + 1, updated_at = NOW()
		 WHERE id = $1 RETURNING iteration_count`,
		taskID).Scan(&count)
	return count, err
}

// TaskEvent is a single entry in the task audit trail.
type TaskEvent struct {
	ID         string         `json:"id"`
	TaskID     string         `json:"task_id"`
	AgentID    string         `json:"agent_id"`
	EventType  string         `json:"event_type"`
	Payload    map[string]any `json:"payload"`
	TokensUsed int            `json:"tokens_used"`
	CostCents  int            `json:"cost_cents"`
	CreatedAt  time.Time      `json:"created_at"`
}

// LogEvent appends an entry to the task_events audit trail.
func (s *Store) LogEvent(ctx context.Context, e TaskEvent) error {
	payload, _ := json.Marshal(e.Payload)
	agentID := (*string)(nil)
	if e.AgentID != "" {
		agentID = &e.AgentID
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO task_events (task_id, agent_id, event_type, payload, tokens_used, cost_cents)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.TaskID, agentID, e.EventType, payload, e.TokensUsed, e.CostCents)
	return err
}

// ListEvents returns the audit trail for a task, oldest first.
func (s *Store) ListEvents(ctx context.Context, taskID string, limit int) ([]TaskEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, COALESCE(agent_id::text,''), event_type, payload, tokens_used, cost_cents, created_at
		 FROM task_events WHERE task_id = $1 ORDER BY created_at ASC LIMIT $2`,
		taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []TaskEvent
	for rows.Next() {
		var ev TaskEvent
		var payload []byte
		if err := rows.Scan(&ev.ID, &ev.TaskID, &ev.AgentID, &ev.EventType, &payload,
			&ev.TokensUsed, &ev.CostCents, &ev.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal(payload, &ev.Payload) //nolint:errcheck
		events = append(events, ev)
	}
	return events, rows.Err()
}

// SiblingStatus checks whether all subtasks of parentID are in terminal state.
// Returns true only when every sibling is 'done' or 'cancelled'.
func (s *Store) SiblingStatus(ctx context.Context, parentID string) (allTerminal bool, err error) {
	var active int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks
		 WHERE parent_id = $1 AND status NOT IN ('done','cancelled')`,
		parentID).Scan(&active)
	return active == 0, err
}

// GetSubtasks returns all immediate children of a parent task with their results.
func (s *Store) GetSubtasks(ctx context.Context, parentID string) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, status, COALESCE(result,''), assigned_to, iteration_count, created_at
		 FROM tasks WHERE parent_id = $1 ORDER BY created_at`,
		parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &t.Result, &t.AssignedTo, &t.IterationCount, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
