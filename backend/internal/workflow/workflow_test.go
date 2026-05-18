// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package workflow

import (
	"testing"
)

func TestInterpolate_NoVars(t *testing.T) {
	result := interpolate("hello world", nil)
	if result != "hello world" { t.Errorf("got %q", result) }
}

func TestInterpolate_SimpleVar(t *testing.T) {
	result := interpolate("Hello {{name}}", map[string]any{"name": "World"})
	if result != "Hello World" { t.Errorf("got %q", result) }
}

func TestInterpolate_MultipleVars(t *testing.T) {
	vars := map[string]any{"first": "John", "last": "Doe"}
	result := interpolate("{{first}} {{last}}", vars)
	if result != "John Doe" { t.Errorf("got %q", result) }
}

func TestInterpolate_MissingVar(t *testing.T) {
	result := interpolate("Hello {{name}}", map[string]any{})
	// Missing vars should be left as-is or replaced with empty
	if result == "" { t.Error("should not be empty") }
}

func TestInterpolate_NilVars(t *testing.T) {
	result := interpolate("Hello {{name}}", nil)
	if result == "" { t.Error("should not be empty") }
}

func TestWorkflow_Fields(t *testing.T) {
	wf := Workflow{ID: "wf1", Name: "Test Workflow"}
	if wf.ID != "wf1" { t.Error("wrong id") }
	if wf.Name != "Test Workflow" { t.Error("wrong name") }
}

func TestStep_Fields(t *testing.T) {
	s := Step{ID: "s1", Type: "prompt", Prompt: "Ask the LLM"}
	if s.Type != "prompt" { t.Error("wrong type") }
}

func TestStep_Types(t *testing.T) {
	types := []string{"prompt", "tool", "condition", "api", "delegate", "parallel"}
	for _, typ := range types {
		s := Step{Type: typ}
		if s.Type == "" { t.Error("empty type") }
	}
}

func TestRun_Fields(t *testing.T) {
	r := Run{ID: "r1", WorkflowID: "wf1", Status: "running"}
	if r.Status != "running" { t.Error("wrong status") }
}
