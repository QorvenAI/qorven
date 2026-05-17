// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ManageAgents lets the agent create/update/delete other agents via chat.
type ManageAgents struct{}

func NewManageAgents() *ManageAgents { return &ManageAgents{} }
func (m *ManageAgents) Name() string { return "manage_agents" }
func (m *ManageAgents) Description() string {
	return "Create, update, or delete agents"
}
func (m *ManageAgents) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":        map[string]any{"type": "string", "description": "create, update, delete, or list"},
		"name":          map[string]any{"type": "string", "description": "agent name (for create)"},
		"id":            map[string]any{"type": "string", "description": "agent ID (for update/delete)"},
		"model":         map[string]any{"type": "string", "description": "LLM model"},
		"role":          map[string]any{"type": "string", "description": "agent role"},
		"system_prompt": map[string]any{"type": "string", "description": "system prompt"},
	}, "required": []string{"action"}}
}

// OnAgentCreate/Update/Delete are callbacks set by gateway.
var OnAgentCreate func(ctx context.Context, name, model, role, prompt string) (string, error)
var OnAgentUpdate func(ctx context.Context, id string, fields map[string]any) error
var OnAgentDelete func(ctx context.Context, id string) error
var OnAgentList func(ctx context.Context) ([]map[string]string, error)

func (m *ManageAgents) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	switch action {
	case "create":
		name, _ := args["name"].(string)
		model, _ := args["model"].(string)
		role, _ := args["role"].(string)
		prompt, _ := args["system_prompt"].(string)
		if name == "" { return ErrorResult("name required") }
		if OnAgentCreate == nil { return ErrorResult("agent creation not available") }
		id, err := OnAgentCreate(ctx, name, model, role, prompt)
		if err != nil { return ErrorResult(err.Error()) }
		return TextResult(fmt.Sprintf("Agent created: %s (id: %s)", name, id))
	case "update":
		id, _ := args["id"].(string)
		if id == "" { return ErrorResult("id required") }
		if OnAgentUpdate == nil { return ErrorResult("agent update not available") }
		fields := map[string]any{}
		for _, k := range []string{"model", "role", "system_prompt"} {
			if v, ok := args[k]; ok { fields[k] = v }
		}
		if err := OnAgentUpdate(ctx, id, fields); err != nil { return ErrorResult(err.Error()) }
		return TextResult("Agent updated")
	case "delete":
		id, _ := args["id"].(string)
		if id == "" { return ErrorResult("id required") }
		if OnAgentDelete == nil { return ErrorResult("agent deletion not available") }
		if err := OnAgentDelete(ctx, id); err != nil { return ErrorResult(err.Error()) }
		return TextResult("Agent deleted")
	case "list":
		if OnAgentList == nil { return ErrorResult("agent listing not available") }
		agents, err := OnAgentList(ctx)
		if err != nil { return ErrorResult(err.Error()) }
		data, _ := json.Marshal(agents)
		return TextResult(string(data))
	default:
		return ErrorResult("actions: create (name, model, role), update (id, fields), delete (id), list")
	}
}
