// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
)

// ─── Table constructors ───────────────────────────────────────────────────────

func (m *Model) newHomeSessionTable() table.Model {
	w := m.width
	if w < 40 {
		w = 40
	}
	h := 8
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Agent", Width: 16},
		{Title: "Channel", Width: 10},
		{Title: "Msgs", Width: 6},
		{Title: "Tokens", Width: 12},
		{Title: "Updated", Width: 12},
	}
	rows := make([]table.Row, len(m.homeData.RecentSessions))
	for i, s := range m.homeData.RecentSessions {
		rows[i] = table.Row{s.ID, s.Agent, s.Channel, s.MsgCount, s.Tokens, s.Updated}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No sessions yet — send a message to start", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newWorkersAgentTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "●", Width: 2},
		{Title: "Name", Width: 22},
		{Title: "Provider", Width: 14},
		{Title: "Model", Width: 18},
		{Title: "Status", Width: 10},
		{Title: "Capabilities", Width: 22},
	}
	rows := make([]table.Row, len(m.workersAgents))
	for i, a := range m.workersAgents {
		dot := "○"
		if a.Status == "running" || a.Status == "active" {
			dot = "●"
		}
		caps := strings.Join(a.Capabilities, ", ")
		if len(caps) > 22 {
			caps = caps[:21] + "…"
		}
		rows[i] = table.Row{dot, a.Name, a.Provider, a.Model, a.Status, caps}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"○", "No daemon agents — start one with /workers", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newWorkersPlanTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Title", Width: 32},
		{Title: "Proposed By", Width: 16},
		{Title: "Status", Width: 10},
		{Title: "Action", Width: 18},
	}
	rows := make([]table.Row, len(m.workersPlans))
	for i, p := range m.workersPlans {
		id := p.ID
		if len(id) > 8 {
			id = id[:8]
		}
		action := ""
		if p.Status == "pending" {
			action = "↵approve  d reject"
		}
		rows[i] = table.Row{id, p.Title, p.ProposedBy, p.Status, action}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No pending plans", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newTasksTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Title", Width: 30},
		{Title: "Assigned", Width: 10},
		{Title: "Priority", Width: 10},
		{Title: "Status", Width: 12},
		{Title: "Updated", Width: 12},
	}
	rows := make([]table.Row, len(m.tasksData))
	for i, t := range m.tasksData {
		rows[i] = table.Row{t.ID, t.Title, t.AssignedTo, t.Priority, t.Status, t.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No tasks", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newCronTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Agent", Width: 10},
		{Title: "Schedule", Width: 16},
		{Title: "Task", Width: 30},
		{Title: "Enabled", Width: 8},
		{Title: "Last Run", Width: 16},
	}
	rows := make([]table.Row, len(m.cronData))
	for i, j := range m.cronData {
		rows[i] = table.Row{j.ID, j.AgentID, j.Expression, j.Task, j.Enabled, j.LastRun}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "", "No cron jobs configured", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newMCPTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "Name", Width: 22},
		{Title: "URL", Width: 30},
		{Title: "Status", Width: 12},
		{Title: "ID (full)", Width: 36},
	}
	rows := make([]table.Row, len(m.mcpData))
	for i, s := range m.mcpData {
		url := s.URL
		if len(url) > 28 {
			url = url[:28] + "…"
		}
		rows[i] = table.Row{s.Name, url, s.Status, s.ID}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No MCP servers registered", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newRoomsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 24},
		{Title: "Description", Width: 30},
		{Title: "Members", Width: 8},
		{Title: "Messages", Width: 10},
	}
	rows := make([]table.Row, len(m.roomsData))
	for i, r := range m.roomsData {
		rows[i] = table.Row{r.ID, r.Name, r.Description, r.MemberCount, r.MessageCount}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No rooms", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newChannelsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "●", Width: 2},
		{Title: "Kind", Width: 12},
		{Title: "Name", Width: 22},
		{Title: "Agent", Width: 14},
		{Title: "Status", Width: 10},
		{Title: "ID", Width: 10},
	}
	channelIcons := map[string]string{
		"slack": "⚡", "email": "✉", "telegram": "✈", "webhook": "⎈",
		"discord": "◈", "whatsapp": "◉", "sms": "✆", "twitter": "◎",
		"instagram": "◑", "line": "◐", "signal": "◍", "wechat": "◎",
	}
	rows := make([]table.Row, len(m.channelsData))
	for i, c := range m.channelsData {
		dot := "○"
		if c.Status == "running" {
			dot = "●"
		}
		icon := channelIcons[c.Kind]
		if icon == "" {
			icon = "◇"
		}
		id := c.ID
		if len(id) > 8 {
			id = id[:8]
		}
		rows[i] = table.Row{dot, icon + " " + c.Kind, c.Name, c.Agent, c.Status, id}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "", "No channels configured — n=new", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newSkillsTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Slug", Width: 20},
		{Title: "Name", Width: 20},
		{Title: "Description", Width: 40},
	}
	rows := make([]table.Row, len(m.skillsData))
	for i, s := range m.skillsData {
		rows[i] = table.Row{s.ID, s.Slug, s.Name, s.Description}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "", "No skills registered", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newPlansTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Title", Width: 38},
		{Title: "Status", Width: 12},
		{Title: "Created", Width: 12},
	}
	rows := make([]table.Row, len(m.plansData))
	for i, p := range m.plansData {
		flag := p.Status
		if p.Status == "pending" {
			flag = "pending ↵approve d reject"
		}
		rows[i] = table.Row{p.ID, p.Title, flag, p.CreatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "No plans", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newMemoryTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "Score", Width: 8},
		{Title: "Type", Width: 12},
		{Title: "Content", Width: 60},
	}
	rows := make([]table.Row, len(m.memoryData))
	for i, r := range m.memoryData {
		rows[i] = table.Row{fmt.Sprintf("%.2f", r.Score), r.Type, r.Content}
	}
	if len(rows) == 0 {
		if m.memoryQuery == "" {
			rows = []table.Row{{"", "", "Type: /memory <query> to search"}}
		} else {
			rows = []table.Row{{"", "", "No results for: " + m.memoryQuery}}
		}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newDriveTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Name", Width: 30},
		{Title: "Kind", Width: 12},
		{Title: "Size", Width: 10},
		{Title: "Updated", Width: 12},
	}
	rows := make([]table.Row, len(m.driveData))
	for i, f := range m.driveData {
		rows[i] = table.Row{f.ID, f.Name, f.Kind, f.Size, f.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"—", "Drive is empty", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newDiscoveredTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "Provider", Width: 16},
		{Title: "Model ID", Width: 48},
		{Title: "Action", Width: 16},
	}
	rows := make([]table.Row, len(m.discoveredData))
	for i, d := range m.discoveredData {
		rows[i] = table.Row{d.ProviderID, d.ModelID, "↵ add  d ignore"}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No new models discovered", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newSupervisorTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "Agent", Width: 10},
		{Title: "Severity", Width: 10},
		{Title: "Status", Width: 12},
		{Title: "Message", Width: 40},
		{Title: "Created", Width: 12},
	}
	rows := make([]table.Row, len(m.escalationsData))
	for i, e := range m.escalationsData {
		rows[i] = table.Row{e.ID, e.AgentID, e.Severity, e.Status, e.Message, e.CreatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "", "", "", "No pending escalations", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newRouterTable() table.Model {
	w, h := m.listDimensions()
	cols := []table.Column{
		{Title: "Category", Width: 20},
		{Title: "Description", Width: 38},
		{Title: "Assigned Model", Width: 26},
	}
	rows := make([]table.Row, len(m.routerData))
	for i, c := range m.routerData {
		rows[i] = table.Row{c.Name, c.Description, c.AssignedTo}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No routing categories configured", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithWidth(w), table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newGitHubPRTable() table.Model {
	_, h := m.listDimensions()
	cols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Title", Width: 32},
		{Title: "Branch", Width: 22},
		{Title: "CI", Width: 10},
		{Title: "Author", Width: 14},
		{Title: "Age", Width: 6},
	}
	rows := make([]table.Row, len(m.githubPRs))
	for i, p := range m.githubPRs {
		ci := "⟳"
		switch p.CIStatus {
		case "passing":
			ci = "✓"
		case "failing":
			ci = "✗"
		}
		branch := p.HeadBranch
		if len(branch) > 20 {
			branch = branch[:20] + "…"
		}
		title := p.Title
		if len(title) > 30 {
			title = title[:30] + "…"
		}
		rows[i] = table.Row{fmt.Sprintf("#%d", p.Number), title, branch, ci, p.Author, p.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No open pull requests", "", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

func (m *Model) newGitHubIssueTable() table.Model {
	_, h := m.listDimensions()
	cols := []table.Column{
		{Title: "#", Width: 5},
		{Title: "Title", Width: 34},
		{Title: "Labels", Width: 18},
		{Title: "Assignee", Width: 14},
		{Title: "Age", Width: 6},
	}
	rows := make([]table.Row, len(m.githubIssues))
	for i, iss := range m.githubIssues {
		labels := strings.Join(iss.Labels, ", ")
		if len(labels) > 16 {
			labels = labels[:16] + "…"
		}
		title := iss.Title
		if len(title) > 32 {
			title = title[:32] + "…"
		}
		assignee := iss.Assignee
		if assignee == "" {
			assignee = "—"
		}
		rows[i] = table.Row{fmt.Sprintf("#%d", iss.Number), title, labels, assignee, iss.UpdatedAt}
	}
	if len(rows) == 0 {
		rows = []table.Row{{"", "No open issues", "", "", ""}}
	}
	t := table.New(table.WithColumns(cols), table.WithRows(rows), table.WithFocused(true),
		table.WithHeight(h))
	t.SetStyles(qorvenTableStyles())
	return t
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

func qorvenTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderClr).
		BorderBottom(true).
		Bold(true).
		Foreground(logoMagenta)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#0B0B0F")).
		Background(purple).
		Bold(true)
	s.Cell = s.Cell.Foreground(lipgloss.Color("#C4C4D4"))
	return s
}

func (m *Model) newList(title string, items []list.Item, singular, plural string) list.Model {
	w, h := m.listDimensions()
	l := list.New(items, list.NewDefaultDelegate(), w, h)
	l.Title = title
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetStatusBarItemName(singular, plural)
	l.Styles.Title = lipgloss.NewStyle().Foreground(logoMagenta).Bold(true).Padding(0, 1)
	return l
}
