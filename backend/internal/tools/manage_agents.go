// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
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
		"model":         map[string]any{"type": "string", "description": "explicit LLM model ID (optional; if omitted, model is chosen from budget_cents and role)"},
		"role":          map[string]any{"type": "string", "description": "agent role — one of: chief, code, architect, reviewer, devops, qa, researcher, analyst, designer, product, writer, marketer, sales, support, legal, social, general, worker"},
		"system_prompt": map[string]any{"type": "string", "description": "custom system prompt (optional; if omitted, archetype soul is used)"},
		"budget_cents":  map[string]any{"type": "number", "description": "per-agent spend cap in cents; auto-selects model tier when model is not specified (e.g. 500=simple/cheap, 2000=standard, 6000=complex/coding)"},
	}, "required": []string{"action"}}
}

// OnAgentCreate/Update/Delete are callbacks set by gateway.
var OnAgentCreate func(ctx context.Context, name, model, role, prompt string) (string, error)
var OnAgentUpdate func(ctx context.Context, id string, fields map[string]any) error
var OnAgentDelete func(ctx context.Context, id string) error
var OnAgentList func(ctx context.Context) ([]map[string]string, error)

// OnModelForTier resolves a model ID for a given tier string.
// Wired by gateway to SmartRouter.BestModelForTier.
var OnModelForTier func(tier string) string

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
		// Auto-select model from budget_cents when no explicit model given.
		if model == "" && OnModelForTier != nil {
			if rawBudget, ok := args["budget_cents"]; ok {
				budgetCents := int64(manageToFloat64(rawBudget))
				if budgetCents > 0 {
					tier := spawnTierForBudget(budgetCents, role)
					model = OnModelForTier(tier)
				}
			}
		}
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

func manageToFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}
