// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"fmt"

	"github.com/qorvenai/qorven/internal/tools"
)

// ExecuteActionTool wraps the Executor as a tools.Tool for agent use.
type ExecuteActionTool struct {
	executor *Executor
}

func NewExecuteActionTool(exec *Executor) *ExecuteActionTool {
	return &ExecuteActionTool{executor: exec}
}

func (t *ExecuteActionTool) Name() string { return "execute_action" }

func (t *ExecuteActionTool) Description() string {
	return "Execute an action on a connected external service (Gmail, Slack, Sheets, HubSpot, etc.). Use this when you need to interact with an external app the user has connected."
}

func (t *ExecuteActionTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"platform": map[string]any{"type": "string", "description": "Platform ID (e.g. gmail, slack, hubspot, google-sheets)"},
			"action":   map[string]any{"type": "string", "description": "Action key (e.g. send_email, send_message, read_range)"},
			"params":   map[string]any{"type": "object", "description": "Action parameters as key-value pairs"},
		},
		"required": []string{"platform", "action", "params"},
	}
}

func (t *ExecuteActionTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	platformID, _ := args["platform"].(string)
	actionKey, _ := args["action"].(string)
	params, _ := args["params"].(map[string]any)

	if platformID == "" || actionKey == "" {
		return tools.ErrorResult("platform and action are required")
	}

	result, err := t.executor.Execute(ctx, platformID, actionKey, params)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("Error: %v", err))
	}
	return tools.TextResult(fmt.Sprintf("%s.%s completed.\n\n%s", platformID, actionKey, result))
}
