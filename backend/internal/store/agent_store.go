// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// Agent type constants.
const (
	AgentTypeOpen       = "open"
	AgentTypePredefined = "predefined"
)

// Agent status constants.
const (
	AgentStatusActive       = "active"
	AgentStatusInactive     = "inactive"
	AgentStatusSummoning    = "summoning"
	AgentStatusSummonFailed = "summon_failed"
)

// AgentData represents an agent (Soul) in the database.
type AgentData struct {
	BaseModel
	TenantID            uuid.UUID `json:"tenant_id"`
	AgentKey            string    `json:"agent_key"`
	DisplayName         string    `json:"display_name,omitempty"`
	Frontmatter         string    `json:"frontmatter,omitempty"`
	OwnerID             string    `json:"owner_id"`
	Provider            string    `json:"provider"`
	Model               string    `json:"model"`
	ContextWindow       int       `json:"context_window"`
	MaxToolIterations   int       `json:"max_tool_iterations"`
	Workspace           string    `json:"workspace"`
	RestrictToWorkspace bool      `json:"restrict_to_workspace"`
	AgentType           string    `json:"agent_type"`
	IsDefault           bool      `json:"is_default"`
	Status              string    `json:"status"`

	// Budget: optional monthly spending limit in cents (nil = unlimited).
	BudgetMonthlyCents *int `json:"budget_monthly_cents,omitempty"`

	// Per-agent JSONB config (nullable — nil means "use global defaults").
	ToolsConfig      json.RawMessage `json:"tools_config,omitempty"`
	SandboxConfig    json.RawMessage `json:"sandbox_config,omitempty"`
	SubagentsConfig  json.RawMessage `json:"subagents_config,omitempty"`
	MemoryConfig     json.RawMessage `json:"memory_config,omitempty"`
	CompactionConfig json.RawMessage `json:"compaction_config,omitempty"`
	ContextPruning   json.RawMessage `json:"context_pruning,omitempty"`
	OtherConfig      json.RawMessage `json:"other_config,omitempty"`
}

// ParseMaxTokens extracts max_tokens from other_config JSONB.
func (a *AgentData) ParseMaxTokens() int {
	if len(a.OtherConfig) == 0 {
		return 0
	}
	var cfg struct {
		MaxTokens int `json:"max_tokens"`
	}
	if json.Unmarshal(a.OtherConfig, &cfg) != nil {
		return 0
	}
	return cfg.MaxTokens
}

// ParseSelfEvolve extracts self_evolve from other_config JSONB.
func (a *AgentData) ParseSelfEvolve() bool {
	if len(a.OtherConfig) == 0 {
		return false
	}
	var cfg struct {
		SelfEvolve bool `json:"self_evolve"`
	}
	if json.Unmarshal(a.OtherConfig, &cfg) != nil {
		return false
	}
	return cfg.SelfEvolve
}

// ParseSkillEvolve extracts skill_evolve from other_config JSONB.
func (a *AgentData) ParseSkillEvolve() bool {
	if len(a.OtherConfig) == 0 {
		return false
	}
	var cfg struct {
		SkillEvolve bool `json:"skill_evolve"`
	}
	if json.Unmarshal(a.OtherConfig, &cfg) != nil {
		return false
	}
	return cfg.SkillEvolve
}

// ParseSkillNudgeInterval extracts skill_nudge_interval from other_config JSONB.
func (a *AgentData) ParseSkillNudgeInterval() int {
	if len(a.OtherConfig) == 0 {
		return 15
	}
	var cfg struct {
		SkillNudgeInterval *int `json:"skill_nudge_interval"`
	}
	if json.Unmarshal(a.OtherConfig, &cfg) != nil {
		return 15
	}
	if cfg.SkillNudgeInterval == nil {
		return 15
	}
	return *cfg.SkillNudgeInterval
}

// ParseShellDenyGroups extracts shell_deny_groups from other_config JSONB.
func (a *AgentData) ParseShellDenyGroups() map[string]bool {
	if len(a.OtherConfig) == 0 {
		return nil
	}
	var cfg struct {
		ShellDenyGroups map[string]bool `json:"shell_deny_groups"`
	}
	if json.Unmarshal(a.OtherConfig, &cfg) != nil || len(cfg.ShellDenyGroups) == 0 {
		return nil
	}
	return cfg.ShellDenyGroups
}

// WorkspaceSharingConfig controls per-user workspace isolation.
type WorkspaceSharingConfig struct {
	SharedDM            bool     `json:"shared_dm"`
	SharedGroup         bool     `json:"shared_group"`
	SharedUsers         []string `json:"shared_users,omitempty"`
	ShareMemory         bool     `json:"share_memory"`
	ShareKnowledgeGraph bool     `json:"share_knowledge_graph"`
}

