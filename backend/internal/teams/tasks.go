// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CreateTask creates a new task with auto-generated task number and identifier.
func (s *Store) CreateTask(ctx context.Context, task *Task) error {
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.Status == "" {
		task.Status = TaskStatusPending
	}
	if task.TaskType == "" {
		task.TaskType = "general"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("tasks.create: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock team row to serialize task_number generation
	tx.Exec(ctx, `SELECT 1 FROM crews WHERE id = $1 FOR UPDATE`, task.TeamID)

	// Scope task_number per (team_id, chat_id)
	var taskNumber int
	tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(task_number), 0) + 1 FROM team_tasks WHERE team_id = $1 AND COALESCE(chat_id, '') = $2`,
		task.TeamID, task.ChatID).Scan(&taskNumber)
	task.TaskNumber = taskNumber

	// Generate identifier: T-001-ab3f
	hex := strings.ReplaceAll(task.ID, "-", "")
	task.Identifier = fmt.Sprintf("T-%03d-%s", taskNumber, hex[len(hex)-4:])

	metaJSON, _ := json.Marshal(task.Metadata)

	_, err = tx.Exec(ctx,
		`INSERT INTO team_tasks (id, team_id, subject, description, status, owner_agent_id, priority, result,
		 user_id, channel, task_type, task_number, identifier, created_by_agent_id, parent_id, chat_id,
		 metadata, locked_at, lock_expires_at, progress_percent, progress_step, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		task.ID, task.TeamID, task.Subject, task.Description, task.Status,
		nullStr(task.OwnerAgentID), task.Priority, nullStr(task.Result),
		nullStr(task.UserID), nullStr(task.Channel),
		task.TaskType, taskNumber, task.Identifier,
		nullStr(task.CreatedByAgent), nullStr(task.ParentID),
		nullStr(task.ChatID), metaJSON,
		task.LockedAt, task.LockExpiresAt,
		task.ProgressPercent, task.ProgressStep,
		now, now)
	if err != nil {
		return fmt.Errorf("tasks.create: insert: %w", err)
	}

	// Record creation event
	s.recordTaskEvent(ctx, tx, task.ID, "created", "agent", task.CreatedByAgent, nil)

	return tx.Commit(ctx)
}

// GetTask retrieves a single task by ID.
func (s *Store) GetTask(ctx context.Context, taskID string) (*Task, error) {
	var t Task
	meta := []byte{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, team_id, subject, COALESCE(description,''), status, COALESCE(owner_agent_id::text,''),
		 priority, COALESCE(result,''), COALESCE(user_id,''), COALESCE(channel,''),
		 task_type, task_number, identifier, COALESCE(created_by_agent_id::text,''),
		 COALESCE(parent_id::text,''), COALESCE(chat_id,''), COALESCE(metadata,'{}'),
		 locked_at, lock_expires_at, progress_percent, COALESCE(progress_step,''),
		 followup_at, followup_count, followup_max,
		 created_at, updated_at
		 FROM team_tasks WHERE id = $1`, taskID).Scan(
		&t.ID, &t.TeamID, &t.Subject, &t.Description, &t.Status, &t.OwnerAgentID,
		&t.Priority, &t.Result, &t.UserID, &t.Channel,
		&t.TaskType, &t.TaskNumber, &t.Identifier, &t.CreatedByAgent,
		&t.ParentID, &t.ChatID, &meta,
		&t.LockedAt, &t.LockExpiresAt, &t.ProgressPercent, &t.ProgressStep,
		&t.FollowupAt, &t.FollowupCount, &t.FollowupMax,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("tasks.get: %w", err)
	}
	json.Unmarshal(meta, &t.Metadata)
	return &t, nil
}

// ListTasks returns tasks for a team with filtering and pagination.
func (s *Store) ListTasks(ctx context.Context, teamID string, opts TaskListOpts) ([]Task, error) {
	query := `SELECT id, team_id, subject, COALESCE(description,''), status, COALESCE(owner_agent_id::text,''),
		priority, COALESCE(result,''), COALESCE(user_id,''), COALESCE(channel,''),
		task_type, task_number, identifier, COALESCE(created_by_agent_id::text,''),
		COALESCE(parent_id::text,''), COALESCE(chat_id,''), COALESCE(metadata,'{}'),
		progress_percent, COALESCE(progress_step,''), created_at, updated_at
		FROM team_tasks WHERE team_id = $1`

	args := []any{teamID}
	argN := 2

	if opts.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, opts.Status)
		argN++
	}
	if opts.UserID != "" {
		query += fmt.Sprintf(" AND user_id = $%d", argN)
		args = append(args, opts.UserID)
		argN++
	}
	if opts.Channel != "" {
		query += fmt.Sprintf(" AND channel = $%d", argN)
		args = append(args, opts.Channel)
		argN++
	}
	if opts.ChatID != "" {
		query += fmt.Sprintf(" AND chat_id = $%d", argN)
		args = append(args, opts.ChatID)
		argN++
	}

	orderBy := "created_at DESC"
	if opts.OrderBy == "priority" {
		orderBy = "priority DESC, created_at DESC"
	} else if opts.OrderBy == "updated" {
		orderBy = "updated_at DESC"
	}
	query += " ORDER BY " + orderBy

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, limit, opts.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks.list: %w", err)
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		meta := []byte{}
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Description, &t.Status, &t.OwnerAgentID,
			&t.Priority, &t.Result, &t.UserID, &t.Channel,
			&t.TaskType, &t.TaskNumber, &t.Identifier, &t.CreatedByAgent,
			&t.ParentID, &t.ChatID, &meta,
			&t.ProgressPercent, &t.ProgressStep, &t.CreatedAt, &t.UpdatedAt)
		json.Unmarshal(meta, &t.Metadata)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// TaskListOpts holds filtering options for ListTasks.
type TaskListOpts struct {
	Status  string
	UserID  string
	Channel string
	ChatID  string
	OrderBy string // "created", "priority", "updated"
	Limit   int
	Offset  int
}

// UpdateTask updates specific fields of a task.
func (s *Store) UpdateTask(ctx context.Context, taskID string, updates map[string]any) error {
	allowed := map[string]bool{
		"subject": true, "description": true, "status": true, "priority": true,
		"result": true, "owner_agent_id": true, "metadata": true, "task_type": true,
		"assignee_user_id": true, "parent_id": true,
	}

	sets := []string{}
	args := []any{}
	argN := 1

	for k, v := range updates {
		if !allowed[k] {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = $%d", k, argN))
		if k == "metadata" {
			b, _ := json.Marshal(v)
			args = append(args, b)
		} else {
			args = append(args, v)
		}
		argN++
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, fmt.Sprintf("updated_at = $%d", argN))
	args = append(args, time.Now())
	argN++

	args = append(args, taskID)
	query := fmt.Sprintf("UPDATE team_tasks SET %s WHERE id = $%d", strings.Join(sets, ", "), argN)

	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

// DeleteTask removes a task.
func (s *Store) DeleteTask(ctx context.Context, taskID, teamID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM team_tasks WHERE id = $1 AND team_id = $2`, taskID, teamID)
	return err
}

// DeleteTasks removes multiple tasks.
func (s *Store) DeleteTasks(ctx context.Context, taskIDs []string, teamID string) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM team_tasks WHERE id = ANY($1) AND team_id = $2`, taskIDs, teamID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// ListActiveTasksByChatID returns in-progress tasks for a specific chat.
func (s *Store) ListActiveTasksByChatID(ctx context.Context, chatID string) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, subject, status, task_number, identifier, COALESCE(owner_agent_id::text,''),
		 progress_percent, COALESCE(progress_step,''), created_at
		 FROM team_tasks WHERE chat_id = $1 AND status IN ('pending','assigned','in_progress','review','blocked')
		 ORDER BY task_number`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Status, &t.TaskNumber, &t.Identifier,
			&t.OwnerAgentID, &t.ProgressPercent, &t.ProgressStep, &t.CreatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
