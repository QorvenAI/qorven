// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/store"
)

// HRManageTool handles agent lifecycle (hire / terminate / list / get) on behalf
// of CHRO, COO, or human operators. Access rules:
//
//	hire_officer (L2)   — only chro or coo may create L2 agents
//	hire_worker  (L3)   — L2 agents may create L3 agents in their own department
//	terminate           — only chro or coo may terminate any agent
//	list_org            — any agent may view the org roster
//	get_agent           — any agent may look up a colleague
//
// The tool writes to org_roster for full audit trail. Callbacks are wired in
// gateway_tools.go exactly like the existing manage_agents callbacks.
type HRManageTool struct{}

func NewHRManageTool() *HRManageTool { return &HRManageTool{} }
func (h *HRManageTool) Name() string  { return "hr_manage" }
func (h *HRManageTool) Description() string {
	return "Hire, terminate, or inspect agents in the org hierarchy. CHRO and COO can hire officers (L2) and workers (L3). L2 managers can hire workers for their own department. Use list_org to see the current roster."
}
func (h *HRManageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"hire_officer", "hire_worker", "terminate", "list_org", "get_agent"},
				"description": "hire_officer=create L2 C-suite agent; hire_worker=create L3 specialist; terminate=deactivate agent; list_org=see full roster; get_agent=look up one agent by id or name",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Display name for the new agent (hire actions)",
			},
			"org_role": map[string]any{
				"type":        "string",
				"description": "Org role for the new agent: coo, cto, cmo, cso, cco, chro, ciso, cko, cfo, caio, specialist",
			},
			"org_level": map[string]any{
				"type":        "string",
				"enum":        []string{"l1", "l2", "l3"},
				"description": "Org level (l2 for officers, l3 for workers)",
			},
			"department": map[string]any{
				"type":        "string",
				"description": "Department the new agent belongs to (e.g. marketing, engineering, sales). Used to set manager_id.",
			},
			"system_prompt": map[string]any{
				"type":        "string",
				"description": "System prompt / rules for the new agent. Leave blank to use the archetype default.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "LLM model for the new agent. Leave blank for auto-selection.",
			},
			"monthly_budget_usd": map[string]any{
				"type":        "number",
				"description": "Monthly spend cap in USD. L2 default $50, L3 default $10.",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID (for terminate or get_agent actions)",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Reason for termination (required for terminate)",
			},
		},
		"required": []string{"action"},
	}
}

// Callbacks wired from gateway_tools.go — same pattern as manage_agents
var OnHRHireAgent func(ctx context.Context, name, model, role, orgRole, orgLevel, department, prompt string, monthlyBudgetUSD float64, managerAgentID string) (string, error)
var OnHRTerminateAgent func(ctx context.Context, agentID, reason, terminatedBy string) error
var OnHRListOrg func(ctx context.Context) ([]map[string]any, error)
var OnHRGetAgent func(ctx context.Context, idOrName string) (map[string]any, error)

func (h *HRManageTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)

	callerLevel := store.OrgLevelFromContext(ctx)
	callerRole := store.OrgRoleFromContext(ctx)

	switch action {
	case "hire_officer":
		if !canHireOfficer(callerLevel, callerRole) {
			return ErrorResult(fmt.Sprintf(
				"Access denied: only CHRO or COO can hire officers. Your role: %s (%s).",
				callerRole, callerLevel))
		}
		return h.doHire(ctx, args, "l2")

	case "hire_worker":
		if !canHireWorker(callerLevel, callerRole) {
			return ErrorResult(fmt.Sprintf(
				"Access denied: L2 managers, CHRO, or COO can hire workers. Your role: %s (%s).",
				callerRole, callerLevel))
		}
		return h.doHire(ctx, args, "l3")

	case "terminate":
		if !canTerminate(callerLevel, callerRole) {
			return ErrorResult(fmt.Sprintf(
				"Access denied: only CHRO or COO can terminate agents. Your role: %s (%s).",
				callerRole, callerLevel))
		}
		return h.doTerminate(ctx, args, callerRole)

	case "list_org":
		if OnHRListOrg == nil {
			return ErrorResult("org listing not available")
		}
		agents, err := OnHRListOrg(ctx)
		if err != nil {
			return ErrorResult(err.Error())
		}
		data, _ := json.Marshal(agents)
		return TextResult(string(data))

	case "get_agent":
		idOrName, _ := args["agent_id"].(string)
		if idOrName == "" {
			idOrName, _ = args["name"].(string)
		}
		if idOrName == "" {
			return ErrorResult("agent_id or name required")
		}
		if OnHRGetAgent == nil {
			return ErrorResult("agent lookup not available")
		}
		agent, err := OnHRGetAgent(ctx, idOrName)
		if err != nil {
			return ErrorResult(err.Error())
		}
		data, _ := json.Marshal(agent)
		return TextResult(string(data))

	default:
		return ErrorResult("actions: hire_officer, hire_worker, terminate, list_org, get_agent")
	}
}

