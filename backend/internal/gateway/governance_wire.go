// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"context"
	"log/slog"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/governance"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/pii"
)

// buildGovernanceHooks creates the callback struct that bridges the governance
// package (policy engine, approval matrix, SoD, exceptions, SLA) into the
// agent loop without creating import cycles.
func (gw *Gateway) buildGovernanceHooks() *agent.GovernanceHooks {
	h := &agent.GovernanceHooks{}

	// Policy Engine: evaluate every triggerable event against loaded policies.
	if gw.policyEngine != nil {
		h.EvaluatePolicy = func(ctx context.Context, tenantID, agentID, triggerEvent string, eventCtx map[string]any) (string, string) {
			decision := gw.policyEngine.Evaluate(ctx, triggerEvent, agentID, 3, eventCtx)
			if decision.Action == "allow" || decision.Action == "log" {
				return "allow", ""
			}
			return decision.Action, decision.PolicyName + ": " + decision.Message
		}
	}

	// Approval Matrix: check if an action requires approval before proceeding.
	if gw.approvalStore != nil {
		h.CheckApproval = func(ctx context.Context, tenantID, actionType string, costUSD float64) (bool, string, string) {
			rule, err := gw.approvalStore.CheckRequiresApproval(ctx, tenantID, actionType, costUSD)
			if err != nil || rule == nil {
				return false, "", ""
			}
			return true, rule.ActionType, rule.ApproverRole
		}
	}

	// Segregation of Duties: prevent same agent from doing conflicting actions.
	if gw.sodStore != nil {
		h.CheckSoD = func(ctx context.Context, tenantID, agentID, action string) (bool, string) {
			return gw.sodStore.CheckViolation(ctx, tenantID, agentID, action)
		}
		h.ResolveGovernedAction = governance.ToolToGovernedAction
		h.RecordGovernedAction = func(ctx context.Context, tenantID, agentID, action string) {
			gw.sodStore.RecordAction(ctx, tenantID, agentID, action)
		}
	}

	// Exception Recording: log governance variances and failures.
	if gw.exceptionStore != nil {
		h.RecordException = func(ctx context.Context, tenantID, agentID, exType, severity, description string, exCtx map[string]any) {
			err := gw.exceptionStore.Record(ctx, governance.Exception{
				TenantID:    tenantID,
				AgentID:     agentID,
				Category:    exType,
				Severity:    severity,
				Description: description,
				Context:     exCtx,
			})
			if err != nil {
				slog.Warn("governance.exception.record_error", "error", err)
			}
		}
	}

	// Task State Transition: record in governance audit trail.
	if gw.taskStateMachine != nil {
		h.RecordTaskTransition = func(ctx context.Context, tenantID, taskID, fromState, toState, changedBy, reason string) {
			if err := gw.taskStateMachine.Transition(ctx, tenantID, taskID, fromState, toState, changedBy, reason); err != nil {
				slog.Warn("governance.task_transition.error", "error", err, "task", taskID)
			}
		}
	}

	// SLA Measurement: record data points.
	if gw.slaStore != nil {
		h.RecordSLAEvent = func(ctx context.Context, tenantID, slaID string, value float64, met bool) {
			if err := gw.slaStore.RecordMeasurement(ctx, tenantID, slaID, value, met); err != nil {
				slog.Warn("governance.sla.record_error", "error", err)
			}
		}
	}

	// Designation Lookup: resolve agent designation metadata.
	if gw.designationStore != nil {
		h.LookupDesignation = func(ctx context.Context, tenantID, agentKey string) (string, string, bool, []string) {
			d, err := gw.designationStore.GetByAgentKey(ctx, tenantID, agentKey)
			if err != nil || d == nil {
				return "", "", false, nil
			}
			return d.ModelTier, d.SkillFamily, d.CanCreateSubagents, d.ApprovalScope
		}
	}

	// PII Detection: run the real PII engine against agent output so the
	// "Block PII in outputs" policy can match on has_pii == "true".
	// Scans only high-sensitivity kinds (SSN, credit card, IBAN, phone) to
	// avoid false-positive triggers on IPs and example email addresses that
	// commonly appear in technical output.
	h.DetectPII = func(content string) bool {
		return len(pii.Scan(content, pii.Config{
			Kinds: pii.KindSSN | pii.KindCreditCard | pii.KindIBAN | pii.KindPhone,
		})) > 0
	}

	// HasBlockingOutputPolicy: check in-memory policies — no DB round-trip.
	if gw.policyEngine != nil {
		h.HasBlockingOutputPolicy = func(tenantID string) bool {
			return gw.policyEngine.HasBlockingOutputPolicy(tenantID)
		}
	}

	// RequestApproval: block the agent on a governance hold, writing an
	// approval_requests row so the inbox shows it, and unblocking when the
	// inbox's decide endpoint calls Gate.Reply. The gate_request_id stored in
	// the context column is what links the two rows.
	if gw.approvalStore != nil && gw.permissionGate != nil {
		h.RequestApproval = func(ctx context.Context, tenantID, agentID, agentKey, actionType, reason string, args any) bool {
			id, wait, err := gw.permissionGate.NewPending(ctx, permissions.RequestInput{
				AgentKey: agentKey,
				Tool:     actionType,
				Args:     args,
				Reason:   reason,
				TenantID: tenantID,
				AgentID:  agentID,
			})
			if err != nil {
				slog.Warn("governance.request_approval.gate_error", "error", err)
				return false
			}
			if err := gw.approvalStore.CreateRequest(ctx, governance.ApprovalRequest{
				TenantID:     tenantID,
				ActionType:   actionType,
				RequestorID:  agentID,
				RequestorKey: agentKey,
				ApproverRole: "admin",
				Status:       "pending",
				Context:      map[string]any{"gate_request_id": id, "reason": reason},
			}); err != nil {
				slog.Warn("governance.approval.create_error", "error", err)
			}
			v := wait() // blocks until Gate.Reply fires or timeout
			return v.Allowed()
		}
	}

	return h
}
