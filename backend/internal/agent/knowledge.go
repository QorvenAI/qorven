// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"

	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/mcp"
)

// IntegrationKnowledge builds the full integration knowledge block
// injected into agent system prompts. Includes connected services + MCP tools.
func IntegrationKnowledge(ctx context.Context, tenantID, agentID string, connKB *connectors.KnowledgeStore, mcpMgr *mcp.Manager) string {
	var parts []string

	// Connected external services (Gmail, Slack, HubSpot, etc.)
	if connKB != nil {
		if k, err := connKB.BuildKnowledge(ctx, tenantID); err == nil && k != "" {
			parts = append(parts, k)
		}
	}

	// MCP servers
	if mcpMgr != nil {
		if k, err := mcpMgr.BuildKnowledge(ctx, tenantID, agentID); err == nil && k != "" {
			parts = append(parts, k)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "\n" + strings.Join(parts, "\n")
}