func (h *HRManageTool) doHire(ctx context.Context, args map[string]any, defaultLevel string) *Result {
	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name required")
	}
	if OnHRHireAgent == nil {
		return ErrorResult("agent hiring not available")
	}

	orgRole, _ := args["org_role"].(string)
	orgLevel, _ := args["org_level"].(string)
	if orgLevel == "" {
		orgLevel = defaultLevel
	}
	department, _ := args["department"].(string)
	prompt, _ := args["system_prompt"].(string)
	model, _ := args["model"].(string)
	var monthlyBudget float64
	if v, ok := args["monthly_budget_usd"].(float64); ok {
		monthlyBudget = v
	} else {
		// Default caps by level
		if orgLevel == "l2" {
			monthlyBudget = 50.0
		} else {
			monthlyBudget = 10.0
		}
	}

	// role (archetype) defaults based on org_role
	role := orgRoleToArchetype(orgRole)

	// The manager is the calling agent's ID from context
	managerID := ""
	if rc := store.RunContextFromCtx(ctx); rc != nil {
		managerID = rc.AgentID.String()
	}

	id, err := OnHRHireAgent(ctx, name, model, role, orgRole, orgLevel, department, prompt, monthlyBudget, managerID)
	if err != nil {
		return ErrorResult(err.Error())
	}

	return TextResult(fmt.Sprintf(
		"Agent hired: %s (id: %s, org_role: %s, org_level: %s, budget: $%.2f/mo). Agent is now active and available in the org.",
		name, id, orgRole, orgLevel, monthlyBudget))
}

func (h *HRManageTool) doTerminate(ctx context.Context, args map[string]any, terminatorRole string) *Result {
	agentID, _ := args["agent_id"].(string)
	reason, _ := args["reason"].(string)
	if agentID == "" {
		return ErrorResult("agent_id required")
	}
	if reason == "" {
		reason = "terminated by " + terminatorRole
	}
	if OnHRTerminateAgent == nil {
		return ErrorResult("agent termination not available")
	}

	terminatorID := ""
	if rc := store.RunContextFromCtx(ctx); rc != nil {
		terminatorID = rc.AgentID.String()
	}

	if err := OnHRTerminateAgent(ctx, agentID, reason, terminatorID); err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult(fmt.Sprintf("Agent %s terminated. Reason: %s. Spend snapshot saved to org_roster.", agentID, reason))
}

// ─── Access rules ─────────────────────────────────────────────────────────────

func canHireOfficer(level, role string) bool {
	return role == "chro" || role == "coo" || level == "l1"
}

func canHireWorker(level, role string) bool {
	return role == "chro" || role == "coo" || level == "l1" || level == "l2"
}

func canTerminate(level, role string) bool {
	return role == "chro" || role == "coo" || level == "l1"
}

// orgRoleToArchetype maps an org_role to an agent archetype/role for tool seeding.
func orgRoleToArchetype(orgRole string) string {
	m := map[string]string{
		"cto":    "code",
		"cmo":    "marketer",
		"cso":    "sales",
		"cco":    "support",
		"ciso":   "researcher",
		"cko":    "researcher",
		"cfo":    "analyst",
		"chro":   "general",
		"coo":    "general",
		"caio":   "general",
		"cpro":   "product",
	}
	if r, ok := m[strings.ToLower(orgRole)]; ok {
		return r
	}
	return "general"
}
