// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import (
	"context"
	"strings"
)

// GovernanceHooks provides callbacks into the governance engine without creating
// import cycles. The gateway layer implements this interface and wires it into
// the agent loop at startup.
type GovernanceHooks struct {
	// EvaluatePolicy checks an event against the policy engine.
	// Returns action ("allow","deny","warn","require_approval","throttle","log","escalate")
	// and a human-readable reason. nil = governance disabled.
	EvaluatePolicy func(ctx context.Context, tenantID, agentID, triggerEvent string, eventCtx map[string]any) (action string, reason string)

	// CheckApproval determines whether an action requires approval.
	// Returns (requires bool, rule name, approver role). nil = no approval matrix.
	CheckApproval func(ctx context.Context, tenantID, actionType string, costUSD float64) (requires bool, ruleName string, approverRole string)

	// CheckSoD checks segregation of duties for the given agent+action.
	// Returns (violated bool, rule name). nil = no SoD enforcement.
	CheckSoD func(ctx context.Context, tenantID, agentID, action string) (violated bool, ruleName string)

	// RecordException logs a governance exception/variance.
	// nil = exception recording disabled.
	RecordException func(ctx context.Context, tenantID, agentID, exType, severity, description string, exCtx map[string]any)

	// RecordTaskTransition records a task state change in the governance audit trail.
	// nil = governance task tracking disabled.
	RecordTaskTransition func(ctx context.Context, tenantID, taskID, fromState, toState, changedBy, reason string)

	// RecordSLAEvent records an SLA measurement data point.
	// nil = SLA tracking disabled.
	RecordSLAEvent func(ctx context.Context, tenantID, slaID string, value float64, met bool)

	// LookupDesignation resolves an agent's designation to get its governance metadata.
	// Returns (modelTier, skillFamily string, canSpawn bool, approvalScope []string).
	// nil = designation enforcement disabled.
	LookupDesignation func(ctx context.Context, tenantID, agentKey string) (modelTier string, skillFamily string, canSpawn bool, approvalScope []string)

	// ResolveGovernedAction maps a concrete tool name to the governance action
	// vocabulary used by SoD rules (e.g. "exec" → "write_code"). Returns "" when
	// the tool has no SoD implication. nil = taxonomy resolution disabled.
	ResolveGovernedAction func(tool string) string

	// RecordGovernedAction logs that an agent performed a governed action so that
	// a later CheckViolation call can detect a conflict. nil = recording disabled.
	RecordGovernedAction func(ctx context.Context, tenantID, agentID, action string)

	// DetectPII reports whether the given text contains PII, using the real PII
	// engine. Used to enrich the output_deliver event so the "Block PII in
	// outputs" policy can match. nil = PII detection disabled.
	DetectPII func(content string) bool

	// HasBlockingOutputPolicy reports whether any enabled output_deliver policy
	// with action deny or require_approval exists for the given tenant. When
	// true the agent loop buffers live text instead of streaming it so the
	// governance gate can intercept it before the user sees it.
	// nil = assume no blocking policy (live streaming proceeds as normal).
	HasBlockingOutputPolicy func(tenantID string) bool
}

// SetGovernanceHooks wires the governance engine callbacks.
func (l *Loop) SetGovernanceHooks(h *GovernanceHooks) { l.governanceHooks = h }

// toolCategory classifies a tool name into a governance-relevant category.
func toolCategory(name string) string {
	switch {
	case name == "exec" || name == "bash" || name == "shell":
		return "shell"
	case name == "write_file" || name == "read_file" || name == "delete_file":
		return "filesystem"
	case name == "git" || strings.HasPrefix(name, "git_"):
		return "version_control"
	case name == "web_fetch" || name == "web_search" || name == "scrape":
		return "external_api"
	case name == "email_send" || name == "social_post" || name == "cms_publish":
		return "external_publish"
	case name == "memory_write" || name == "memory_delete":
		return "memory"
	case name == "delegate" || name == "spawn_agent":
		return "agent_lifecycle"
	case name == "billing" || name == "budget":
		return "finance"
	case strings.HasPrefix(name, "mcp_"):
		return "mcp_plugin"
	default:
		return "general"
	}
}
