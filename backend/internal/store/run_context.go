// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"

	"github.com/google/uuid"

	"github.com/qorvenai/qorven/internal/sandbox"
)

// runContextKey is the context key for RunContext.
type runContextKey struct{}

// RunContext consolidates all agent-loop-injected context values into a single
// typed struct. This replaces many individual context.WithValue calls with one
// WithRunContext call.
type RunContext struct {
	// Identity
	AgentID   uuid.UUID
	AgentKey  string
	TenantID  uuid.UUID
	UserID    string
	AgentType string
	SenderID  string

	// Flags
	SelfEvolve          bool
	SharedMemory        bool
	SharedKG            bool
	RestrictToWorkspace bool

	// Tool configuration
	BuiltinToolSettings map[string][]byte
	ChannelType         string
	ParentModel         string
	ParentProvider      string
	SandboxCfg          *sandbox.Config
	ShellDenyGroups     map[string]bool

	// Workspace
	Workspace        string
	TeamWorkspace    string
	TeamID           string
	WorkspaceChannel string
	WorkspaceChatID  string
	TeamTaskID       string
	LeaderAgentID    string
	AgentToolKey     string
}

// WithRunContext stores a RunContext on the context.
func WithRunContext(ctx context.Context, rc *RunContext) context.Context {
	return context.WithValue(ctx, runContextKey{}, rc)
}

// RunContextFromCtx extracts RunContext from context. Returns nil if not set.
func RunContextFromCtx(ctx context.Context) *RunContext {
	rc, _ := ctx.Value(runContextKey{}).(*RunContext)
	return rc
}
