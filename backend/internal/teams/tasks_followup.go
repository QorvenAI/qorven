// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"time"
)

// SetTaskFollowup schedules a reminder for a task.
func (s *Store) SetTaskFollowup(ctx context.Context, taskID, teamID string, followupAt time.Time, maxReminders int, message, channel, chatID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET followup_at = $1, followup_max = $2, followup_count = 0,
		 followup_message = $3, followup_channel = $4, followup_chat_id = $5, updated_at = $6
		 WHERE id = $7 AND team_id = $8`,
		followupAt, maxReminders, message, channel, chatID, time.Now(), taskID, teamID)
	return err
}

// ClearTaskFollowup removes the followup reminder from a task.
func (s *Store) ClearTaskFollowup(ctx context.Context, taskID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET followup_at = NULL, followup_count = 0, followup_max = 0,
		 followup_message = '', followup_channel = '', followup_chat_id = '', updated_at = $1
		 WHERE id = $2`, time.Now(), taskID)
	return err
}

// ListDueFollowups returns all tasks with followups that are due now.
func (s *Store) ListDueFollowups(ctx context.Context) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, subject, status, task_number, identifier,
		 COALESCE(owner_agent_id::text,''), followup_at, followup_count, followup_max,
		 COALESCE(followup_message,''), COALESCE(followup_channel,''), COALESCE(followup_chat_id,'')
		 FROM team_tasks
		 WHERE followup_at IS NOT NULL AND followup_at <= NOW()
		   AND status IN ('pending','assigned','in_progress','review','blocked')
		   AND (followup_max = 0 OR followup_count < followup_max)
		 ORDER BY followup_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Status, &t.TaskNumber, &t.Identifier,
			&t.OwnerAgentID, &t.FollowupAt, &t.FollowupCount, &t.FollowupMax,
			&t.FollowupMsg, &t.FollowupChannel, &t.FollowupChatID)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// IncrementFollowupCount bumps the count and optionally sets next followup time.
func (s *Store) IncrementFollowupCount(ctx context.Context, taskID string, nextAt *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET followup_count = followup_count + 1, followup_at = $1, updated_at = $2
		 WHERE id = $3`, nextAt, time.Now(), taskID)
	return err
}

// ClearFollowupByScope clears followups for all tasks in a channel+chat scope.
func (s *Store) ClearFollowupByScope(ctx context.Context, channel, chatID string) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET followup_at = NULL, followup_count = 0, updated_at = $1
		 WHERE followup_channel = $2 AND followup_chat_id = $3 AND followup_at IS NOT NULL`,
		time.Now(), channel, chatID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// SetFollowupForActiveTasks sets followup reminders for all active tasks in a scope.
func (s *Store) SetFollowupForActiveTasks(ctx context.Context, teamID, channel, chatID string, followupAt time.Time, maxReminders int, message string) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET followup_at = $1, followup_max = $2, followup_count = 0,
		 followup_message = $3, followup_channel = $4, followup_chat_id = $5, updated_at = $6
		 WHERE team_id = $7 AND status IN ('pending','assigned','in_progress')
		   AND COALESCE(chat_id,'') = $5`,
		followupAt, maxReminders, message, channel, chatID, time.Now(), teamID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
