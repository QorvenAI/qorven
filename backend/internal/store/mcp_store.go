// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type MCPServerData struct {
	BaseModel
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name,omitempty"`
	Transport   string          `json:"transport"`
	Command     string          `json:"command,omitempty"`
	Args        json.RawMessage `json:"args,omitempty"`
	URL         string          `json:"url,omitempty"`
	Headers     json.RawMessage `json:"headers,omitempty"`
	Env         json.RawMessage `json:"env,omitempty"`
	APIKey      string          `json:"api_key,omitempty"`
	ToolPrefix  string          `json:"tool_prefix,omitempty"`
	TimeoutSec  int             `json:"timeout_sec"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	Enabled     bool            `json:"enabled"`
	CreatedBy   string          `json:"created_by"`
}

type MCPAgentGrant struct {
	ID              uuid.UUID       `json:"id"`
	ServerID        uuid.UUID       `json:"server_id"`
	AgentID         uuid.UUID       `json:"agent_id"`
	Enabled         bool            `json:"enabled"`
	ToolAllow       json.RawMessage `json:"tool_allow,omitempty"`
	ToolDeny        json.RawMessage `json:"tool_deny,omitempty"`
	ConfigOverrides json.RawMessage `json:"config_overrides,omitempty"`
	GrantedBy       string          `json:"granted_by"`
	CreatedAt       time.Time       `json:"created_at"`
}

type MCPUserGrant struct {
	ID        uuid.UUID       `json:"id"`
	ServerID  uuid.UUID       `json:"server_id"`
	UserID    string          `json:"user_id"`
	Enabled   bool            `json:"enabled"`
	ToolAllow json.RawMessage `json:"tool_allow,omitempty"`
	ToolDeny  json.RawMessage `json:"tool_deny,omitempty"`
	GrantedBy string          `json:"granted_by"`
	CreatedAt time.Time       `json:"created_at"`
}

type MCPAccessRequest struct {
	ID          uuid.UUID       `json:"id"`
	ServerID    uuid.UUID       `json:"server_id"`
	AgentID     *uuid.UUID      `json:"agent_id,omitempty"`
	UserID      string          `json:"user_id,omitempty"`
	Scope       string          `json:"scope"`
	Status      string          `json:"status"`
	Reason      string          `json:"reason,omitempty"`
	ToolAllow   json.RawMessage `json:"tool_allow,omitempty"`
	RequestedBy string          `json:"requested_by"`
	ReviewedBy  string          `json:"reviewed_by,omitempty"`
	ReviewedAt  *time.Time      `json:"reviewed_at,omitempty"`
	ReviewNote  string          `json:"review_note,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type MCPAccessInfo struct {
	Server    MCPServerData `json:"server"`
	ToolAllow []string      `json:"tool_allow,omitempty"`
	ToolDeny  []string      `json:"tool_deny,omitempty"`
}

type MCPUserCredentials struct {
	APIKey  string            `json:"api_key,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type MCPServerStore interface {
	CreateServer(ctx context.Context, s *MCPServerData) error
	GetServer(ctx context.Context, id uuid.UUID) (*MCPServerData, error)
	GetServerByName(ctx context.Context, name string) (*MCPServerData, error)
	ListServers(ctx context.Context) ([]MCPServerData, error)
	UpdateServer(ctx context.Context, id uuid.UUID, updates map[string]any) error
	DeleteServer(ctx context.Context, id uuid.UUID) error
	GrantToAgent(ctx context.Context, g *MCPAgentGrant) error
	RevokeFromAgent(ctx context.Context, serverID, agentID uuid.UUID) error
	ListAgentGrants(ctx context.Context, agentID uuid.UUID) ([]MCPAgentGrant, error)
	ListServerGrants(ctx context.Context, serverID uuid.UUID) ([]MCPAgentGrant, error)
	GrantToUser(ctx context.Context, g *MCPUserGrant) error
	RevokeFromUser(ctx context.Context, serverID uuid.UUID, userID string) error
	CountAgentGrantsByServer(ctx context.Context) (map[uuid.UUID]int, error)
	ListAccessible(ctx context.Context, agentID uuid.UUID, userID string) ([]MCPAccessInfo, error)
	CreateRequest(ctx context.Context, req *MCPAccessRequest) error
	ListPendingRequests(ctx context.Context) ([]MCPAccessRequest, error)
	ReviewRequest(ctx context.Context, requestID uuid.UUID, approved bool, reviewedBy, note string) error
	GetUserCredentials(ctx context.Context, serverID uuid.UUID, userID string) (*MCPUserCredentials, error)
	SetUserCredentials(ctx context.Context, serverID uuid.UUID, userID string, creds MCPUserCredentials) error
	DeleteUserCredentials(ctx context.Context, serverID uuid.UUID, userID string) error
}
