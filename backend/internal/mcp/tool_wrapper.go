// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mcp

import (
	"context"

	"github.com/qorvenai/qorven/internal/tools"
)

// MCPTool wraps a discovered MCP tool as a Qorven tool.
type MCPTool struct {
	client     *Client
	serverName string
	toolName   string
	desc       string
	schema     map[string]any
}

func (t *MCPTool) Name() string        { return t.toolName }
func (t *MCPTool) Description() string { return t.desc }
func (t *MCPTool) Parameters() map[string]any {
	if t.schema != nil { return t.schema }
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *MCPTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	result, err := t.client.CallToolAny(ctx, t.serverName, t.toolName, args)
	if err != nil { return tools.ErrorResult("MCP tool error: " + err.Error()) }
	return tools.TextResult(result)
}

// RegisterDiscoveredTools registers all MCP tools into the Qorven tool registry.
func RegisterDiscoveredTools(client *Client, registry *tools.Registry) int {
	allTools := client.GetAllTools()
	count := 0
	for _, dt := range allTools {
		registry.Register(&MCPTool{
			client:     client,
			serverName: dt.ServerName,
			toolName:   dt.Name,
			desc:       dt.Description + " (MCP: " + dt.ServerName + ")",
			schema:     dt.InputSchema,
		})
		count++
	}
	return count
}
