// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package templates

type WorkspaceTemplate struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Icon        string        `json:"icon"`
	Category    string        `json:"category"`
	Version     string        `json:"version"`
	Author      string        `json:"author,omitempty"`
	Agents      []AgentSpec   `json:"agents"`
	Dashboard   DashboardSpec `json:"dashboard"`
	Skills      []string      `json:"skills"`
	Connectors  []string      `json:"connectors"`
	ExportURL   string        `json:"export_url,omitempty"`
}

type AgentSpec struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	ReportsTo    string   `json:"reports_to,omitempty"`
	Model        string   `json:"model,omitempty"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools,omitempty"`
}

type DashboardSpec struct {
	Layout string      `json:"layout"`
	Blocks []BlockSpec `json:"blocks"`
}

type BlockSpec struct {
	Type  string         `json:"type"`
	Title string         `json:"title,omitempty"`
	Props map[string]any `json:"props,omitempty"`
}
