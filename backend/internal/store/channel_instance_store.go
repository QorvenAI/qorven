// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// ChannelInstanceData represents a channel instance in the database.
type ChannelInstanceData struct {
	BaseModel
	TenantID    uuid.UUID       `json:"tenant_id,omitempty"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	ChannelType string          `json:"channel_type"`
	AgentID     uuid.UUID       `json:"agent_id"`
	Credentials []byte          `json:"-"`
	Config      json.RawMessage `json:"config"`
	Enabled     bool            `json:"enabled"`
	CreatedBy   string          `json:"created_by"`
}

// IsDefaultChannelInstance returns true if the instance name matches a default/seeded channel.
func IsDefaultChannelInstance(name string) bool {
	if strings.HasSuffix(name, "/default") {
		return true
	}
	switch name {
	case "telegram", "discord", "whatsapp":
		return true
	}
	return false
}

// ChannelInstanceListOpts configures channel instance listing.
type ChannelInstanceListOpts struct {
	Search string
	Limit  int
	Offset int
}

// ChannelInstanceStore manages channel instance definitions.
type ChannelInstanceStore interface {
	Create(ctx context.Context, inst *ChannelInstanceData) error
	Get(ctx context.Context, id uuid.UUID) (*ChannelInstanceData, error)
	GetByName(ctx context.Context, name string) (*ChannelInstanceData, error)
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListEnabled(ctx context.Context) ([]ChannelInstanceData, error)
	ListAll(ctx context.Context) ([]ChannelInstanceData, error)
	ListAllInstances(ctx context.Context) ([]ChannelInstanceData, error)
	ListAllEnabled(ctx context.Context) ([]ChannelInstanceData, error)
	ListPaged(ctx context.Context, opts ChannelInstanceListOpts) ([]ChannelInstanceData, error)
	CountInstances(ctx context.Context, opts ChannelInstanceListOpts) (int, error)
}
