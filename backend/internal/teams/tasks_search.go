// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"fmt"
)

// SearchTasks performs full-text search on task subjects and descriptions.
func (s *Store) SearchTasks(ctx context.Context, teamID, query string, limit int, userID string) ([]Task, error) {
	if limit <= 0 {
		limit = 20
	}

	sql := `SELECT id, team_id, subject, COALESCE(description,''), status, COALESCE(owner_agent_id::text,''),
		priority, task_number, identifier, COALESCE(chat_id,''), progress_percent, created_at
		FROM team_tasks
		WHERE team_id = $1
		  AND (subject ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%' OR identifier ILIKE '%' || $2 || '%')`

	args := []any{teamID, query}
	argN := 3

	if userID != "" {
		sql += fmt.Sprintf(" AND user_id = $%d", argN)
		args = append(args, userID)
		argN++
	}

	sql += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("tasks.search: %w", err)
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Description, &t.Status, &t.OwnerAgentID,
			&t.Priority, &t.TaskNumber, &t.Identifier, &t.ChatID, &t.ProgressPercent, &t.CreatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListTaskScopes returns distinct channel+chatID combinations that have tasks.
func (s *Store) ListTaskScopes(ctx context.Context, teamID string) ([]ScopeEntry, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT COALESCE(channel,''), COALESCE(chat_id,'')
		 FROM team_tasks WHERE team_id = $1 AND channel IS NOT NULL AND channel != ''
		 ORDER BY channel, chat_id`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scopes := []ScopeEntry{}
	for rows.Next() {
		var s ScopeEntry
		rows.Scan(&s.Channel, &s.ChatID)
		scopes = append(scopes, s)
	}
	return scopes, nil
}

// GetTasksByIDs retrieves multiple tasks by their IDs.
func (s *Store) GetTasksByIDs(ctx context.Context, ids []string) ([]Task, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, subject, COALESCE(description,''), status, COALESCE(owner_agent_id::text,''),
		 priority, task_number, identifier, COALESCE(chat_id,''), progress_percent, created_at
		 FROM team_tasks WHERE id = ANY($1) ORDER BY task_number`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Description, &t.Status, &t.OwnerAgentID,
			&t.Priority, &t.TaskNumber, &t.Identifier, &t.ChatID, &t.ProgressPercent, &t.CreatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}
