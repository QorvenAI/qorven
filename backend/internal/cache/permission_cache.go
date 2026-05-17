// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/bus"
)

type agentAccessEntry struct {
	Allowed bool
	Role    string
}

// PermissionCache provides short-TTL caching for hot permission lookups.
type PermissionCache struct {
	tenantResolve Cache[uuid.UUID]
	tenantRole    Cache[string]
	agentAccess   Cache[agentAccessEntry]
	teamAccess    Cache[bool]
}

// NewPermissionCache creates a permission cache using the backend configured
// by QORVEN_CACHE_BACKEND (see NewFromEnv for accepted values).
func NewPermissionCache() *PermissionCache {
	return &PermissionCache{
		tenantResolve: NewFromEnv[uuid.UUID]("perm.tenant_resolve"),
		tenantRole:    NewFromEnv[string]("perm.tenant_role"),
		agentAccess:   NewFromEnv[agentAccessEntry]("perm.agent_access"),
		teamAccess:    NewFromEnv[bool]("perm.team_access"),
	}
}

const (
	tenantResolveTTL = 60 * time.Second
	tenantRoleTTL    = 30 * time.Second
	agentAccessTTL   = 30 * time.Second
	teamAccessTTL    = 30 * time.Second
)

func (pc *PermissionCache) GetTenantResolve(ctx context.Context, userID string) (uuid.UUID, bool) {
	return pc.tenantResolve.Get(ctx, userID)
}

func (pc *PermissionCache) SetTenantResolve(ctx context.Context, userID string, tenantID uuid.UUID) {
	pc.tenantResolve.Set(ctx, userID, tenantID, tenantResolveTTL)
}

func (pc *PermissionCache) GetTenantRole(ctx context.Context, tenantID uuid.UUID, userID string) (string, bool) {
	return pc.tenantRole.Get(ctx, tenantID.String()+":"+userID)
}

func (pc *PermissionCache) SetTenantRole(ctx context.Context, tenantID uuid.UUID, userID, role string) {
	pc.tenantRole.Set(ctx, tenantID.String()+":"+userID, role, tenantRoleTTL)
}

func (pc *PermissionCache) GetAgentAccess(ctx context.Context, agentID uuid.UUID, userID string) (bool, string, bool) {
	entry, ok := pc.agentAccess.Get(ctx, agentID.String()+":"+userID)
	if !ok {
		return false, "", false
	}
	return entry.Allowed, entry.Role, true
}

func (pc *PermissionCache) SetAgentAccess(ctx context.Context, agentID uuid.UUID, userID string, allowed bool, role string) {
	pc.agentAccess.Set(ctx, agentID.String()+":"+userID, agentAccessEntry{Allowed: allowed, Role: role}, agentAccessTTL)
}

func (pc *PermissionCache) GetTeamAccess(ctx context.Context, teamID uuid.UUID, userID string) (bool, bool) {
	return pc.teamAccess.Get(ctx, teamID.String()+":"+userID)
}

func (pc *PermissionCache) SetTeamAccess(ctx context.Context, teamID uuid.UUID, userID string, allowed bool) {
	pc.teamAccess.Set(ctx, teamID.String()+":"+userID, allowed, teamAccessTTL)
}

// HandleInvalidation processes a cache invalidation event from the bus.
func (pc *PermissionCache) HandleInvalidation(p bus.CacheInvalidatePayload) {
	slog.Debug("perm_cache.invalidated", "kind", p.Kind, "key", p.Key)
	ctx := context.Background()
	switch p.Kind {
	case "tenant_users":
		if p.Key != "" {
			pc.tenantResolve.Delete(ctx, p.Key)
			pc.tenantRole.Clear(ctx)
		} else {
			pc.tenantResolve.Clear(ctx)
			pc.tenantRole.Clear(ctx)
		}
	case "agent_access":
		if p.Key != "" {
			pc.agentAccess.DeleteByPrefix(ctx, p.Key+":")
		} else {
			pc.agentAccess.Clear(ctx)
		}
	case "team_access":
		if p.Key != "" {
			pc.teamAccess.DeleteByPrefix(ctx, p.Key+":")
		} else {
			pc.teamAccess.Clear(ctx)
		}
	}
}
