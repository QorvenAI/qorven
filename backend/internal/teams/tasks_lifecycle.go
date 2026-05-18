// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"fmt"
	"time"
)

// AssignTask assigns a task to an agent.
func (s *Store) AssignTask(ctx context.Context, taskID, agentID, teamID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET owner_agent_id = $1, status = $2, updated_at = $3
		 WHERE id = $4 AND team_id = $5`,
		agentID, TaskStatusAssigned, time.Now(), taskID, teamID)
	if err != nil {
		return fmt.Errorf("tasks.assign: %w", err)
	}
	s.recordTaskEventSimple(ctx, taskID, "assigned", "agent", agentID)
	return nil
}

// ClaimTask lets an agent claim an unassigned task.
func (s *Store) ClaimTask(ctx context.Context, taskID, agentID, teamID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET owner_agent_id = $1, status = $2, locked_at = $3,
		 lock_expires_at = $4, updated_at = $3
		 WHERE id = $5 AND team_id = $6 AND (owner_agent_id IS NULL OR owner_agent_id::text = '')`,
		agentID, TaskStatusAssigned, time.Now(), time.Now().Add(30*time.Minute), taskID, teamID)
	if err != nil {
		return fmt.Errorf("tasks.claim: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("task already claimed")
	}
	s.recordTaskEventSimple(ctx, taskID, "claimed", "agent", agentID)
	return nil
}

// CompleteTask marks a task as completed with a result.
func (s *Store) CompleteTask(ctx context.Context, taskID, teamID, result string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, result = $2, progress_percent = 100,
		 locked_at = NULL, lock_expires_at = NULL, followup_at = NULL, updated_at = $3
		 WHERE id = $4 AND team_id = $5`,
		TaskStatusCompleted, result, time.Now(), taskID, teamID)
	if err != nil {
		return fmt.Errorf("tasks.complete: %w", err)
	}
	s.recordTaskEventSimple(ctx, taskID, "completed", "agent", "")
	return nil
}

// CancelTask cancels a task with a reason.
func (s *Store) CancelTask(ctx context.Context, taskID, teamID, reason string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, result = $2,
		 locked_at = NULL, lock_expires_at = NULL, followup_at = NULL, updated_at = $3
		 WHERE id = $4 AND team_id = $5`,
		TaskStatusCancelled, reason, time.Now(), taskID, teamID)
	if err != nil {
		return fmt.Errorf("tasks.cancel: %w", err)
	}
	s.recordTaskEventSimple(ctx, taskID, "cancelled", "human", reason)
	return nil
}

// FailTask marks a task as failed.
func (s *Store) FailTask(ctx context.Context, taskID, teamID, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, result = $2,
		 locked_at = NULL, lock_expires_at = NULL, updated_at = $3
		 WHERE id = $4 AND team_id = $5`,
		TaskStatusFailed, errMsg, time.Now(), taskID, teamID)
	if err != nil {
		return fmt.Errorf("tasks.fail: %w", err)
	}
	s.recordTaskEventSimple(ctx, taskID, "failed", "agent", errMsg)
	return nil
}

// ReviewTask moves a task to review status.
func (s *Store) ReviewTask(ctx context.Context, taskID, teamID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, updated_at = $2 WHERE id = $3 AND team_id = $4`,
		TaskStatusReview, time.Now(), taskID, teamID)
	return err
}

// ApproveTask approves a reviewed task (marks complete).
func (s *Store) ApproveTask(ctx context.Context, taskID, teamID, comment string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, result = COALESCE(result,'') || ' [Approved: ' || $2 || ']',
		 progress_percent = 100, locked_at = NULL, lock_expires_at = NULL, followup_at = NULL, updated_at = $3
		 WHERE id = $4 AND team_id = $5`,
		TaskStatusCompleted, comment, time.Now(), taskID, teamID)
	if err != nil {
		return err
	}
	s.recordTaskEventSimple(ctx, taskID, "approved", "human", comment)
	return nil
}

// RejectTask rejects a reviewed task (sends back to in_progress).
func (s *Store) RejectTask(ctx context.Context, taskID, teamID, reason string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, updated_at = $2 WHERE id = $3 AND team_id = $4`,
		TaskStatusInProgress, time.Now(), taskID, teamID)
	if err != nil {
		return err
	}
	s.recordTaskEventSimple(ctx, taskID, "rejected", "human", reason)
	return nil
}

// ResetTaskStatus resets a task back to pending.
func (s *Store) ResetTaskStatus(ctx context.Context, taskID, teamID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE team_tasks SET status = $1, owner_agent_id = NULL, progress_percent = 0,
		 progress_step = '', locked_at = NULL, lock_expires_at = NULL, updated_at = $2
		 WHERE id = $3 AND team_id = $4`,
		TaskStatusPending, time.Now(), taskID, teamID)
	return err
}
