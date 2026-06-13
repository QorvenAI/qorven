// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import "testing"

func TestToolToGovernedAction(t *testing.T) {
	cases := map[string]string{
		// write_code tools
		"exec":        "write_code",
		"apply_patch": "write_code",
		"write_file":  "write_code",
		"code_edit":   "write_code",
		"edit":        "write_code",
		"multi_edit":  "write_code",
		// approve_deploy tools
		"gh_create_release": "approve_deploy",
		"gh_open_pr":        "approve_deploy",
		"gh_merge_pr":       "approve_deploy",
		"gh_push_file":      "approve_deploy",
		// budget tools
		"request_budget_raise": "request_budget",
		"propose_allocation":   "request_budget",
		"decide_budget_request": "approve_budget",
		"set_budget":            "approve_budget",
		// agent lifecycle tools
		"spawn":       "create_agent",
		"spawn_team":  "create_agent",
		"manage_agents": "create_agent",
		"hr_manage":   "create_agent",
		// unknown tool
		"unknown_xyz": "",
		"web_search":  "",
		"memory_get":  "",
	}
	for tool, want := range cases {
		if got := ToolToGovernedAction(tool); got != want {
			t.Errorf("ToolToGovernedAction(%q) = %q, want %q", tool, got, want)
		}
	}
}
