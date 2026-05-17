// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/tools"
)

// RunContext holds all context values injected into the agent loop.
type RunContext struct {
	AgentID     uuid.UUID
	AgentKey    string
	TenantID    string
	UserID      string
	AgentType   string // "open" or "predefined"
	SenderID    string
	Workspace   string
	ChannelType string
	SessionKey  string
}

type runContextKey struct{}

// WithRunContext injects RunContext into context.
func WithRunContext(ctx context.Context, rc *RunContext) context.Context {
	return context.WithValue(ctx, runContextKey{}, rc)
}

// RunContextFromCtx retrieves RunContext from context.
func RunContextFromCtx(ctx context.Context) *RunContext {
	if v := ctx.Value(runContextKey{}); v != nil {
		return v.(*RunContext)
	}
	return nil
}

// ContextInjector enriches context with agent, tenant, user, workspace values.
type ContextInjector struct {
	agentID       uuid.UUID
	agentKey      string
	tenantID      string
	workspace     string
	inputGuard    *InputGuard
	injectionAction string // "log", "warn", "block", "off"
	maxMessageChars int
}

// NewContextInjector creates a new injector.
func NewContextInjector(agentID uuid.UUID, agentKey, tenantID, workspace string) *ContextInjector {
	return &ContextInjector{
		agentID:         agentID,
		agentKey:        agentKey,
		tenantID:        tenantID,
		workspace:       workspace,
		injectionAction: "warn",
		maxMessageChars: 32000,
	}
}

// SetInputGuard configures input guard for injection detection.
func (ci *ContextInjector) SetInputGuard(guard *InputGuard, action string) {
	ci.inputGuard = guard
	ci.injectionAction = action
}

// SetMaxMessageChars sets the max message size before truncation.
func (ci *ContextInjector) SetMaxMessageChars(max int) {
	ci.maxMessageChars = max
}

// InjectRequest holds the request parameters for context injection.
type InjectRequest struct {
	UserID      string
	SenderID    string
	Channel     string
	ChannelType string
	ChatID      string
	SessionKey  string
	Message     string
	AgentType   string
}

// InjectResult holds the result of context injection.
type InjectResult struct {
	Ctx     context.Context
	Message string // potentially truncated
	Blocked bool
	Error   error
}

