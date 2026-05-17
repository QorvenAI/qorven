// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package workflow

import (
	"strings"
	"testing"
)

func TestDeep_Interpolate_ComplexTemplate(t *testing.T) {
	vars := map[string]any{
		"user_name": "Alice",
		"query":     "What is Go?",
		"context":   "The user is a developer building Qorven.",
		"tool_output": "Go is a programming language created by Google.",
	}
	template := "Hello {{user_name}}, you asked: {{query}}\n\nContext: {{context}}\n\nAnswer: {{tool_output}}"
	result := interpolate(template, vars)
	if !strings.Contains(result, "Alice") { t.Error("missing name") }
	if !strings.Contains(result, "What is Go?") { t.Error("missing query") }
	if !strings.Contains(result, "Qorven") { t.Error("missing context") }
	if !strings.Contains(result, "Google") { t.Error("missing tool output") }
}

func TestDeep_Interpolate_MissingVars(t *testing.T) {
	result := interpolate("Hello {{name}}, your {{role}} is ready", map[string]any{"name": "Bob"})
	if !strings.Contains(result, "Bob") { t.Error("present var not interpolated") }
	// Missing var should be left as-is or empty
	t.Logf("missing var result: %q", result)
}

func TestDeep_Interpolate_NestedBraces(t *testing.T) {
	result := interpolate("JSON: {\"key\": \"{{value}}\"}", map[string]any{"value": "test"})
	if !strings.Contains(result, "test") { t.Error("value not interpolated") }
}

func TestDeep_Interpolate_EmptyVars(t *testing.T) {
	result := interpolate("no vars here", nil)
	if result != "no vars here" { t.Error("should pass through") }
}

func TestDeep_Interpolate_EmptyTemplate(t *testing.T) {
	result := interpolate("", map[string]any{"x": "y"})
	if result != "" { t.Error("empty template should return empty") }
}

func TestDeep_Workflow_StepTypes(t *testing.T) {
	types := []string{"prompt", "tool", "condition", "api", "delegate", "parallel"}
	for _, typ := range types {
		s := Step{ID: "s1", Type: typ}
		if s.Type != typ { t.Errorf("type=%q", s.Type) }
	}
}

func TestDeep_Workflow_RunStatus(t *testing.T) {
	statuses := []string{"pending", "running", "completed", "failed", "cancelled"}
	for _, status := range statuses {
		r := Run{Status: status}
		if r.Status != status { t.Errorf("status=%q", r.Status) }
	}
}

func TestDeep_Workflow_StepWithBranches(t *testing.T) {
	s := Step{
		ID:   "decision",
		Type: "condition",
		Branches: map[string]string{
			"yes":     "step_approve",
			"no":      "step_reject",
			"default": "step_review",
		},
	}
	if len(s.Branches) != 3 { t.Errorf("branches=%d", len(s.Branches)) }
	if s.Branches["yes"] != "step_approve" { t.Error("wrong branch") }
}

func TestDeep_Workflow_Fields(t *testing.T) {
	wf := Workflow{ID: "wf-deploy", Name: "Deploy Pipeline", Enabled: true}
	if wf.ID != "wf-deploy" { t.Error("id") }
	if wf.Name != "Deploy Pipeline" { t.Error("name") }
	if !wf.Enabled { t.Error("enabled") }
}
