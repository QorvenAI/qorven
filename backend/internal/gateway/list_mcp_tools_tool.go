// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/mcp"
	"github.com/qorvenai/qorven/internal/tools"
)

// listMCPToolsTool lets an agent fetch the tool catalogue for an MCP server on demand.
// Symmetric to listConnectorActionsTool — the manifest emits "server (N tools)" and
// the agent calls this tool when it needs to know what tools the server exposes.
type listMCPToolsTool struct {
	mgr      *mcp.Manager
	tenantID string
	agentID  string
}

func newListMCPToolsTool(mgr *mcp.Manager, tenantID, agentID string) *listMCPToolsTool {
	return &listMCPToolsTool{mgr: mgr, tenantID: tenantID, agentID: agentID}
}

func (t *listMCPToolsTool) Name() string { return "list_mcp_tools" }

func (t *listMCPToolsTool) Description() string {
	return "Returns the tools available on an MCP server. " +
		"Call this when you need to know what tools a specific MCP server provides. " +
		"Pass the server name from the MCP Servers list in your context."
}

func (t *listMCPToolsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"server": map[string]any{
				"type":        "string",
				"description": "MCP server name (e.g. 'context7', 'filesystem', 'postgres')",
			},
		},
		"required": []string{"server"},
	}
}

func (t *listMCPToolsTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	serverName, _ := args["server"].(string)
	serverName = strings.TrimSpace(serverName)
	if serverName == "" {
		return tools.ErrorResult("server is required")
	}

	servers, err := t.mgr.List(ctx, t.tenantID)
	if err != nil {
		return tools.ErrorResult("could not list MCP servers: " + err.Error())
	}

	lc := strings.ToLower(serverName)
	for _, s := range servers {
		if !s.Enabled || !s.Installed {
			continue
		}
		if t.agentID != "" && len(s.AssignedAgents) > 0 {
			found := false
			for _, a := range s.AssignedAgents {
				if a == t.agentID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if strings.ToLower(s.Name) != lc {
			continue
		}

		var discovered []mcp.DiscoveredTool
		json.Unmarshal(s.ToolsDiscovered, &discovered)
		if len(discovered) == 0 {
			return &tools.Result{ForLLM: fmt.Sprintf("No tools discovered for MCP server %q.", s.Name)}
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "## %s tools\n\n", s.Name)
		if s.Description != "" {
			fmt.Fprintf(&sb, "%s\n\n", s.Description)
		}
		for _, tool := range discovered {
			name := tool.Name
			if s.ToolPrefix != "" {
				name = s.ToolPrefix + "_" + tool.Name
			}
			fmt.Fprintf(&sb, "### %s\n%s\n\n", name, tool.Description)
		}
		return &tools.Result{ForLLM: sb.String()}
	}

	return tools.ErrorResult(fmt.Sprintf("no enabled MCP server found matching %q", serverName))
}
