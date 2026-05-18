// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/json"
	"testing"
)

func TestResolveRole(t *testing.T) {
	tests := []struct {
		key   string
		found bool
	}{
		{"prime", true},
		{"code", true},
		{"researcher", true},
		{"support", true},
		{"general", true},
		{"unknown_agent_xyz", false},
	}
	for _, tc := range tests {
		_, ok := ResolveRole(tc.key)
		if ok != tc.found {
			t.Errorf("ResolveRole(%q): got found=%v, want %v", tc.key, ok, tc.found)
		}
	}
}

func TestApplyRole_Prime(t *testing.T) {
	ag := &Agent{
		AgentKey:          "prime",
		MaxToolIterations: 20,
	}
	role, _ := ResolveRole("prime")
	mode := ApplyRole(ag, role)

	if mode != PromptIntake {
		t.Errorf("expected PromptIntake, got %q", mode)
	}
	if ag.MaxToolIterations != 5 {
		t.Errorf("expected MaxToolIterations=5, got %d", ag.MaxToolIterations)
	}

	var allowed []string
	if err := json.Unmarshal(ag.ToolsAllowed, &allowed); err != nil {
		t.Fatalf("tools_allowed not valid JSON: %v", err)
	}
	if len(allowed) != 2 {
		t.Errorf("expected 2 allowed tools, got %d: %v", len(allowed), allowed)
	}
	wantTools := map[string]bool{"ask_followup_question": true, "produce_project_brief": true}
	for _, tool := range allowed {
		if !wantTools[tool] {
			t.Errorf("unexpected tool in allowed list: %q", tool)
		}
	}
	// Deny list must be nil when allowlist is set (allowlist is the complete surface).
	if ag.ToolsDenied != nil {
		t.Errorf("expected ToolsDenied=nil when allowlist is set, got %s", ag.ToolsDenied)
	}
}

func TestApplyRole_CodeSpecialist(t *testing.T) {
	ag := &Agent{
		AgentKey:          "code",
		MaxToolIterations: 15,
	}
	role, _ := ResolveRole("code")
	mode := ApplyRole(ag, role)

	// code role does not force a PromptMode
	if mode != "" {
		t.Errorf("expected empty PromptMode for code role, got %q", mode)
	}
	// MaxIterations: role caps at 25, but ag had 15 so ag keeps 15 (role only caps downward)
	if ag.MaxToolIterations != 15 {
		t.Errorf("expected MaxToolIterations=15 (ag value kept), got %d", ag.MaxToolIterations)
	}
	// ToolsAllowed should be nil (code role uses deny-list only)
	if ag.ToolsAllowed != nil {
		t.Errorf("expected ToolsAllowed=nil for deny-list role, got %s", ag.ToolsAllowed)
	}
	var denied []string
	if err := json.Unmarshal(ag.ToolsDenied, &denied); err != nil {
		t.Fatalf("tools_denied not valid JSON: %v", err)
	}
	if len(denied) == 0 {
		t.Error("expected non-empty deny list for code role")
	}
}

func TestApplyRole_MaxIterCapOnlyDownward(t *testing.T) {
	ag := &Agent{
		AgentKey:          "prime",
		MaxToolIterations: 3, // already lower than role's 5
	}
	role, _ := ResolveRole("prime")
	ApplyRole(ag, role)

	// Role should not raise the cap above the agent's own setting
	if ag.MaxToolIterations != 3 {
		t.Errorf("expected MaxToolIterations=3 (agent lower than role), got %d", ag.MaxToolIterations)
	}
}

func TestApplyRole_UnknownKey(t *testing.T) {
	ag := &Agent{AgentKey: "unknown_xyz", MaxToolIterations: 10}
	if _, ok := ResolveRole(ag.AgentKey); ok {
		t.Fatal("expected no role for unknown key")
	}
	// Agent should be untouched
	if ag.MaxToolIterations != 10 {
		t.Errorf("unexpected mutation of agent without a role")
	}
}
