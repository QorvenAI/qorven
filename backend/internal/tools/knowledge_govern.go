// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qorvenai/qorven/internal/store"
)

// KnowledgeGovernTool allows CKO to manage agent clearances, grants, and audit KB access.
type KnowledgeGovernTool struct{}

func NewKnowledgeGovernTool() *KnowledgeGovernTool { return &KnowledgeGovernTool{} }
func (k *KnowledgeGovernTool) Name() string        { return "knowledge_govern" }
func (k *KnowledgeGovernTool) Description() string {
	return "Manage knowledge governance: set agent clearances, grant/revoke KB access, audit access logs, check onboarding status. CKO-only tool."
}
func (k *KnowledgeGovernTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					"set_clearance", "get_clearance", "grant_access", "revoke_grant",
					"list_grants", "audit_access", "onboarding_status",
				},
				"description": "set_clearance=change agent's max classification; get_clearance=view current level; grant_access=create KB grant; revoke_grant=revoke a grant by ID; list_grants=all active grants; audit_access=recent access log; onboarding_status=check agent onboarding stage",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Target agent ID",
			},
			"clearance_level": map[string]any{
				"type":        "string",
				"enum":        []string{"public", "internal", "confidential", "restricted"},
				"description": "New clearance level (for set_clearance)",
			},
			"grant_to": map[string]any{
				"type":        "string",
				"description": "Agent ID receiving the grant (for grant_access)",
			},
			"scope": map[string]any{
				"type":        "string",
				"description": "Scope of the grant: company, team, agent (for grant_access)",
			},
			"grant_id": map[string]any{
				"type":        "string",
				"description": "Grant ID to revoke (for revoke_grant)",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Reason for the action (audit trail)",
			},
		},
		"required": []string{"action"},
	}
}

// Callbacks wired from gateway
var OnKGSetClearance func(ctx context.Context, agentID string, level int, reason, setBy string) error
var OnKGGetClearance func(ctx context.Context, agentID string) (map[string]any, error)
var OnKGGrantAccess func(ctx context.Context, grantorID, granteeID, scope string, maxClass int, purpose string) (string, error)
var OnKGRevokeGrant func(ctx context.Context, grantID, revokedBy string) error
var OnKGListGrants func(ctx context.Context) ([]map[string]any, error)
var OnKGAuditAccess func(ctx context.Context, agentID string, limit int) ([]map[string]any, error)
var OnKGOnboardingStatus func(ctx context.Context, agentID string) (map[string]any, error)

func (k *KnowledgeGovernTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)

	callerRole := store.OrgRoleFromContext(ctx)
	callerLevel := store.OrgLevelFromContext(ctx)

	// Only CKO, COO, or L1 can use this tool
	if callerRole != "cko" && callerRole != "coo" && callerLevel != "l1" {
		return ErrorResult(fmt.Sprintf(
			"Access denied: only CKO, COO, or CEO can use knowledge_govern. Your role: %s (%s).",
			callerRole, callerLevel))
	}

	switch action {
	case "set_clearance":
		return k.setClearance(ctx, args)
	case "get_clearance":
		return k.getClearance(ctx, args)
	case "grant_access":
		return k.grantAccess(ctx, args)
	case "revoke_grant":
		return k.revokeGrant(ctx, args)
	case "list_grants":
		return k.listGrants(ctx)
	case "audit_access":
		return k.auditAccess(ctx, args)
	case "onboarding_status":
		return k.onboardingStatus(ctx, args)
	default:
		return ErrorResult("actions: set_clearance, get_clearance, grant_access, revoke_grant, list_grants, audit_access, onboarding_status")
	}
}

