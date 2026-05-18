// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"log/slog"
)

// EmbeddingProvider generates vector embeddings for semantic search.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

var taskEmbedder EmbeddingProvider

// SetTaskEmbeddingProvider sets the embedding provider for task search.
func SetTaskEmbeddingProvider(p EmbeddingProvider) {
	taskEmbedder = p
}

// GenerateTaskEmbedding creates and stores an embedding for a task's subject.
func (s *Store) GenerateTaskEmbedding(ctx context.Context, taskID, subject string) {
	if taskEmbedder == nil {
		return
	}
	embedding, err := taskEmbedder.Embed(ctx, subject)
	if err != nil {
		slog.Warn("tasks.embedding: failed", "task", taskID, "err", err)
		return
	}
	s.pool.Exec(ctx,
		`UPDATE team_tasks SET embedding = $1 WHERE id = $2`, embedding, taskID)
}

// SearchTasksByEmbedding performs semantic search on tasks using vector similarity.
func (s *Store) SearchTasksByEmbedding(ctx context.Context, teamID string, embedding []float32, limit int, userID string) ([]Task, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `SELECT id, team_id, subject, COALESCE(description,''), status, COALESCE(owner_agent_id::text,''),
		priority, task_number, identifier, COALESCE(chat_id,''), progress_percent, created_at
		FROM team_tasks
		WHERE team_id = $1 AND embedding IS NOT NULL`

	args := []any{teamID}
	if userID != "" {
		query += ` AND user_id = $3`
		args = append(args, embedding, userID)
		query += ` ORDER BY embedding <=> $2 LIMIT $4`
		args = append(args, limit)
	} else {
		args = append(args, embedding)
		query += ` ORDER BY embedding <=> $2 LIMIT $3`
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
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

// BackfillTaskEmbeddings generates embeddings for all tasks that don't have one.
func (s *Store) BackfillTaskEmbeddings(ctx context.Context) (int, error) {
	if taskEmbedder == nil {
		return 0, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, subject FROM team_tasks WHERE embedding IS NULL AND subject != '' LIMIT 100`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, subject string
		rows.Scan(&id, &subject)
		s.GenerateTaskEmbedding(ctx, id, subject)
		count++
	}
	return count, nil
}
