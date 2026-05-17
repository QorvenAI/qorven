// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WorkspaceBuilderTool lets Prime build, modify, and manage workspaces through conversation.
// This is the tool that makes "tell Prime to build you a CRM" actually work end-to-end.
//
// Usage in conversation:
//   User: "Build me a CRM workspace"
//   Prime: calls workspace_builder(action="build", description="CRM workspace for sales team")
//   Prime: "✅ CRM workspace installed! 4 agents created. View at /dashboard/crm"
type WorkspaceBuilderTool struct {
	apiBase  string
	getToken func() string
}

func NewWorkspaceBuilderTool(apiBase string, getToken func() string) *WorkspaceBuilderTool {
	if apiBase == "" {
		apiBase = "http://localhost:4200"
	}
	return &WorkspaceBuilderTool{apiBase: apiBase, getToken: getToken}
}

func (t *WorkspaceBuilderTool) Name() string { return "workspace_builder" }

func (t *WorkspaceBuilderTool) Description() string {
	return `Build and manage Qorven workspaces for users. Creates complete AI teams with dashboards.

Use this when a user asks to:
- "Build me a CRM / analytics / HR / marketing / support workspace"
- "Create a team for [purpose]"
- "Set up a dashboard for [purpose]"
- "Add an agent that does [task]"
- "What workspaces do we have?"

Actions: build, list, add_agent, dashboard, status`
}

func (t *WorkspaceBuilderTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"build", "list", "add_agent", "dashboard", "status"},
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Natural language description of what to build",
			},
			"template_id": map[string]any{
				"type":        "string",
				"description": "Force a specific template: crm, social, hr, support, devops, ecommerce, legal, content, freelance, education, analytics, research, trading, invoicing",
			},
			"agent": map[string]any{
				"type":        "object",
				"description": "For add_agent: {key, name, role, system_prompt, tools:[]}",
			},
			"dashboard_config": map[string]any{
				"type":        "object",
				"description": "For dashboard action: {layout, title, blocks:[{type,title,...}]}",
			},
			"workspace_id": map[string]any{
				"type":        "string",
				"description": "Target workspace ID for add_agent or dashboard actions",
			},
		},
		"required": []string{"action"},
	}
}

func (t *WorkspaceBuilderTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	switch action {
	case "build":
		return t.build(ctx, args)
	case "list":
		return t.list(ctx)
	case "add_agent":
		return t.addAgent(ctx, args)
	case "dashboard":
		return t.saveDashboard(ctx, args)
	case "status":
		return t.status(ctx)
	default:
		return ErrorResult("unknown action: " + action)
	}
}

func (t *WorkspaceBuilderTool) build(ctx context.Context, args map[string]any) *Result {
	desc, _ := args["description"].(string)
	tmplID, _ := args["template_id"].(string)

	if desc == "" && tmplID == "" {
		return ErrorResult("description or template_id required")
	}
	if desc == "" {
		desc = "Install the " + tmplID + " template"
	}

	body, _ := json.Marshal(map[string]any{"description": desc, "template_id": tmplID})
	resp, err := t.post(ctx, "/v1/templates/self-build", body)
	if err != nil {
		return ErrorResult("build failed: " + err.Error())
	}

	var r map[string]any
	json.Unmarshal(resp, &r)

	name, _ := r["name"].(string)
	count, _ := r["agent_count"].(float64)
	dashURL, _ := r["dashboard_url"].(string)
	used, _ := r["template_id"].(string)

	out := fmt.Sprintf("✅ **%s** workspace is live!\n\n**%d agents** are ready.\nTemplate: `%s`\nDashboard: `%s`",
		name, int(count), used, dashURL)

	if conns, ok := r["connectors"].([]any); ok && len(conns) > 0 {
		cs := make([]string, 0, len(conns))
		for _, c := range conns {
			cs = append(cs, fmt.Sprint(c))
		}
		out += "\n\nSuggested integrations to connect: " + strings.Join(cs, ", ")
	}
	return TextResult(out)
}