// ParseWorkspaceSharing extracts workspace_sharing from other_config JSONB.
func (a *AgentData) ParseWorkspaceSharing() *WorkspaceSharingConfig {
	if len(a.OtherConfig) == 0 {
		return nil
	}
	var cfg struct {
		WS *WorkspaceSharingConfig `json:"workspace_sharing"`
	}
	if json.Unmarshal(a.OtherConfig, &cfg) != nil || cfg.WS == nil {
		return nil
	}
	if !cfg.WS.SharedDM && !cfg.WS.SharedGroup && len(cfg.WS.SharedUsers) == 0 && !cfg.WS.ShareMemory && !cfg.WS.ShareKnowledgeGraph {
		return nil
	}
	return cfg.WS
}

// AgentShareData represents an agent share grant.
type AgentShareData struct {
	BaseModel
	AgentID   uuid.UUID `json:"agent_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	GrantedBy string    `json:"granted_by"`
}

// ContextFile represents a context file stored in the database (backward compat).
type ContextFile struct {
	FileName string
	Content  string
}

// AgentContextFileData represents an agent-level context file.
type AgentContextFileData struct {
	AgentID  uuid.UUID `json:"agent_id"`
	FileName string    `json:"file_name"`
	Content  string    `json:"content"`
}

// UserContextFileData represents a per-user context file.
type UserContextFileData struct {
	AgentID  uuid.UUID `json:"agent_id"`
	UserID   string    `json:"user_id"`
	FileName string    `json:"file_name"`
	Content  string    `json:"content"`
}

// UserAgentOverrideData represents per-user agent overrides.
type UserAgentOverrideData struct {
	AgentID  uuid.UUID `json:"agent_id"`
	UserID   string    `json:"user_id"`
	Provider string    `json:"provider,omitempty"`
	Model    string    `json:"model,omitempty"`
}

// UserInstanceData represents a user instance for a predefined agent.
type UserInstanceData struct {
	UserID      string            `json:"user_id"`
	FirstSeenAt *string           `json:"first_seen_at,omitempty"`
	LastSeenAt  *string           `json:"last_seen_at,omitempty"`
	FileCount   int               `json:"file_count"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// AgentCRUDStore manages core agent CRUD operations.
type AgentCRUDStore interface {
	Create(ctx context.Context, agent *AgentData) error
	GetByKey(ctx context.Context, agentKey string) (*AgentData, error)
	GetByID(ctx context.Context, id uuid.UUID) (*AgentData, error)
	GetByIDUnscoped(ctx context.Context, id uuid.UUID) (*AgentData, error)
	GetByKeys(ctx context.Context, keys []string) ([]AgentData, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]AgentData, error)
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, ownerID string) ([]AgentData, error)
	GetDefault(ctx context.Context) (*AgentData, error)
}

// AgentAccessStore manages agent sharing and access control.
type AgentAccessStore interface {
	ShareAgent(ctx context.Context, agentID uuid.UUID, userID, role, grantedBy string) error
	RevokeShare(ctx context.Context, agentID uuid.UUID, userID string) error
	ListShares(ctx context.Context, agentID uuid.UUID) ([]AgentShareData, error)
	CanAccess(ctx context.Context, agentID uuid.UUID, userID string) (bool, string, error)
	ListAccessible(ctx context.Context, userID string) ([]AgentData, error)
}

// AgentContextStore manages agent-level and per-user context files and overrides.
type AgentContextStore interface {
	GetAgentContextFiles(ctx context.Context, agentID uuid.UUID) ([]ContextFile, error)
	SetAgentContextFile(ctx context.Context, agentID uuid.UUID, fileName, content string) error
	PropagateContextFile(ctx context.Context, agentID uuid.UUID, fileName string) (int, error)
	GetUserContextFiles(ctx context.Context, agentID uuid.UUID, userID string) ([]ContextFile, error)
	ListUserContextFilesByName(ctx context.Context, agentID uuid.UUID, fileName string) ([]UserContextFileData, error)
	SetUserContextFile(ctx context.Context, agentID uuid.UUID, userID, fileName, content string) error
	DeleteUserContextFile(ctx context.Context, agentID uuid.UUID, userID, fileName string) error
	MigrateUserDataOnMerge(ctx context.Context, oldUserIDs []string, newUserID string) error
	GetUserOverride(ctx context.Context, agentID uuid.UUID, userID string) (*UserAgentOverrideData, error)
	SetUserOverride(ctx context.Context, override *UserAgentOverrideData) error
}

// AgentProfileStore manages user-agent profiles and instances.
type AgentProfileStore interface {
	GetOrCreateUserProfile(ctx context.Context, agentID uuid.UUID, userID, workspace, channel string) (isNew bool, effectiveWorkspace string, err error)
	EnsureUserProfile(ctx context.Context, agentID uuid.UUID, userID string) error
	ListUserInstances(ctx context.Context, agentID uuid.UUID) ([]UserInstanceData, error)
	UpdateUserProfileMetadata(ctx context.Context, agentID uuid.UUID, userID string, metadata map[string]string) error
}

// AgentStore composes all agent sub-interfaces.
type AgentStore interface {
	AgentCRUDStore
	AgentAccessStore
	AgentContextStore
	AgentProfileStore
}
