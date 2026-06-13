// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

// ToolToGovernedAction maps a concrete tool name to the governance "action"
// vocabulary used by separation-of-duties rules. Returns "" when the tool is
// not governed by any SoD rule (so no SoD check applies).
func ToolToGovernedAction(tool string) string {
	switch tool {
	// write_code: any tool that produces or modifies code/files
	case "exec", "apply_patch", "write_file", "code_edit", "edit", "multi_edit":
		return "write_code"

	// approve_deploy: releasing or publishing to production
	case "gh_create_release", "gh_open_pr", "gh_merge_pr", "gh_push_file":
		return "approve_deploy"

	// request_budget: asking for more budget
	case "request_budget_raise", "propose_allocation":
		return "request_budget"

	// approve_budget: granting or deciding on budget requests
	case "decide_budget_request", "set_budget":
		return "approve_budget"

	// create_agent: spinning up new agents
	case "spawn", "spawn_team", "manage_agents", "hr_manage":
		return "create_agent"

	// approve_agent: approving agent creation (no dedicated tool; approval flow uses hr_manage)
	case "approve_agent":
		return "approve_agent"

	// write_policy: creating or modifying governance rules
	case "set_rule", "skill_manage":
		return "write_policy"

	// evaluate_policy: checking policies (no dedicated tool today)
	case "evaluate_policy":
		return "evaluate_policy"
	}
	return ""
}
