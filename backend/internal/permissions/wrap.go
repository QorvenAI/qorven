// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

import (
	"context"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// GatedToolOptions configures a gated tool wrapper.
type GatedToolOptions struct {
	// SessionIDFromArgs extracts the session id from the tool's args so
	// the gate persists the correct relationship. When nil the wrapper
	// reads args["session_id"] directly. The tool runner must populate
	// this arg before dispatch — orchestrator integration does so; ad-hoc
	// callers (tests, one-off HTTP handlers) need to set it too.
	SessionIDFromArgs func(args map[string]any) string

	// AgentKey is the soul key the gate records. Zero value is OK.
	AgentKey string

	// Reason is a short human-readable string shown in the UI.
	Reason string

	// RequestedBy is the actor the gate attributes the request to.
	// Typically the session owner or the agent key when the agent is
	// the implicit caller.
	RequestedBy string

	// Timeout overrides the gate's default for this tool.
	Timeout time.Duration
}

// defaultSessionIDFromArgs reads "session_id" from the tool args map.
func defaultSessionIDFromArgs(args map[string]any) string {
	if args == nil {
		return ""
	}
	if s, ok := args["session_id"].(string); ok {
		return s
	}
	return ""
}

// Wrap decorates an existing tool so every Execute call is gated.
// Wrapped tools preserve Name/Description/Parameters unchanged — the
// gate is invisible to the LLM's function-call interface. When the user
// denies, the wrapped tool returns an ErrorResult referencing the
// permission request id so admins can audit.
func Wrap(gate *Gate, inner tools.Tool, opts GatedToolOptions) tools.Tool {
	if opts.SessionIDFromArgs == nil {
		opts.SessionIDFromArgs = defaultSessionIDFromArgs
	}
	return &gatedTool{inner: inner, gateFn: func() *Gate { return gate }, opts: opts}
}

// WrapLazy is like Wrap but resolves the gate at Execute time via the
// supplied getter. Use this when the tool is registered before the
// gate is constructed (common when tool registration happens during
// early bootstrap and the gate depends on the DB being ready).
//
// If getGate returns nil at Execute time the wrapper fails closed with
// "gate not configured" — the same behavior as Wrap(nil, ...). Callers
// that want a permissive fallback must opt in explicitly.
func WrapLazy(getGate func() *Gate, inner tools.Tool, opts GatedToolOptions) tools.Tool {
	if getGate == nil {
		return Wrap(nil, inner, opts)
	}
	if opts.SessionIDFromArgs == nil {
		opts.SessionIDFromArgs = defaultSessionIDFromArgs
	}
	return &gatedTool{inner: inner, gateFn: getGate, opts: opts}
}

type gatedTool struct {
	inner  tools.Tool
	gateFn func() *Gate
	opts   GatedToolOptions
}

// IsGated returns true when t was produced by Wrap or WrapLazy. The
// destructive-tool manifest test in internal/tools uses this to
// assert every sensitive tool has been wrapped before CI passes.
//
// Callers MUST NOT use this for authorization decisions at request
// time — the gate itself is the authority. IsGated is a test/audit
// helper only.
func IsGated(t tools.Tool) bool {
	_, ok := t.(*gatedTool)
	return ok
}

func (g *gatedTool) Name() string              { return g.inner.Name() }
func (g *gatedTool) Description() string       { return g.inner.Description() }
func (g *gatedTool) Parameters() map[string]any { return g.inner.Parameters() }

func (g *gatedTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	var gate *Gate
	if g.gateFn != nil {
		gate = g.gateFn()
	}
	if gate == nil {
		// Defensive: a misconfigured gate should never silently skip the
		// prompt. Return a clear error instead of running the tool.
		return tools.ErrorResult("permissions: gate not configured")
	}
	verdict, err := gate.Request(ctx, RequestInput{
		SessionID:   g.opts.SessionIDFromArgs(args),
		AgentKey:    g.opts.AgentKey,
		Tool:        g.inner.Name(),
		Args:        args,
		Reason:      g.opts.Reason,
		RequestedBy: g.opts.RequestedBy,
		// propagate tenant so permission_requests.tenant_id
		// is a real UUID (not the 'default' literal) and the
		// multi-tenant RLS policy accepts the row. The ctx is
		// stamped by agent.Loop.executeTool.
		TenantID: tools.TenantIDFromCtx(ctx),
		UserID:   tools.UserIDFromCtx(ctx),
		AgentID:  tools.AgentIDFromCtx(ctx),
		Timeout:  g.opts.Timeout,
	})
	if err != nil {
		if IsExpired(err) {
			return tools.ErrorResult("permission request expired — user did not respond in time")
		}
		return tools.ErrorResult("permission gate failed: " + err.Error())
	}
	if !verdict.Allowed() {
		note := ""
		if verdict.Request != nil {
			note = verdict.Request.Note
		}
		msg := "user denied this tool call"
		if note != "" {
			msg += ": " + note
		}
		return tools.ErrorResult(msg)
	}
	return g.inner.Execute(ctx, args)
}
