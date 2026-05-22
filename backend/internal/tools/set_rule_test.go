// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"
)

func TestSetRuleTool_Name(t *testing.T) {
	tool := &SetRuleTool{}
	if tool.Name() != "set_rule" {
		t.Fatalf("expected name=set_rule, got %q", tool.Name())
	}
}

func TestSetRuleTool_Parameters(t *testing.T) {
	tool := &SetRuleTool{}
	params := tool.Parameters()
	props, _ := params["properties"].(map[string]any)
	required := []string{"description", "trigger_type", "trigger_spec", "action_type", "action_spec"}
	for _, field := range required {
		if _, ok := props[field]; !ok {
			t.Errorf("missing property: %s", field)
		}
	}
}

func TestSetRuleTool_Execute_MissingDB(t *testing.T) {
	tool := &SetRuleTool{} // nil DB
	result := tool.Execute(context.Background(), map[string]any{
		"description":  "test",
		"trigger_type": "cron",
		"trigger_spec": map[string]any{"cron": "0 2 * * 0"},
		"action_type":  "run_tool",
		"action_spec":  map[string]any{"tool": "antivirus_push"},
	})
	if !strings.Contains(result.ForLLM, "unavailable") {
		t.Fatalf("expected 'unavailable' in error result, got: %s", result.ForLLM)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestSetRuleTool_Execute_MissingRequiredFields(t *testing.T) {
	tool := &SetRuleTool{} // nil DB — db check fires first
	result := tool.Execute(context.Background(), map[string]any{
		"trigger_spec": map[string]any{},
		"action_spec":  map[string]any{},
	})
	if !result.IsError {
		t.Fatalf("expected IsError=true, got result: %s", result.ForLLM)
	}
}
