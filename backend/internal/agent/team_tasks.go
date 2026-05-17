// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TeamTask represents a unit of work in the shared team task list.
// Leader agents create tasks, member agents claim and complete them.
type TeamTask struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	TeamID      string    `json:"team_id"`
	TaskNumber  int       `json:"task_number"`
	Subject     string    `json:"subject"`
	Description string    `json:"description"`
	Status      TaskStatus `json:"status"`
	Priority    int       `json:"priority"`
	AssigneeID  string    `json:"assignee_id,omitempty"`
	CreatorID   string    `json:"creator_id"`
	Result      string    `json:"result,omitempty"`
	FailReason  string    `json:"fail_reason,omitempty"`
	Progress    int       `json:"progress"` // 0-100
	BlockedBy   []string  `json:"blocked_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TaskStatus is the lifecycle state of a team task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskCancelled  TaskStatus = "cancelled"
)

// TeamTaskStore manages the shared task list (in-memory for now, DB later).
type TeamTaskStore struct {
	tasks      map[string]*TeamTask
	nextNumber int
	mu         sync.Mutex
	onEvent    func(eventType string, task *TeamTask) // optional callback for bus integration
}

// NewTeamTaskStore creates an empty task store.
func NewTeamTaskStore() *TeamTaskStore {
	return &TeamTaskStore{tasks: make(map[string]*TeamTask), nextNumber: 1}
}

// SetOnEvent sets the callback for task lifecycle events.
// Called with event type ("created", "claimed", "completed", "failed", "cancelled") and the task.
func (s *TeamTaskStore) SetOnEvent(fn func(string, *TeamTask)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = fn
}

func (s *TeamTaskStore) emitEvent(eventType string, task *TeamTask) {
	if s.onEvent != nil {
		go s.onEvent(eventType, task)
	}
}

// Create adds a new task. Returns the task ID.
func (s *TeamTaskStore) Create(ctx context.Context, task TeamTask) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task.ID = uuid.New().String()
	task.Status = TaskPending
	task.CreatedAt = time.Now()
	task.TaskNumber = s.nextNumber
	s.nextNumber++

	s.tasks[task.ID] = &task
	slog.Info("team.task.created", "id", task.ID, "num", task.TaskNumber, "subject", task.Subject, "assignee", task.AssigneeID)
	s.emitEvent("created", &task)
	return task.ID, nil
}

// Claim atomically claims a pending task for an agent. Only one agent can claim.
func (s *TeamTaskStore) Claim(ctx context.Context, taskID, agentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if task.Status != TaskPending {
		return fmt.Errorf("task %s is %s, cannot claim", taskID, task.Status)
	}

	// Check blocked_by dependencies
	for _, blockerID := range task.BlockedBy {
		if blocker, exists := s.tasks[blockerID]; exists {
			if blocker.Status != TaskCompleted {
				return fmt.Errorf("task %s blocked by task %s (status: %s)", taskID, blockerID, blocker.Status)
			}
		}
	}

	task.Status = TaskInProgress
	task.AssigneeID = agentID
	slog.Info("team.task.claimed", "id", taskID, "agent", agentID)
	s.emitEvent("claimed", task)
	return nil
}

// Complete marks a task as done with a result summary.
func (s *TeamTaskStore) Complete(ctx context.Context, taskID, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if task.Status == TaskCompleted {
		return nil // idempotent
	}
	if task.Status != TaskInProgress && task.Status != TaskPending {
		return fmt.Errorf("task %s is %s, cannot complete", taskID, task.Status)
	}

	now := time.Now()
	task.Status = TaskCompleted
	task.Result = result
	task.Progress = 100
	task.CompletedAt = &now
	slog.Info("team.task.completed", "id", taskID, "num", task.TaskNumber)
	s.emitEvent("completed", task)
	return nil
}

// Fail marks a task as failed (blocker escalation).
func (s *TeamTaskStore) Fail(ctx context.Context, taskID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if task.Status != TaskInProgress {
		return fmt.Errorf("task %s is %s, cannot fail", taskID, task.Status)
	}

	task.Status = TaskFailed
	task.FailReason = reason
	slog.Warn("team.task.failed", "id", taskID, "num", task.TaskNumber, "reason", truncateLog(reason, 100))
	s.emitEvent("failed", task)
	return nil
}

// Cancel cancels a task (leader only in practice).
func (s *TeamTaskStore) Cancel(ctx context.Context, taskID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if task.Status == TaskCompleted || task.Status == TaskCancelled {
		return nil
	}

	task.Status = TaskCancelled
	task.FailReason = reason
	return nil
}

// UpdateProgress sets the progress percentage (0-100).
func (s *TeamTaskStore) UpdateProgress(ctx context.Context, taskID string, percent int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %s not found", taskID)
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	task.Progress = percent
	return nil
}

// List returns tasks filtered by status (empty = all).
func (s *TeamTaskStore) List(ctx context.Context, teamID, status string) []*TeamTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*TeamTask
	for _, t := range s.tasks {
		if teamID != "" && t.TeamID != teamID {
			continue
		}
		if status != "" && string(t.Status) != status {
			continue
		}
		result = append(result, t)
	}
	return result
}

// Get returns a single task by ID.
func (s *TeamTaskStore) Get(ctx context.Context, taskID string) (*TeamTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	return task, nil
}

// FindDuplicate checks if an active task with the same subject exists.
func (s *TeamTaskStore) FindDuplicate(ctx context.Context, teamID, subject string) *TeamTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	subjectLower := strings.ToLower(strings.TrimSpace(subject))
	for _, t := range s.tasks {
		if t.TeamID == teamID &&
			(t.Status == TaskPending || t.Status == TaskInProgress) &&
			strings.ToLower(strings.TrimSpace(t.Subject)) == subjectLower {
			return t
		}
	}
	return nil
}

// PendingDependents returns tasks that were blocked by the given task
// and are now unblocked (all blockers completed).
func (s *TeamTaskStore) PendingDependents(ctx context.Context, completedTaskID string) []*TeamTask {
	s.mu.Lock()
	defer s.mu.Unlock()

	var unblocked []*TeamTask
	for _, t := range s.tasks {
		if t.Status != TaskPending {
			continue
		}
		for _, blockerID := range t.BlockedBy {
			if blockerID == completedTaskID {
				// Check if ALL blockers are now completed
				allDone := true
				for _, bid := range t.BlockedBy {
					if blocker, exists := s.tasks[bid]; exists {
						if blocker.Status != TaskCompleted {
							allDone = false
							break
						}
					}
				}
				if allDone {
					unblocked = append(unblocked, t)
				}
				break
			}
		}
	}
	return unblocked
}

func truncateLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
