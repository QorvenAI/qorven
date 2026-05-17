// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/qorvenai/qorven/internal/tools"
)

// ConnectorTool wraps a connector action as a tool for the agent loop.
type ConnectorTool struct {
	connector Connector
	action    Action
	credStore CredentialStore
}

// CredentialStore retrieves stored credentials for a connector.
type CredentialStore interface {
	GetCredentials(ctx context.Context, agentID, connectorID string) (map[string]string, error)
}

// NewConnectorTool creates a tool from a connector action.
func NewConnectorTool(c Connector, action Action, credStore CredentialStore) *ConnectorTool {
	return &ConnectorTool{connector: c, action: action, credStore: credStore}
}

func (t *ConnectorTool) Name() string {
	return t.connector.Manifest().ID + "_" + t.action.ID
}

func (t *ConnectorTool) Description() string {
	return fmt.Sprintf("[%s] %s", t.connector.Manifest().Name, t.action.Description)
}

func (t *ConnectorTool) Parameters() map[string]any {
	props := make(map[string]any)
	required := []string{}
	for _, p := range t.action.Parameters {
		props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func (t *ConnectorTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	agentID := tools.AgentIDFromCtx(ctx)

	// Load credentials
	creds, err := t.credStore.GetCredentials(ctx, agentID, t.connector.Manifest().ID)
	if err != nil {
		return tools.ErrorResult("no credentials configured for " + t.connector.Manifest().Name)
	}

	// Execute
	result, err := t.connector.Execute(ctx, t.action.ID, creds, args)
	if err != nil {
		return tools.ErrorResult(err.Error())
	}

	// Format result
	data, _ := json.Marshal(result)
	return tools.TextResult(string(data))
}

// RegisterConnectorTools registers all connector actions as tools.
func RegisterConnectorTools(reg *tools.Registry, connReg *Registry, credStore CredentialStore) {
	for _, manifest := range connReg.List() {
		conn, _ := connReg.Get(manifest.ID)
		for _, action := range manifest.Actions {
			reg.Register(NewConnectorTool(conn, action, credStore))
		}
	}
}
