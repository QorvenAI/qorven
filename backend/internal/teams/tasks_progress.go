// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"time"
)

// UpdateTaskProgress updates the progress percentage and current step.
func (s *Store) UpdateTaskProgress(ctx context.Context, taskID, teamID string, percent int, step string) error {
	if percent < 0 { percent = 0 }
	if percent > 100 { percent = 100 }

	status := TaskStatusInProgress
	if percent == 100 {
		status = TaskStatusCompleted
	}

	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET progress_percent = $1, progress_step = $2, status = $3, updated_at = $4
		 WHERE id = $5 AND team_id = $6`,
		percent, step, status, time.Now(), taskID, teamID)
	return err
}

// RenewTaskLock extends the lock expiry for an in-progress task.
func (s *Store) RenewTaskLock(ctx context.Context, taskID, teamID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET locked_at = $1, lock_expires_at = $2, updated_at = $1
		 WHERE id = $3 AND team_id = $4 AND status IN ('assigned','in_progress')`,
		time.Now(), time.Now().Add(30*time.Minute), taskID, teamID)
	return err
}

// RecoverStaleTasks finds tasks with expired locks and resets them to pending.
func (s *Store) RecoverStaleTasks(ctx context.Context) ([]RecoveredTask, error) {
	rows, err := s.pool.Query(ctx,
		`UPDATE team_tasks SET status = 'pending', owner_agent_id = NULL,
		 locked_at = NULL, lock_expires_at = NULL, updated_at = NOW()
		 WHERE status IN ('assigned','in_progress') AND lock_expires_at < NOW()
		 RETURNING id, team_id, task_number, subject`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recovered := []RecoveredTask{}
	for rows.Next() {
		var r RecoveredTask
		rows.Scan(&r.ID, &r.TeamID, &r.TaskNumber, &r.Subject)
		recovered = append(recovered, r)
	}
	return recovered, nil
}

// ListRecoverableTasks returns tasks that are stale (lock expired) but not yet recovered.
func (s *Store) ListRecoverableTasks(ctx context.Context, teamID string) ([]Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, subject, status, task_number, identifier, COALESCE(owner_agent_id::text,''),
		 locked_at, lock_expires_at, created_at
		 FROM team_tasks WHERE team_id = $1 AND status IN ('assigned','in_progress')
		 AND lock_expires_at < NOW() ORDER BY lock_expires_at`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Status, &t.TaskNumber, &t.Identifier,
			&t.OwnerAgentID, &t.LockedAt, &t.LockExpiresAt, &t.CreatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// ListStalledTasks returns tasks that haven't been updated in the given duration.
func (s *Store) ListStalledTasks(ctx context.Context, teamID string, staleDuration time.Duration) ([]Task, error) {
	cutoff := time.Now().Add(-staleDuration)
	rows, err := s.pool.Query(ctx,
		`SELECT id, team_id, subject, status, task_number, identifier, COALESCE(owner_agent_id::text,''),
		 progress_percent, COALESCE(progress_step,''), updated_at
		 FROM team_tasks WHERE team_id = $1 AND status IN ('assigned','in_progress')
		 AND updated_at < $2 ORDER BY updated_at`, teamID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []Task{}
	for rows.Next() {
		var t Task
		rows.Scan(&t.ID, &t.TeamID, &t.Subject, &t.Status, &t.TaskNumber, &t.Identifier,
			&t.OwnerAgentID, &t.ProgressPercent, &t.ProgressStep, &t.UpdatedAt)
		tasks = append(tasks, t)
	}
	return tasks, nil
}
