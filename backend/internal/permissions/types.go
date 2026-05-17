// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package permissions implements the human-in-the-loop gate for
// dangerous tool calls. A gated tool calls PermissionGate.Request
// before it actually executes; the gate persists a row in
// permission_requests, emits permission.requested to the caller's
// SSE stream, and blocks the goroutine until either a matching
// permission.replied arrives (via POST /v1/permissions/{id}/reply)
// or the configured timeout fires.
//
// Phase 2 wires gh_push_file as the first gated tool. Additional
// tools opt in by wrapping their Execute via the ToolWrapper in
// internal/permissions/wrap.go.
package permissions

import (
	"encoding/json"
	"fmt"
	"time"
)

// State is the lifecycle of a single request.
type State string

const (
	StatePending State = "pending"
	StateAllowed State = "allowed"
	StateDenied  State = "denied"
	StateExpired State = "expired"
)

// Request is the persisted form of a gated tool call.
type Request struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id,omitempty"`
	PlanID      string          `json:"plan_id,omitempty"`
	NodeID      string          `json:"node_id,omitempty"`
	AgentKey    string          `json:"agent_key,omitempty"`
	Tool        string          `json:"tool"`
	Args        json.RawMessage `json:"args"`
	Reason      string          `json:"reason,omitempty"`
	State       State           `json:"state"`
	RequestedBy string          `json:"requested_by,omitempty"`
	RepliedBy   string          `json:"replied_by,omitempty"`
	Note        string          `json:"note,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	RepliedAt   *time.Time      `json:"replied_at,omitempty"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
}

// RequestInput is what the tool runner hands the gate.
type RequestInput struct {
	SessionID   string
	PlanID      string
	NodeID      string
	AgentKey    string
	Tool        string
	Args        any
	Reason      string
	RequestedBy string
	// TenantID attributes the request to its tenant. Propagated via
	// RunRequest.TenantID → gatedTool.Execute → this field. Required
	// for RLS + multi-tenant cleanup. When empty the row falls back
	// to the table's 'default' default — acceptable in single-tenant
	// installs, a real bug under multi-tenant RLS because the
	// 'default' literal fails the tenant_id::uuid cast inside the
	// RLS policy.
	TenantID string
	// UserID is the user the policy check runs against. When set and
	// a matching permission_policies row exists, Request() returns
	// immediately without prompting.
	UserID string
	// AgentID scopes the policy lookup to a specific Qor. When set, per-agent
	// policies are checked before workspace-wide policies.
	AgentID string
	// Timeout caps how long the gate blocks waiting for a reply. When
	// zero, uses the gate's default. Negative means "no timeout"
	// (generally unsafe — the call will block indefinitely).
	Timeout time.Duration
}

// Decision is the user's verdict delivered via the reply endpoint.
type Decision string

const (
	DecisionAllow        Decision = "allow"
	DecisionDeny         Decision = "deny"
	DecisionAlwaysAllow  Decision = "allow_always"
	DecisionAllowSession Decision = "allow_session" // transient: allow for this session only
	DecisionAllow1h      Decision = "allow_1h"      // transient: allow for the next 1 hour
)

// ReplyInput is the payload for POST /v1/permissions/{id}/reply.
type ReplyInput struct {
	Decision  Decision `json:"decision"`
	Note      string   `json:"note,omitempty"`
	RepliedBy string   `json:"-"` // populated from request context
}

// Verdict is returned by PermissionGate.Request once a reply arrives
// or the timeout fires.
type Verdict struct {
	Decision Decision
	Request  *Request
	Expired  bool // true when the timeout fired before any reply
}

// Allowed returns true when the caller may proceed with the tool call.
func (v *Verdict) Allowed() bool { return v != nil && v.Decision == DecisionAllow && !v.Expired }

// ErrDenied signals a tool handler that the user refused. Wrap in the
// tool's error result.
type DeniedError struct {
	RequestID string
	Note      string
}

func (e *DeniedError) Error() string {
	if e.Note != "" {
		return fmt.Sprintf("permission denied (request %s): %s", e.RequestID, e.Note)
	}
	return fmt.Sprintf("permission denied (request %s)", e.RequestID)
}

// IsDenied reports whether err is a DeniedError.
func IsDenied(err error) bool {
	_, ok := err.(*DeniedError)
	return ok
}

// ExpiredError signals the auto-deny path fired.
type ExpiredError struct {
	RequestID string
	After     time.Duration
}

func (e *ExpiredError) Error() string {
	return fmt.Sprintf("permission request %s expired after %s", e.RequestID, e.After)
}

// IsExpired reports whether err is an ExpiredError.
func IsExpired(err error) bool {
	_, ok := err.(*ExpiredError)
	return ok
}

// PermScope is the three-tier permission level for a tool.
// It is distinct from Scope (API-key scopes) in policy.go.
type PermScope string

const (
	ScopeAutoApproved PermScope = "auto_approved" // runs without prompting
	ScopeAskFirst     PermScope = "ask_first"     // prompts once, then remembers
	ScopeBlocked      PermScope = "blocked"       // never runs; danger-confirm to unblock
)

// PolicyEntry is a single row from permission_policies.
type PolicyEntry struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	UserID    string    `json:"user_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	Tool      string    `json:"tool"`
	Scope     PermScope `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
}

// IsAutoApproved reports whether this policy short-circuits the gate.
func (p PolicyEntry) IsAutoApproved() bool { return p.Scope == ScopeAutoApproved }

// IsBlocked reports whether this policy hard-blocks the tool.
func (p PolicyEntry) IsBlocked() bool { return p.Scope == ScopeBlocked }
