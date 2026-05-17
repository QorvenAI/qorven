// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey          contextKey = "qorven_user_id"
	AgentIDKey         contextKey = "qorven_agent_id"
	AgentKeyKey        contextKey = "qorven_agent_key"
	AgentTypeKey       contextKey = "qorven_agent_type"
	SenderIDKey        contextKey = "qorven_sender_id"
	SelfEvolveKey      contextKey = "qorven_self_evolve"
	LocaleKey          contextKey = "qorven_locale"
	TenantIDKey        contextKey = "qorven_tenant_id"
	TenantSlugKey      contextKey = "qorven_tenant_slug"
	CrossTenantKey     contextKey = "qorven_cross_tenant"
	RoleKey            contextKey = "qorven_role"
	SharedMemoryKey    contextKey = "qorven_shared_memory"
	SharedKGKey        contextKey = "qorven_shared_kg"
	ShellDenyGroupsKey contextKey = "qorven_shell_deny_groups"
)

const RoleOwner = "owner"

// --- User ID ---

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, UserIDKey, id)
}

func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok && v != "" {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.UserID
	}
	return ""
}

// --- Agent ID ---

func WithAgentID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, AgentIDKey, id)
}

func AgentIDFromContext(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(AgentIDKey).(uuid.UUID); ok && v != uuid.Nil {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.AgentID
	}
	return uuid.Nil
}

// --- Agent Key ---

func WithAgentKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, AgentKeyKey, key)
}

func AgentKeyFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AgentKeyKey).(string); ok && v != "" {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.AgentKey
	}
	return ""
}

// --- Agent Type ---

func WithAgentType(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, AgentTypeKey, t)
}

func AgentTypeFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AgentTypeKey).(string); ok && v != "" {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.AgentType
	}
	return ""
}

// --- Sender ID ---

func WithSenderID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, SenderIDKey, id)
}

func SenderIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(SenderIDKey).(string); ok && v != "" {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.SenderID
	}
	return ""
}

// --- Self Evolve ---

func WithSelfEvolve(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, SelfEvolveKey, v)
}

func SelfEvolveFromContext(ctx context.Context) bool {
	if v, ok := ctx.Value(SelfEvolveKey).(bool); ok {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.SelfEvolve
	}
	return false
}

// --- Locale ---

func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, LocaleKey, locale)
}

func LocaleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(LocaleKey).(string); ok && v != "" {
		return v
	}
	return "en"
}

// --- Tenant ID ---

func WithTenantID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, TenantIDKey, id)
}

func TenantIDFromContext(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(TenantIDKey).(uuid.UUID); ok && v != uuid.Nil {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.TenantID
	}
	return uuid.Nil
}

// --- Tenant Slug ---

func WithTenantSlug(ctx context.Context, slug string) context.Context {
	return context.WithValue(ctx, TenantSlugKey, slug)
}

func TenantSlugFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(TenantSlugKey).(string); ok {
		return v
	}
	return ""
}

// --- Role ---

func WithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, RoleKey, role)
}

func RoleFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(RoleKey).(string); ok {
		return v
	}
	return ""
}

func IsOwnerRole(ctx context.Context) bool {
	return RoleFromContext(ctx) == RoleOwner
}

// --- Cross Tenant ---

func WithCrossTenant(ctx context.Context) context.Context {
	return context.WithValue(ctx, CrossTenantKey, true)
}

func IsCrossTenant(ctx context.Context) bool {
	v, _ := ctx.Value(CrossTenantKey).(bool)
	return v
}

// --- Shared Memory ---

func WithSharedMemory(ctx context.Context) context.Context {
	return context.WithValue(ctx, SharedMemoryKey, true)
}

func IsSharedMemory(ctx context.Context) bool {
	if v, ok := ctx.Value(SharedMemoryKey).(bool); ok {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.SharedMemory
	}
	return false
}

func MemoryUserID(ctx context.Context) string {
	if IsSharedMemory(ctx) {
		return ""
	}
	return UserIDFromContext(ctx)
}

// --- Shared KG ---

func WithSharedKG(ctx context.Context) context.Context {
	return context.WithValue(ctx, SharedKGKey, true)
}

func IsSharedKG(ctx context.Context) bool {
	if v, ok := ctx.Value(SharedKGKey).(bool); ok {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.SharedKG
	}
	return false
}

func KGUserID(ctx context.Context) string {
	if IsSharedKG(ctx) {
		return ""
	}
	return UserIDFromContext(ctx)
}

// --- Shell Deny Groups ---

func WithShellDenyGroups(ctx context.Context, groups map[string]bool) context.Context {
	return context.WithValue(ctx, ShellDenyGroupsKey, groups)
}

func ShellDenyGroupsFromContext(ctx context.Context) map[string]bool {
	if v, _ := ctx.Value(ShellDenyGroupsKey).(map[string]bool); v != nil {
		return v
	}
	if rc := RunContextFromCtx(ctx); rc != nil {
		return rc.ShellDenyGroups
	}
	return nil
}
