// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/tools"
)

// listConnectorActionsTool lets an agent fetch the full action catalogue for a
// connected service on demand. The agent calls this when the manifest line
// (emitted by BuildKnowledge) is not enough to proceed.
//
// JIT pattern: BuildKnowledge emits ~50 tokens per platform; this tool lets
// the agent pay ~300 tokens for a single platform catalogue only when it
// actually needs to call that platform — instead of paying ~1,430 tokens for
// all platforms on every turn.
type listConnectorActionsTool struct {
	store    *connectors.KnowledgeStore
	tenantID string
}

func newListConnectorActionsTool(store *connectors.KnowledgeStore, tenantID string) *listConnectorActionsTool {
	return &listConnectorActionsTool{store: store, tenantID: tenantID}
}

func (t *listConnectorActionsTool) Name() string { return "list_connector_actions" }

func (t *listConnectorActionsTool) Description() string {
	return "Returns the available actions for a connected service. " +
		"Call this when you need to know what actions a platform supports before calling execute_action. " +
		"Pass the platform name or ID from the Connected Services list."
}

func (t *listConnectorActionsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"platform": map[string]any{
				"type":        "string",
				"description": "Platform name or ID (e.g. 'gmail', 'stripe', 'hubspot')",
			},
		},
		"required": []string{"platform"},
	}
}

func (t *listConnectorActionsTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	platform, _ := args["platform"].(string)
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return tools.ErrorResult("platform is required")
	}

	connected, err := t.store.ListConnected(ctx, t.tenantID)
	if err != nil {
		return tools.ErrorResult("could not load connected services: " + err.Error())
	}

	var match *connectors.Platform
	lc := strings.ToLower(platform)
	for i := range connected {
		p := &connected[i]
		if strings.ToLower(p.ID) == lc || strings.ToLower(p.Name) == lc {
			match = p
			break
		}
	}
	if match == nil {
		names := make([]string, len(connected))
		for i, p := range connected {
			names[i] = p.Name
		}
		return tools.ErrorResult(fmt.Sprintf(
			"no connected service found for %q — available: %s",
			platform, strings.Join(names, ", ")))
	}

	actions, err := t.store.ListActions(ctx, match.ID)
	if err != nil {
		return tools.ErrorResult("could not load actions: " + err.Error())
	}
	if len(actions) == 0 {
		return &tools.Result{ForLLM: fmt.Sprintf("No actions available for %s.", match.Name)}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "## %s actions (%s)\n\n", match.Name, match.Category)
	fmt.Fprintf(&sb, "Call: execute_action(\"%s\", \"<action_key>\", {params})\n\n", match.ID)
	for _, a := range actions {
		fmt.Fprintf(&sb, "### %s\n%s\n", a.ActionKey, a.Description)
		if a.WhenToUse != "" {
			fmt.Fprintf(&sb, "When: %s\n", a.WhenToUse)
		}
		if len(a.Params) > 2 {
			var params map[string]any
			if json.Unmarshal(a.Params, &params) == nil && len(params) > 0 {
				paramsJSON, _ := json.Marshal(params)
				fmt.Fprintf(&sb, "Params: %s\n", string(paramsJSON))
			}
		}
		sb.WriteString("\n")
	}
	return &tools.Result{ForLLM: sb.String()}
}
