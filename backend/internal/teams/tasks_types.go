// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package teams

import (
	"encoding/json"
	"time"
)

// Task status constants.
const (
	TaskStatusPending    = "pending"
	TaskStatusAssigned   = "assigned"
	TaskStatusInProgress = "in_progress"
	TaskStatusReview     = "review"
	TaskStatusCompleted  = "completed"
	TaskStatusCancelled  = "cancelled"
	TaskStatusFailed     = "failed"
	TaskStatusBlocked    = "blocked"
)

// Task priority levels.
const (
	PriorityLow    = 0
	PriorityNormal = 1
	PriorityHigh   = 2
	PriorityUrgent = 3
)

// Task represents a team task with full lifecycle tracking.
type Task struct {
	ID              string          `json:"id"`
	TeamID          string          `json:"team_id"`
	Subject         string          `json:"subject"`
	Description     string          `json:"description,omitempty"`
	Status          string          `json:"status"`
	OwnerAgentID    string          `json:"owner_agent_id,omitempty"`
	Priority        int             `json:"priority"`
	Result          string          `json:"result,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	UserID          string          `json:"user_id,omitempty"`
	Channel         string          `json:"channel,omitempty"`
	TaskType        string          `json:"task_type,omitempty"`
	TaskNumber      int             `json:"task_number,omitempty"`
	Identifier      string          `json:"identifier,omitempty"`
	CreatedByAgent  string          `json:"created_by_agent_id,omitempty"`
	AssigneeUserID  string          `json:"assignee_user_id,omitempty"`
	ParentID        string          `json:"parent_id,omitempty"`
	ChatID          string          `json:"chat_id,omitempty"`
	BlockedBy       []string        `json:"blocked_by,omitempty"`
	LockedAt        *time.Time      `json:"locked_at,omitempty"`
	LockExpiresAt   *time.Time      `json:"lock_expires_at,omitempty"`
	ProgressPercent int             `json:"progress_percent,omitempty"`
	ProgressStep    string          `json:"progress_step,omitempty"`
	FollowupAt      *time.Time      `json:"followup_at,omitempty"`
	FollowupCount   int             `json:"followup_count,omitempty"`
	FollowupMax     int             `json:"followup_max,omitempty"`
	FollowupMsg     string          `json:"followup_message,omitempty"`
	FollowupChannel string          `json:"followup_channel,omitempty"`
	FollowupChatID  string          `json:"followup_chat_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// TaskComment is a note or blocker comment on a task.
type TaskComment struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	AgentID     string    `json:"agent_id,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	Content     string    `json:"content"`
	CommentType string    `json:"comment_type,omitempty"` // "note" or "blocker"
	AgentKey    string    `json:"agent_key,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskEvent is an audit log entry for task state changes.
type TaskEvent struct {
	ID        string          `json:"id"`
	TaskID    string          `json:"task_id"`
	EventType string          `json:"event_type"` // created, assigned, completed, failed, etc.
	ActorType string          `json:"actor_type"` // "agent" or "human"
	ActorID   string          `json:"actor_id"`
	Data      json.RawMessage `json:"data,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// TaskAttachment is a file attached to a task.
type TaskAttachment struct {
	ID              string          `json:"id"`
	TaskID          string          `json:"task_id"`
	TeamID          string          `json:"team_id"`
	ChatID          string          `json:"chat_id,omitempty"`
	Path            string          `json:"path"`
	FileSize        int64           `json:"file_size"`
	MimeType        string          `json:"mime_type,omitempty"`
	CreatedByAgent  string          `json:"created_by_agent_id,omitempty"`
	CreatedBySender string          `json:"created_by_sender_id,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// ScopeEntry identifies a channel+chat scope for task filtering.
type ScopeEntry struct {
	Channel string `json:"channel"`
	ChatID  string `json:"chat_id"`
}

// RecoveredTask holds info about a task recovered from stale state.
type RecoveredTask struct {
	ID         string `json:"id"`
	TeamID     string `json:"team_id"`
	TaskNumber int    `json:"task_number"`
	Subject    string `json:"subject"`
}
