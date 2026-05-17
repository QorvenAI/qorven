// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// APIKeyData represents a gateway API key with scoped permissions.
type APIKeyData struct {
	ID         uuid.UUID  `json:"id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	KeyHash    string     `json:"-"`
	Scopes     []string   `json:"scopes"`
	OwnerID    string     `json:"owner_id,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Revoked    bool       `json:"revoked"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// APIKeyStore manages gateway API keys.
type APIKeyStore interface {
	Create(ctx context.Context, key *APIKeyData) error
	GetByHash(ctx context.Context, keyHash string) (*APIKeyData, error)
	List(ctx context.Context, ownerID string) ([]APIKeyData, error)
	Revoke(ctx context.Context, id uuid.UUID, ownerID string) error
	Delete(ctx context.Context, id uuid.UUID, ownerID string) error
	TouchLastUsed(ctx context.Context, id uuid.UUID) error
}