func (k *KnowledgeGovernTool) setClearance(ctx context.Context, args map[string]any) *Result {
	agentID, _ := args["agent_id"].(string)
	levelStr, _ := args["clearance_level"].(string)
	reason, _ := args["reason"].(string)
	if agentID == "" || levelStr == "" {
		return ErrorResult("agent_id and clearance_level required")
	}
	if OnKGSetClearance == nil {
		return ErrorResult("clearance management not available")
	}

	levelMap := map[string]int{"public": 0, "internal": 1, "confidential": 2, "restricted": 3}
	level, ok := levelMap[levelStr]
	if !ok {
		return ErrorResult("invalid clearance_level: use public, internal, confidential, or restricted")
	}

	callerID := ""
	if rc := store.RunContextFromCtx(ctx); rc != nil {
		callerID = rc.AgentID.String()
	}

	if err := OnKGSetClearance(ctx, agentID, level, reason, callerID); err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult(fmt.Sprintf("Clearance for agent %s set to %s. Reason: %s", agentID, levelStr, reason))
}

func (k *KnowledgeGovernTool) getClearance(ctx context.Context, args map[string]any) *Result {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return ErrorResult("agent_id required")
	}
	if OnKGGetClearance == nil {
		return ErrorResult("clearance lookup not available")
	}
	result, err := OnKGGetClearance(ctx, agentID)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.Marshal(result)
	return TextResult(string(data))
}

func (k *KnowledgeGovernTool) grantAccess(ctx context.Context, args map[string]any) *Result {
	agentID, _ := args["agent_id"].(string)
	grantTo, _ := args["grant_to"].(string)
	scope, _ := args["scope"].(string)
	reason, _ := args["reason"].(string)
	if agentID == "" || grantTo == "" || scope == "" {
		return ErrorResult("agent_id (grantor), grant_to (grantee), and scope required")
	}
	if OnKGGrantAccess == nil {
		return ErrorResult("grant management not available")
	}

	// Use grantor's clearance as max for the grant
	var maxClass int
	if info, err := OnKGGetClearance(ctx, grantTo); err == nil {
		if c, ok := info["max_classification"].(int); ok {
			maxClass = c
		} else {
			maxClass = 1
		}
	} else {
		maxClass = 1
	}

	grantID, err := OnKGGrantAccess(ctx, agentID, grantTo, scope, maxClass, reason)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult(fmt.Sprintf("Grant created: %s. Agent %s now has %s access to %s's knowledge (scope: %s).",
		grantID, grantTo, clearanceName(maxClass), agentID, scope))
}

func (k *KnowledgeGovernTool) revokeGrant(ctx context.Context, args map[string]any) *Result {
	grantID, _ := args["grant_id"].(string)
	if grantID == "" {
		return ErrorResult("grant_id required")
	}
	if OnKGRevokeGrant == nil {
		return ErrorResult("revocation not available")
	}
	callerID := ""
	if rc := store.RunContextFromCtx(ctx); rc != nil {
		callerID = rc.AgentID.String()
	}
	if err := OnKGRevokeGrant(ctx, grantID, callerID); err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult(fmt.Sprintf("Grant %s revoked.", grantID))
}

func (k *KnowledgeGovernTool) listGrants(ctx context.Context) *Result {
	if OnKGListGrants == nil {
		return ErrorResult("grant listing not available")
	}
	grants, err := OnKGListGrants(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.Marshal(grants)
	return TextResult(string(data))
}

func (k *KnowledgeGovernTool) auditAccess(ctx context.Context, args map[string]any) *Result {
	agentID, _ := args["agent_id"].(string)
	if OnKGAuditAccess == nil {
		return ErrorResult("audit log not available")
	}
	logs, err := OnKGAuditAccess(ctx, agentID, 50)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.Marshal(logs)
	return TextResult(string(data))
}

func (k *KnowledgeGovernTool) onboardingStatus(ctx context.Context, args map[string]any) *Result {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		return ErrorResult("agent_id required")
	}
	if OnKGOnboardingStatus == nil {
		return ErrorResult("onboarding status not available")
	}
	status, err := OnKGOnboardingStatus(ctx, agentID)
	if err != nil {
		return ErrorResult(err.Error())
	}
	data, _ := json.Marshal(status)
	return TextResult(string(data))
}

func clearanceName(level int) string {
	names := []string{"public", "internal", "confidential", "restricted"}
	if level >= 0 && level < len(names) {
		return names[level]
	}
	return "internal"
}
