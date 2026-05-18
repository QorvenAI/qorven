// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// Link direction constants.
const (
	LinkDirectionOutbound      = "outbound"
	LinkDirectionInbound       = "inbound"
	LinkDirectionBidirectional = "bidirectional"
)

// Link status constants.
const (
	LinkStatusActive   = "active"
	LinkStatusDisabled = "disabled"
)

// AgentLinkData represents a directional link between two agents for delegation.
type AgentLinkData struct {
	BaseModel
	SourceAgentID uuid.UUID       `json:"source_agent_id"`
	TargetAgentID uuid.UUID       `json:"target_agent_id"`
	Direction     string          `json:"direction"`
	TeamID        *uuid.UUID      `json:"team_id,omitempty"`
	Description   string          `json:"description,omitempty"`
	MaxConcurrent int             `json:"max_concurrent"`
	Settings      json.RawMessage `json:"settings,omitempty"`
	Status        string          `json:"status"`
	CreatedBy     string          `json:"created_by"`

	// Joined fields
	SourceAgentKey    string `json:"source_agent_key,omitempty"`
	TargetAgentKey    string `json:"target_agent_key,omitempty"`
	TargetDisplayName string `json:"target_display_name,omitempty"`
	TargetDescription string `json:"target_description,omitempty"`
	TeamName          string `json:"team_name,omitempty"`
	TargetIsTeamLead  bool   `json:"target_is_team_lead,omitempty"`
	TargetTeamName    string `json:"target_team_name,omitempty"`
}

// AgentLinkStore manages inter-agent delegation links.
type AgentLinkStore interface {
	CreateLink(ctx context.Context, link *AgentLinkData) error
	DeleteLink(ctx context.Context, id uuid.UUID) error
	UpdateLink(ctx context.Context, id uuid.UUID, updates map[string]any) error
	GetLink(ctx context.Context, id uuid.UUID) (*AgentLinkData, error)
	ListLinksFrom(ctx context.Context, agentID uuid.UUID) ([]AgentLinkData, error)
	ListLinksTo(ctx context.Context, agentID uuid.UUID) ([]AgentLinkData, error)
	CanDelegate(ctx context.Context, fromAgentID, toAgentID uuid.UUID) (bool, error)
	GetLinkBetween(ctx context.Context, fromAgentID, toAgentID uuid.UUID) (*AgentLinkData, error)
	DelegateTargets(ctx context.Context, fromAgentID uuid.UUID) ([]AgentLinkData, error)
	SearchDelegateTargets(ctx context.Context, fromAgentID uuid.UUID, query string, limit int) ([]AgentLinkData, error)
	SearchDelegateTargetsByEmbedding(ctx context.Context, fromAgentID uuid.UUID, embedding []float32, limit int) ([]AgentLinkData, error)
	DeleteTeamLinksForAgent(ctx context.Context, teamID, agentID uuid.UUID) error
}
