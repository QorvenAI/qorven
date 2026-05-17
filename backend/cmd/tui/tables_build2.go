// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"

	"charm.land/bubbles/v2/table"
)

func (m *Model) newGitHubTaskTable() table.Model {
	_, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 8},
		{Title: "Branch", Width: 24},
		{Title: "Phase", Width: 14},
		{Title: "PR", Width: 6},
		{Title: "Agent", Width: 14},
		{Title: "Updated", Width: 8},
	}
	rows := make([]table.Row, len(m.githubTasks))
	for i, t := range m.githubTasks {
		pr := "—"
		if t.PRNumber > 0 {
			pr = fmt.Sprintf("#%d", t.PRNumber)
		}
		branch := t.Branch
		if len(branch) > 22 {
			branch = branch[:22] + "…"
		}
		agentID := t.AgentID
		if len(agentID) > 12 {
			agentID = agentID[:12]
		}
		rows[i] = table.Row{t.ID, branch, t.Phase, pr, agentID, t.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No active GitHub tasks", "", "", "", ""}}
	}
	tbl := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithHeight(h))
	tbl.SetStyles(qorvenTableStyles())
	return tbl
}

func (m *Model) newSettingsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "Setting", Width: 28},
		{Title: "Value", Width: 52},
	}

	status := m.api.getStatus()
	providers := m.api.listProviders()
	agents := m.api.listAgents()
	skills := m.api.listSkills()
	mcpServers := m.api.listMCPServers()

	enabledProviders := 0
	for _, p := range providers {
		if p.Enabled == "yes" {
			enabledProviders++
		}
	}

	integrations := m.api.listIntegrations()
	integrationRows := []table.Row{{"── Data Integrations", ""}}
	for _, ig := range integrations {
		val := "not configured — set key via n=new"
		if ig.Configured {
			val = "✓ configured  " + ig.KeyHint
		}
		integrationRows = append(integrationRows, table.Row{"  " + ig.Name, val})
	}
	if len(integrations) == 0 {
		integrationRows = append(integrationRows, table.Row{"  (unavailable)", "server unreachable"})
	}

	rows := []table.Row{
		{"── System", ""},
		{"  Gateway Status", status["status"]},
		{"  Version", status["version"]},
		{"  Uptime", status["uptime"]},
		{"  Environment", status["environment"]},
		{"── Agents & Providers", ""},
		{"  Active Agent", m.agentName + " (" + m.agentID + ")"},
		{"  Model", m.modelName},
		{"  Total Agents", fmt.Sprintf("%d", len(agents))},
		{"  Total Providers", fmt.Sprintf("%d (%d enabled)", len(providers), enabledProviders)},
		{"── Extensions", ""},
		{"  Skills Registered", fmt.Sprintf("%d", len(skills))},
		{"  MCP Servers", fmt.Sprintf("%d", len(mcpServers))},
		{"── Session", ""},
		{"  Session ID", m.sessionID},
	}
	rows = append(rows, integrationRows...)

	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newModelsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "✓", Width: 2},
		{Title: "Provider", Width: 18},
		{Title: "Model", Width: 36},
		{Title: "Max Ctx", Width: 10},
		{Title: "$/1M in", Width: 10},
		{Title: "Tools", Width: 6},
	}
	rows := make([]table.Row, len(m.modelsData))
	for i, e := range m.modelsData {
		sel := " "
		if e.Selected {
			if e.IsDefault {
				sel = "★"
			} else {
				sel = "✓"
			}
		}
		maxCtx := ""
		if e.MaxInput > 0 {
			maxCtx = fmt.Sprintf("%dk", e.MaxInput/1000)
		}
		cost := ""
		if e.InputCost > 0 {
			cost = fmt.Sprintf("%.2f", e.InputCost)
		}
		tools := "no"
		if e.HasTools {
			tools = "yes"
		}
		rows[i] = table.Row{sel, e.ProviderName, e.ModelID, maxCtx, cost, tools}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No models in registry", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newSessionsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Agent", Width: 10},
		{Title: "Channel", Width: 10},
		{Title: "Msgs", Width: 6},
		{Title: "Tokens", Width: 12},
		{Title: "Updated", Width: 12},
	}
	rows := make([]table.Row, len(m.sessionsData))
	for i, s := range m.sessionsData {
		rows[i] = table.Row{s.ID, s.Agent, s.Channel, s.MsgCount, s.Tokens, s.Updated}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newProvidersTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 14},
		{Title: "Type", Width: 14},
		{Title: "API Base", Width: 30},
		{Title: "Enabled", Width: 8},
	}
	rows := make([]table.Row, len(m.providersData))
	for i, p := range m.providersData {
		rows[i] = table.Row{p.ID, p.Name, p.Type, p.APIBase, p.Enabled}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newVoiceTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 16},
		{Title: "Kind", Width: 10},
		{Title: "Driver", Width: 16},
		{Title: "Enabled", Width: 8},
	}
	rows := make([]table.Row, len(m.voiceData))
	for i, p := range m.voiceData {
		rows[i] = table.Row{p.ID, p.Name, p.Kind, p.Driver, p.Enabled}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newMediaTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 16},
		{Title: "Kind", Width: 10},
		{Title: "Driver", Width: 16},
		{Title: "Default", Width: 8},
	}
	rows := make([]table.Row, len(m.mediaData))
	for i, p := range m.mediaData {
		rows[i] = table.Row{p.ID, p.Name, p.Kind, p.Driver, p.Default}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newWorkflowsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 28},
		{Title: "Trigger", Width: 14},
		{Title: "Steps", Width: 6},
		{Title: "On", Width: 4},
		{Title: "Updated", Width: 7},
	}
	rows := make([]table.Row, len(m.workflowsData))
	for i, wf := range m.workflowsData {
		name := wf.Name
		if len(name) > 26 {
			name = name[:26] + "…"
		}
		rows[i] = table.Row{wf.ID, name, wf.TriggerType, fmt.Sprintf("%d", wf.StepCount), wf.Enabled, wf.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No workflows", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newVaultTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 24},
		{Title: "Kind", Width: 12},
		{Title: "Description", Width: 36},
		{Title: "Updated", Width: 12},
	}
	rows := make([]table.Row, len(m.vaultData))
	for i, e := range m.vaultData {
		rows[i] = table.Row{e.ID, e.Name, e.Kind, e.Description, e.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "Vault is empty", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newUsageTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "Agent", Width: 30},
		{Title: "API Calls", Width: 12},
		{Title: "Cost USD", Width: 12},
	}
	rows := make([]table.Row, len(m.usageData.ByAgent))
	for i, a := range m.usageData.ByAgent {
		rows[i] = table.Row{a.AgentName, fmt.Sprintf("%d", a.SessionCount), fmt.Sprintf("$%.4f", a.CostUSD)}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"(no data)", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newNotificationsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "●", Width: 2},
		{Title: "Kind", Width: 12},
		{Title: "Title", Width: 28},
		{Title: "Body", Width: 40},
		{Title: "Date", Width: 12},
	}
	rows := make([]table.Row, len(m.notificationsData))
	for i, n := range m.notificationsData {
		dot := "○"
		if !n.Read {
			dot = "●"
		}
		rows[i] = table.Row{dot, n.Kind, n.Title, n.Body, n.CreatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "", "No notifications", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newProviderKeysTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Label", Width: 20},
		{Title: "Status", Width: 10},
		{Title: "Used", Width: 8},
		{Title: "Budget $", Width: 10},
		{Title: "Spent $", Width: 10},
	}
	rows := make([]table.Row, len(m.providerKeysData.Keys))
	for i, k := range m.providerKeysData.Keys {
		budget := "—"
		if k.BudgetUSD > 0 {
			budget = fmt.Sprintf("$%.2f", k.BudgetUSD)
		}
		spent := "—"
		if k.SpentUSD > 0 {
			spent = fmt.Sprintf("$%.2f", k.SpentUSD)
		}
		id := k.ID
		if len(id) > 8 {
			id = id[:8]
		}
		rows[i] = table.Row{id, k.Label, k.Status, fmt.Sprintf("%d", k.UsageCount), budget, spent}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No keys for this provider", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}
