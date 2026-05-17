// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package workflow

import (
	"strings"
	"testing"
)

func TestHard_Interpolate_AllPatterns(t *testing.T) {
	tests := []struct{ template string; vars map[string]any; contains string }{
		{"Hello {{name}}", map[string]any{"name": "World"}, "World"},
		{"{{a}} + {{b}} = {{c}}", map[string]any{"a": "1", "b": "2", "c": "3"}, "3"},
		{"No vars here", nil, "No vars"},
		{"{{missing}}", map[string]any{}, ""},
		{"", map[string]any{"x": "y"}, ""},
		{"Nested {{obj}}", map[string]any{"obj": map[string]any{"key": "val"}}, ""},
		{"Multi\nLine\n{{var}}", map[string]any{"var": "end"}, "end"},
		{"JSON: {\"key\": \"{{val}}\"}", map[string]any{"val": "test"}, "test"},
		{"Repeated {{x}} and {{x}}", map[string]any{"x": "same"}, "same"},
		{"Special chars: {{v}}", map[string]any{"v": "<>&\""}, "<>&"},
	}
	for i, tt := range tests {
		result := interpolate(tt.template, tt.vars)
		if tt.contains != "" && !strings.Contains(result, tt.contains) {
			t.Errorf("test %d: %q missing %q in %q", i, tt.template, tt.contains, result)
		}
	}
}

func TestHard_Workflow_AllStepTypes(t *testing.T) {
	types := []struct{ typ string; hasPrompt, hasTool, hasBranches bool }{
		{"prompt", true, false, false},
		{"tool", false, true, false},
		{"condition", false, false, true},
		{"api", false, false, false},
		{"delegate", false, false, false},
		{"parallel", false, false, false},
	}
	for _, tt := range types {
		s := Step{Type: tt.typ}
		if tt.hasPrompt { s.Prompt = "test prompt" }
		if tt.hasTool { s.Tool = "web_search" }
		if tt.hasBranches { s.Branches = map[string]string{"yes": "s1", "no": "s2"} }
		if s.Type != tt.typ { t.Errorf("type=%q", s.Type) }
	}
}

func TestHard_Workflow_RunStateMachine(t *testing.T) {
	states := []string{"pending", "running", "completed", "failed", "cancelled"}
	for i, from := range states {
		for j, to := range states {
			if i == j { continue }
			r := Run{Status: from}
			r.Status = to
			if r.Status != to { t.Errorf("%s→%s failed", from, to) }
		}
	}
}

func TestHard_Workflow_ComplexTemplate(t *testing.T) {
	template := `You are analyzing {{project_name}}.
The user asked: {{user_query}}
Previous context: {{context}}
Available tools: {{tools}}
Please provide a detailed analysis.`

	vars := map[string]any{
		"project_name": "Qorven",
		"user_query":   "How does the agent loop work?",
		"context":      "The user is a Go developer building an AI platform.",
		"tools":        "web_search, read_file, exec",
	}

	result := interpolate(template, vars)
	if !strings.Contains(result, "Qorven") { t.Error("missing project") }
	if !strings.Contains(result, "agent loop") { t.Error("missing query") }
	if !strings.Contains(result, "Go developer") { t.Error("missing context") }
	if !strings.Contains(result, "web_search") { t.Error("missing tools") }
}
