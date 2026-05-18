// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"

	"github.com/google/uuid"
)

// BuiltinToolTenantConfig represents a per-tenant override for a builtin tool.
type BuiltinToolTenantConfig struct {
	ToolName string    `json:"tool_name"`
	TenantID uuid.UUID `json:"tenant_id"`
	Enabled  *bool     `json:"enabled,omitempty"`
}

// BuiltinToolTenantConfigStore manages per-tenant builtin tool overrides.
type BuiltinToolTenantConfigStore interface {
	ListDisabled(ctx context.Context, tenantID uuid.UUID) ([]string, error)
	ListAll(ctx context.Context, tenantID uuid.UUID) (map[string]bool, error)
	Set(ctx context.Context, tenantID uuid.UUID, toolName string, enabled bool) error
	Delete(ctx context.Context, tenantID uuid.UUID, toolName string) error
}

// SkillTenantConfig represents a per-tenant override for a skill.
type SkillTenantConfig struct {
	SkillID  uuid.UUID `json:"skill_id"`
	TenantID uuid.UUID `json:"tenant_id"`
	Enabled  bool      `json:"enabled"`
}

// SkillTenantConfigStore manages per-tenant skill visibility.
type SkillTenantConfigStore interface {
	ListDisabledSkillIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)
	ListAll(ctx context.Context, tenantID uuid.UUID) (map[uuid.UUID]bool, error)
	Set(ctx context.Context, tenantID uuid.UUID, skillID uuid.UUID, enabled bool) error
	Delete(ctx context.Context, tenantID uuid.UUID, skillID uuid.UUID) error
}