func (t *WorkspaceBuilderTool) list(ctx context.Context) *Result {
	tmplResp, _ := t.get(ctx, "/v1/templates")
	instResp, _ := t.get(ctx, "/v1/templates/installed")

	var templates []map[string]any
	var installed []map[string]any
	json.Unmarshal(tmplResp, &templates)
	json.Unmarshal(instResp, &installed)

	var sb strings.Builder

	if len(installed) > 0 {
		sb.WriteString(fmt.Sprintf("### Installed (%d)\n", len(installed)))
		for _, w := range installed {
			n, _ := w["name"].(string)
			id, _ := w["template_id"].(string)
			cnt, _ := w["agent_count"].(float64)
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`, %d agents)\n", n, id, int(cnt)))
		}
		sb.WriteString("\n")
	}

	if len(templates) > 0 {
		sb.WriteString(fmt.Sprintf("### Available Templates (%d)\n", len(templates)))
		bycat := map[string][]string{}
		for _, tmpl := range templates {
			cat, _ := tmpl["category"].(string)
			id, _ := tmpl["id"].(string)
			name, _ := tmpl["name"].(string)
			icon, _ := tmpl["icon"].(string)
			bycat[cat] = append(bycat[cat], fmt.Sprintf("%s %s (`%s`)", icon, name, id))
		}
		for cat, items := range bycat {
			sb.WriteString(fmt.Sprintf("\n**%s**\n", strings.Title(cat)))
			for _, item := range items {
				sb.WriteString("- " + item + "\n")
			}
		}
	}
	return TextResult(sb.String())
}

func (t *WorkspaceBuilderTool) addAgent(ctx context.Context, args map[string]any) *Result {
	agentDef, ok := args["agent"].(map[string]any)
	if !ok {
		return ErrorResult("agent object required: {key, name, role, system_prompt}")
	}

	// Set sensible defaults
	if agentDef["status"] == nil { agentDef["status"] = "active" }
	if agentDef["tool_profile"] == nil { agentDef["tool_profile"] = "full" }
	if agentDef["max_tool_iterations"] == nil { agentDef["max_tool_iterations"] = 20 }
	if agentDef["context_window"] == nil { agentDef["context_window"] = 128000 }
	if agentDef["memory_enabled"] == nil { agentDef["memory_enabled"] = true }
	if agentDef["temperature"] == nil { agentDef["temperature"] = 0.5 }

	// agent_key and display_name aliases
	if agentDef["agent_key"] == nil {
		if k, _ := agentDef["key"].(string); k != "" {
			agentDef["agent_key"] = k
		}
	}
	if agentDef["display_name"] == nil {
		if n, _ := agentDef["name"].(string); n != "" {
			agentDef["display_name"] = n
		}
	}

	body, _ := json.Marshal(agentDef)
	resp, err := t.post(ctx, "/v1/agents", body)
	if err != nil {
		return ErrorResult("agent creation failed: " + err.Error())
	}

	var r map[string]any
	json.Unmarshal(resp, &r)

	id, _ := r["id"].(string)
	name, _ := r["display_name"].(string)
	if name == "" {
		name, _ = agentDef["display_name"].(string)
	}

	return TextResult(fmt.Sprintf("✅ Agent **%s** created! (ID: `%s`)\n\nThey're active now. Chat at `/qors/%s`", name, id, id))
}

func (t *WorkspaceBuilderTool) saveDashboard(ctx context.Context, args map[string]any) *Result {
	wsID, _ := args["workspace_id"].(string)
	desc, _ := args["description"].(string)
	dashCfg, _ := args["dashboard_config"].(map[string]any)

	if dashCfg == nil && desc != "" {
		dashCfg = inferDashboardBlocks(desc)
	}
	if dashCfg == nil {
		return ErrorResult("provide dashboard_config or description")
	}
	if wsID == "" {
		wsID = fmt.Sprintf("custom-%d", time.Now().Unix())
	}

	body, _ := json.Marshal(map[string]any{
		"template_id": wsID,
		"config":      dashCfg,
	})
	t.post(ctx, "/v1/dashboards", body)

	blockCount := 0
	if blocks, ok := dashCfg["blocks"].([]any); ok {
		blockCount = len(blocks)
	}

	return TextResult(fmt.Sprintf("✅ Dashboard saved with %d blocks! View at: `/dashboard/%s`", blockCount, wsID))
}

func (t *WorkspaceBuilderTool) status(ctx context.Context) *Result {
	resp, err := t.get(ctx, "/v1/templates/installed")
	if err != nil {
		return ErrorResult("status check failed: " + err.Error())
	}

	var installed []map[string]any
	json.Unmarshal(resp, &installed)

	if len(installed) == 0 {
		return TextResult("No workspaces installed. Say 'build me a [type] workspace' to create one!")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%d active workspaces:**\n\n", len(installed)))
	for _, w := range installed {
		n, _ := w["name"].(string)
		id, _ := w["template_id"].(string)
		cnt, _ := w["agent_count"].(float64)
		sb.WriteString(fmt.Sprintf("- **%s** — %d agents — `/dashboard/%s`\n", n, int(cnt), id))
	}
	return TextResult(sb.String())
}

// containsAnyWord checks if s contains any of the given words.
func containsAnyWord(s string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// inferDashboardBlocks generates a sensible dashboard layout from a description.
func inferDashboardBlocks(desc string) map[string]any {
	lower := strings.ToLower(desc)
	blocks := []map[string]any{{"type": "stat-row"}}

	if containsAnyWord(lower, "chart", "graph", "trend", "metric", "performance") {
		blocks = append(blocks, map[string]any{"type": "chart", "chartType": "bar", "title": "Overview"})
	}
	if containsAnyWord(lower, "table", "list", "data", "records") {
		blocks = append(blocks, map[string]any{"type": "data-table", "title": "Data", "searchable": true,
			"columns": []map[string]any{{"key": "name", "label": "Name"}, {"key": "status", "label": "Status"}}})
	}
	if containsAnyWord(lower, "pipeline", "funnel", "stage", "deal") {
		blocks = append(blocks, map[string]any{"type": "pipeline", "title": "Pipeline"})
	}
	if containsAnyWord(lower, "kanban", "board", "task", "todo") {
		blocks = append(blocks, map[string]any{"type": "kanban", "title": "Tasks"})
	}
	if containsAnyWord(lower, "calendar", "schedule", "event") {
		blocks = append(blocks, map[string]any{"type": "calendar", "title": "Schedule"})
	}
	if len(blocks) == 1 {
		blocks = append(blocks, map[string]any{"type": "feed", "title": "Activity"})
	}

	layout := "grid-2col"
	if len(blocks) >= 5 {
		layout = "grid-3col"
	}
	return map[string]any{"layout": layout, "title": "Dashboard", "blocks": blocks}
}

func (t *WorkspaceBuilderTool) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", t.apiBase+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if t.getToken != nil {
		if tok := t.getToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

func (t *WorkspaceBuilderTool) get(ctx context.Context, path string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", t.apiBase+path, nil)
	if t.getToken != nil {
		if tok := t.getToken(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 1<<16))
}
