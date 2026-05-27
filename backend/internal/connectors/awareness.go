// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/tools"
)

// ConnectedPlatformInfo holds summary data about one connected platform.
type ConnectedPlatformInfo struct {
	PlatformID  string   `json:"platform_id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	AccountName string   `json:"account_name"`
	Healthy     bool     `json:"healthy"`
	Actions     []string `json:"actions"`
}

// PlatformLister provides a list of connected platforms for agent awareness.
type PlatformLister interface {
	ListConnectedPlatforms(ctx context.Context, tenantID string) ([]ConnectedPlatformInfo, error)
}

// PlatformAwareness implements PlatformLister by combining RelayStore and KnowledgeStore.
type PlatformAwareness struct {
	relay     *RelayStore
	knowledge *KnowledgeStore
}

func NewPlatformAwareness(relay *RelayStore, knowledge *KnowledgeStore) *PlatformAwareness {
	return &PlatformAwareness{relay: relay, knowledge: knowledge}
}

func (pa *PlatformAwareness) ListConnectedPlatforms(ctx context.Context, tenantID string) ([]ConnectedPlatformInfo, error) {
	accounts, err := pa.relay.ListAccounts(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	var results []ConnectedPlatformInfo
	for _, acc := range accounts {
		info := ConnectedPlatformInfo{
			PlatformID:  acc.PlatformID,
			AccountName: acc.DisplayName,
			Healthy:     acc.Healthy,
		}

		if p, err := pa.knowledge.GetPlatform(ctx, acc.PlatformID); err == nil && p != nil {
			info.Name = p.Name
			info.Category = p.Category
		} else {
			info.Name = acc.PlatformID
			info.Category = "integration"
		}

		if actions, err := pa.knowledge.ListActions(ctx, acc.PlatformID); err == nil {
			for _, a := range actions {
				info.Actions = append(info.Actions, a.ActionKey)
			}
		}

		results = append(results, info)
	}
	return results, nil
}

// ConnectedPlatformsTool provides agent awareness of connected integration platforms.
type ConnectedPlatformsTool struct {
	lister PlatformLister
}

func NewConnectedPlatformsTool(lister PlatformLister) *ConnectedPlatformsTool {
	return &ConnectedPlatformsTool{lister: lister}
}

func (t *ConnectedPlatformsTool) Name() string { return "list_connected_platforms" }

func (t *ConnectedPlatformsTool) Description() string {
	return "List all connected integration platforms and their available actions. Use this to discover what services the user has connected and what you can do with them via execute_action."
}

func (t *ConnectedPlatformsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ConnectedPlatformsTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	tenantID := tools.TenantIDFromCtx(ctx)
	if tenantID == "" {
		return tools.ErrorResult("no tenant context")
	}

	platforms, err := t.lister.ListConnectedPlatforms(ctx, tenantID)
	if err != nil {
		return tools.ErrorResult("failed to list platforms: " + err.Error())
	}

	if len(platforms) == 0 {
		return tools.TextResult("No integration platforms are currently connected. The user can connect platforms in Settings > Integrations.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Connected platforms (%d):\n\n", len(platforms)))
	for _, p := range platforms {
		status := "healthy"
		if !p.Healthy {
			status = "unhealthy"
		}
		sb.WriteString(fmt.Sprintf("- %s (%s) [%s] — %s\n", p.Name, p.PlatformID, p.Category, status))
		if p.AccountName != "" {
			sb.WriteString(fmt.Sprintf("  Account: %s\n", p.AccountName))
		}
		if len(p.Actions) > 0 {
			sb.WriteString(fmt.Sprintf("  Actions: %s\n", strings.Join(p.Actions, ", ")))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("Use execute_action(platform, action, params) to run any of these actions.")
	return tools.TextResult(sb.String())
}
