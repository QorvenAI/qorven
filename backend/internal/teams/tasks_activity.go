// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// AddTaskComment adds a comment or blocker note to a task.
func (s *Store) AddTaskComment(ctx context.Context, c TaskComment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO team_task_comments (task_id, agent_id, user_id, content, comment_type, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.TaskID, nullStr(c.AgentID), nullStr(c.UserID), c.Content,
		coalesce(c.CommentType, "note"), time.Now())
	return err
}

// ListTaskComments returns all comments for a task.
func (s *Store) ListTaskComments(ctx context.Context, taskID string) ([]TaskComment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, COALESCE(agent_id::text,''), COALESCE(user_id,''), content,
		 COALESCE(comment_type,'note'), created_at
		 FROM team_task_comments WHERE task_id = $1 ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []TaskComment{}
	for rows.Next() {
		var c TaskComment
		rows.Scan(&c.ID, &c.TaskID, &c.AgentID, &c.UserID, &c.Content, &c.CommentType, &c.CreatedAt)
		comments = append(comments, c)
	}
	return comments, nil
}

// ListRecentTaskComments returns the N most recent comments.
func (s *Store) ListRecentTaskComments(ctx context.Context, taskID string, limit int) ([]TaskComment, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, COALESCE(agent_id::text,''), COALESCE(user_id,''), content,
		 COALESCE(comment_type,'note'), created_at
		 FROM team_task_comments WHERE task_id = $1 ORDER BY created_at DESC LIMIT $2`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []TaskComment{}
	for rows.Next() {
		var c TaskComment
		rows.Scan(&c.ID, &c.TaskID, &c.AgentID, &c.UserID, &c.Content, &c.CommentType, &c.CreatedAt)
		comments = append(comments, c)
	}
	return comments, nil
}

// RecordTaskEvent logs an audit event for a task.
func (s *Store) RecordTaskEvent(ctx context.Context, e TaskEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO team_task_events (task_id, event_type, actor_type, actor_id, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.TaskID, e.EventType, e.ActorType, e.ActorID, e.Data, time.Now())
	return err
}

// ListTaskEvents returns all events for a task.
func (s *Store) ListTaskEvents(ctx context.Context, taskID string) ([]TaskEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, event_type, actor_type, actor_id, COALESCE(data,'{}'), created_at
		 FROM team_task_events WHERE task_id = $1 ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []TaskEvent{}
	for rows.Next() {
		var e TaskEvent
		rows.Scan(&e.ID, &e.TaskID, &e.EventType, &e.ActorType, &e.ActorID, &e.Data, &e.CreatedAt)
		events = append(events, e)
	}
	return events, nil
}

// ListTeamEvents returns recent events across all tasks in a team.
func (s *Store) ListTeamEvents(ctx context.Context, teamID string, limit, offset int) ([]TaskEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT e.id, e.task_id, e.event_type, e.actor_type, e.actor_id, COALESCE(e.data,'{}'), e.created_at
		 FROM team_task_events e JOIN team_tasks t ON e.task_id = t.id::text
		 WHERE t.team_id = $1 ORDER BY e.created_at DESC LIMIT $2 OFFSET $3`,
		teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []TaskEvent{}
	for rows.Next() {
		var e TaskEvent
		rows.Scan(&e.ID, &e.TaskID, &e.EventType, &e.ActorType, &e.ActorID, &e.Data, &e.CreatedAt)
		events = append(events, e)
	}
	return events, nil
}

// AttachFile attaches a file reference to a task.
func (s *Store) AttachFile(ctx context.Context, a TaskAttachment) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO team_task_attachments (task_id, team_id, chat_id, path, file_size, mime_type,
		 created_by_agent_id, created_by_sender_id, metadata, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		a.TaskID, a.TeamID, nullStr(a.ChatID), a.Path, a.FileSize, nullStr(a.MimeType),
		nullStr(a.CreatedByAgent), nullStr(a.CreatedBySender), a.Metadata, time.Now())
	return err
}

// ListTaskAttachments returns all attachments for a task.
func (s *Store) ListTaskAttachments(ctx context.Context, taskID string) ([]TaskAttachment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, task_id, team_id, COALESCE(chat_id,''), path, file_size, COALESCE(mime_type,''),
		 COALESCE(created_by_agent_id::text,''), COALESCE(created_by_sender_id,''), metadata, created_at
		 FROM team_task_attachments WHERE task_id = $1 ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := []TaskAttachment{}
	for rows.Next() {
		var a TaskAttachment
		rows.Scan(&a.ID, &a.TaskID, &a.TeamID, &a.ChatID, &a.Path, &a.FileSize, &a.MimeType,
			&a.CreatedByAgent, &a.CreatedBySender, &a.Metadata, &a.CreatedAt)
		attachments = append(attachments, a)
	}
	return attachments, nil
}

// DetachFile removes a file attachment from a task.
func (s *Store) DetachFile(ctx context.Context, taskID, path string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM team_task_attachments WHERE task_id = $1 AND path = $2`, taskID, path)
	return err
}

// Internal helpers for recording events inside transactions
func (s *Store) recordTaskEvent(ctx context.Context, tx pgx.Tx, taskID, eventType, actorType, actorID string, data any) {
	var dataJSON json.RawMessage
	if data != nil {
		dataJSON, _ = json.Marshal(data)
	}
	tx.Exec(ctx,
		`INSERT INTO team_task_events (task_id, event_type, actor_type, actor_id, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		taskID, eventType, actorType, actorID, dataJSON, time.Now())
}

func (s *Store) recordTaskEventSimple(ctx context.Context, taskID, eventType, actorType, actorID string) {
	s.pool.Exec(ctx,
		`INSERT INTO team_task_events (task_id, event_type, actor_type, actor_id, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		taskID, eventType, actorType, actorID, time.Now())
}

func coalesce(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