// Inject enriches context with all necessary values for the agent loop.
func (ci *ContextInjector) Inject(ctx context.Context, req *InjectRequest) InjectResult {
	// Inject agent UUID
	if ci.agentID != uuid.Nil {
		ctx = withAgentID(ctx, ci.agentID)
	}
	if ci.agentKey != "" {
		ctx = withAgentKey(ctx, ci.agentKey)
	}

	// Inject tenant
	if ci.tenantID != "" {
		ctx = withTenantID(ctx, ci.tenantID)
	}

	// Inject user
	if req.UserID != "" {
		ctx = withUserID(ctx, req.UserID)
	}

	// Inject agent type
	if req.AgentType != "" {
		ctx = withAgentType(ctx, req.AgentType)
	}

	// Inject sender ID for permission checks
	if req.SenderID != "" {
		ctx = withSenderID(ctx, req.SenderID)
	}

	// Inject channel type
	if req.ChannelType != "" {
		ctx = withChannelType(ctx, req.ChannelType)
	}

	// Workspace setup
	if ci.workspace != "" {
		effectiveWorkspace := ci.workspace
		if req.UserID != "" {
			// Per-user workspace isolation
			effectiveWorkspace = fmt.Sprintf("%s/%s", ci.workspace, sanitizePathSegment(req.UserID))
		}
		if err := os.MkdirAll(effectiveWorkspace, 0755); err != nil {
			slog.Warn("failed to create workspace", "workspace", effectiveWorkspace, "error", err)
		}
		ctx = tools.WithWorkspace(ctx, effectiveWorkspace)
	}

	// Security: scan for injection patterns
	message := req.Message
	if ci.inputGuard != nil && ci.injectionAction != "off" {
		if matches := ci.inputGuard.Scan(message); len(matches) > 0 {
			matchStr := strings.Join(matches, ",")
			switch ci.injectionAction {
			case "block":
				slog.Warn("security.injection_blocked",
					"agent", ci.agentKey, "user", req.UserID,
					"patterns", matchStr)
				return InjectResult{
					Blocked: true,
					Error:   fmt.Errorf("message blocked: potential prompt injection detected (%s)", matchStr),
				}
			case "log":
				slog.Info("security.injection_detected",
					"agent", ci.agentKey, "user", req.UserID,
					"patterns", matchStr)
			default: // "warn"
				slog.Warn("security.injection_detected",
					"agent", ci.agentKey, "user", req.UserID,
					"patterns", matchStr)
			}
		}
	}

	// Security: truncate oversized messages
	if ci.maxMessageChars > 0 && len(message) > ci.maxMessageChars {
		originalLen := len(message)
		message = message[:ci.maxMessageChars] +
			fmt.Sprintf("\n\n[System: Message truncated from %d to %d characters. "+
				"Please send shorter messages or use read_file for large content.]",
				originalLen, ci.maxMessageChars)
		slog.Warn("security.message_truncated",
			"agent", ci.agentKey, "user", req.UserID,
			"original_len", originalLen, "truncated_to", ci.maxMessageChars)
	}

	// Build RunContext
	rc := &RunContext{
		AgentID:     ci.agentID,
		AgentKey:    ci.agentKey,
		TenantID:    ci.tenantID,
		UserID:      req.UserID,
		AgentType:   req.AgentType,
		SenderID:    req.SenderID,
		Workspace:   tools.WorkspaceFromCtx(ctx),
		ChannelType: req.ChannelType,
		SessionKey:  req.SessionKey,
	}
	ctx = WithRunContext(ctx, rc)

	return InjectResult{
		Ctx:     ctx,
		Message: message,
	}
}

// Context key types
type agentIDKey struct{}
type agentKeyKey struct{}
type tenantIDKey struct{}
type userIDKey struct{}
type agentTypeKey struct{}
type senderIDKey struct{}
type channelTypeKey struct{}

func withAgentID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, agentIDKey{}, id)
}

func withAgentKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, agentKeyKey{}, key)
}

func withTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, tenantIDKey{}, id)
}

func withUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

func withAgentType(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, agentTypeKey{}, t)
}

func withSenderID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, senderIDKey{}, id)
}

func withChannelType(ctx context.Context, t string) context.Context {
	return context.WithValue(ctx, channelTypeKey{}, t)
}

// AgentIDFromCtx retrieves agent UUID from context.
func AgentIDFromCtx(ctx context.Context) uuid.UUID {
	if v := ctx.Value(agentIDKey{}); v != nil {
		return v.(uuid.UUID)
	}
	return uuid.Nil
}

// AgentKeyFromCtx retrieves agent key from context.
func AgentKeyFromCtx(ctx context.Context) string {
	if v := ctx.Value(agentKeyKey{}); v != nil {
		return v.(string)
	}
	return ""
}

// TenantIDFromCtx retrieves tenant ID from context.
func TenantIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(tenantIDKey{}); v != nil {
		return v.(string)
	}
	return ""
}

// UserIDFromCtx retrieves user ID from context.
func UserIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(userIDKey{}); v != nil {
		return v.(string)
	}
	return ""
}

// sanitizePathSegment removes unsafe characters from path segments.
func sanitizePathSegment(s string) string {
	// Replace unsafe characters with underscore
	unsafe := []string{"/", "\\", "..", ":", "*", "?", "\"", "<", ">", "|"}
	result := s
	for _, u := range unsafe {
		result = strings.ReplaceAll(result, u, "_")
	}
	return result
}
