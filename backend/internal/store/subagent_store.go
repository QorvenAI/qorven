// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SubagentTaskData represents a persisted subagent task for audit trail and cost attribution.
type SubagentTaskData struct {
	BaseModel
	TenantID       uuid.UUID      `json:"tenant_id"`
	ParentAgentKey string         `json:"parent_agent_key"`
	SessionKey     *string        `json:"session_key,omitempty"`
	Subject        string         `json:"subject"`
	Description    string         `json:"description"`
	Status         string         `json:"status"`
	Result         *string        `json:"result,omitempty"`
	Depth          int            `json:"depth"`
	Model          *string        `json:"model,omitempty"`
	Provider       *string        `json:"provider,omitempty"`
	Iterations     int            `json:"iterations"`
	InputTokens    int64          `json:"input_tokens"`
	OutputTokens   int64          `json:"output_tokens"`
	OriginChannel  *string        `json:"origin_channel,omitempty"`
	OriginChatID   *string        `json:"origin_chat_id,omitempty"`
	OriginPeerKind *string        `json:"origin_peer_kind,omitempty"`
	OriginUserID   *string        `json:"origin_user_id,omitempty"`
	SpawnedBy      *uuid.UUID     `json:"spawned_by,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	ArchivedAt     *time.Time     `json:"archived_at,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// SubagentTaskStore persists subagent task lifecycle for audit trail and cost attribution.
type SubagentTaskStore interface {
	Create(ctx context.Context, task *SubagentTaskData) error
	Get(ctx context.Context, id uuid.UUID) (*SubagentTaskData, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, result *string, iterations int, inputTokens, outputTokens int64) error
	ListByParent(ctx context.Context, parentAgentKey string, statusFilter string) ([]SubagentTaskData, error)
	ListBySession(ctx context.Context, sessionKey string) ([]SubagentTaskData, error)
	Archive(ctx context.Context, olderThan time.Duration) (int64, error)
	UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]any) error
}
