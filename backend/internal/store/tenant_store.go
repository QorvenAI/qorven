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

// MasterTenantID is the fixed UUID v7 for the default/master tenant.
var MasterTenantID = uuid.MustParse("0193a5b0-7000-7000-8000-000000000001")

// Tenant status constants.
const (
	TenantStatusActive    = "active"
	TenantStatusSuspended = "suspended"
	TenantStatusArchived  = "archived"
)

// Tenant role constants.
const (
	TenantRoleOwner    = "owner"
	TenantRoleAdmin    = "admin"
	TenantRoleOperator = "operator"
	TenantRoleMember   = "member"
	TenantRoleViewer   = "viewer"
)

// TenantData represents a tenant in the database.
type TenantData struct {
	ID        uuid.UUID       `json:"id"`
	Name      string          `json:"name"`
	Slug      string          `json:"slug"`
	Status    string          `json:"status"`
	Settings  json.RawMessage `json:"settings,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// TenantUserData represents a user's membership in a tenant.
type TenantUserData struct {
	ID          uuid.UUID       `json:"id"`
	TenantID    uuid.UUID       `json:"tenant_id"`
	UserID      string          `json:"user_id"`
	DisplayName *string         `json:"display_name,omitempty"`
	Role        string          `json:"role"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// TenantStore manages tenants and tenant-user membership.
type TenantStore interface {
	CreateTenant(ctx context.Context, tenant *TenantData) error
	GetTenant(ctx context.Context, id uuid.UUID) (*TenantData, error)
	GetTenantBySlug(ctx context.Context, slug string) (*TenantData, error)
	ListTenants(ctx context.Context) ([]TenantData, error)
	UpdateTenant(ctx context.Context, id uuid.UUID, updates map[string]any) error
	AddUser(ctx context.Context, tenantID uuid.UUID, userID, role string) error
	RemoveUser(ctx context.Context, tenantID uuid.UUID, userID string) error
	GetUserRole(ctx context.Context, tenantID uuid.UUID, userID string) (string, error)
	ListUsers(ctx context.Context, tenantID uuid.UUID) ([]TenantUserData, error)
	ListUserTenants(ctx context.Context, userID string) ([]TenantUserData, error)
	ResolveUserTenant(ctx context.Context, userID string) (uuid.UUID, error)
	GetTenantUser(ctx context.Context, id uuid.UUID) (*TenantUserData, error)
	CreateTenantUserReturning(ctx context.Context, tenantID uuid.UUID, userID, displayName, role string) (*TenantUserData, error)
}
