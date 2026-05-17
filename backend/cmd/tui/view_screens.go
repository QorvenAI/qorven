// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// ─────────────────────────────────────────────────────────────────────────────
// Router view
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderRouterView() string {
	banner := renderBanner(m.width)
	title := fmt.Sprintf("Smart Router  (%d categories)", len(m.routerData))
	titleLine := " " + headerStyle.Render(title) + "  " + dimStyle.Render("r=refresh")
	div := dividerStyle.Render(strings.Repeat("─", m.width))

	var sb strings.Builder
	sb.WriteString(banner + "\n")
	sb.WriteString(titleLine + "\n")
	sb.WriteString(div + "\n")
	sb.WriteString(m.routerTable.View() + "\n")

	if len(m.rankingsData) > 0 {
		sb.WriteString(div + "\n")
		sb.WriteString(" " + headerStyle.Render("Model Rankings") + "  " + dimStyle.Render("source: artificialanalysis.ai") + "\n")
		header := fmt.Sprintf("  %-3s  %-28s  %-10s  %-8s  %-8s  %8s  %8s",
			"#", "Model", "Org", "Intel", "Code", "Tok/s", "$/1M in")
		sb.WriteString(dimStyle.Render(header) + "\n")
		limit := 10
		if len(m.rankingsData) < limit {
			limit = len(m.rankingsData)
		}
		for _, r := range m.rankingsData[:limit] {
			name := r.Name
			if len(name) > 28 {
				name = name[:27] + "…"
			}
			org := r.Organization
			if len(org) > 10 {
				org = org[:9] + "…"
			}
			intel := "—"
			if r.IntelligenceIdx > 0 {
				intel = fmt.Sprintf("%.1f", r.IntelligenceIdx)
			}
			code := "—"
			if r.CodingIdx > 0 {
				code = fmt.Sprintf("%.1f", r.CodingIdx)
			}
			speed := "—"
			if r.SpeedTPS > 0 {
				speed = fmt.Sprintf("%d", int(r.SpeedTPS))
			}
			price := "—"
			if r.InputPricePerM > 0 {
				price = fmt.Sprintf("$%.2f", r.InputPricePerM)
			}
			line := fmt.Sprintf("  %-3d  %-28s  %-10s  %-8s  %-8s  %8s  %8s",
				r.Rank, name, org, intel, code, speed, price)
			sb.WriteString(line + "\n")
		}
	} else {
		sb.WriteString(div + "\n")
		sb.WriteString(" " + dimStyle.Render("Rankings unavailable — add ArtificialAnalysis API key via /settings") + "\n")
	}

	sb.WriteString(div + "\n")
	sb.WriteString(" " + m.help.ShortHelpView([]key.Binding{keys.Up, keys.Down, keys.Refresh, keys.Back}))
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Home screen
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderHomeScreen() string {
	banner := renderBanner(m.width)
	div := dividerStyle.Render(strings.Repeat("─", m.width))

	var sb strings.Builder
	sb.WriteString(banner + "\n")

	status := m.homeData.SystemStatus
	if status == "" {
		status = "unknown"
	}
	statusDot := lipgloss.NewStyle().Foreground(dimText).Render("●")
	if status == "online" || status == "ok" {
		statusDot = lipgloss.NewStyle().Foreground(green).Render("●")
	}
	ver := m.homeData.GatewayVersion
	if ver == "" {
		ver = "—"
	}
	sb.WriteString(" " + statusDot + "  " + headerStyle.Render("Qorven Gateway") + "  " + dimStyle.Render(status+"  v"+ver) + "\n")
	sb.WriteString(div + "\n")

	stats := []struct{ label, val string }{
		{"Plans pending", fmt.Sprintf("%d", m.homeData.PendingPlans)},
		{"Escalations", fmt.Sprintf("%d", m.homeData.PendingEscalations)},
		{"Active tasks", fmt.Sprintf("%d", m.homeData.ActiveTasks)},
		{"Daemon agents", fmt.Sprintf("%d", m.homeData.ActiveDaemonAgents)},
		{"Unread notifs", fmt.Sprintf("%d", m.homeData.UnreadNotifications)},
	}
	colW := (m.width - 2) / len(stats)
	if colW < 10 {
		colW = 10
	}
	var statLine strings.Builder
	statLine.WriteString(" ")
	for i, s := range stats {
		val := s.val
		valStyle := lipgloss.NewStyle().Foreground(purple).Bold(true)
		if (s.label == "Plans pending" || s.label == "Escalations") && val != "0" {
			valStyle = lipgloss.NewStyle().Foreground(amber).Bold(true)
		}
		cell := valStyle.Render(val) + "  " + dimStyle.Render(s.label)
		statLine.WriteString(fmt.Sprintf("%-*s", colW, cell))
		if i < len(stats)-1 {
			statLine.WriteString(dimStyle.Render("│"))
		}
	}
	sb.WriteString(statLine.String() + "\n")
	sb.WriteString(div + "\n")

	sb.WriteString(" " + headerStyle.Render("Recent Sessions") + "  " + dimStyle.Render("↵=resume  r=refresh") + "\n")
	sb.WriteString(m.homeSessionTable.View() + "\n")
	sb.WriteString(div + "\n")

	nav := []struct{ key, label string }{
		{"/chat", "open chat"},
		{"/agents", "agents"},
		{"/plans", "approve plans"},
		{"/channels", "channels"},
		{"/workers", "daemon workers"},
		{"/notifications", "notifications"},
		{"/usage", "token usage"},
		{"/vault", "vault"},
		{"/settings", "settings"},
	}
	sb.WriteString(" " + headerStyle.Render("Quick Nav") + "\n ")
	var navParts []string
	for _, n := range nav {
		navParts = append(navParts, lipgloss.NewStyle().Foreground(purple).Render(n.key)+"  "+dimStyle.Render(n.label))
	}
	for i := 0; i < len(navParts); i += 2 {
		left := navParts[i]
		right := ""
		if i+1 < len(navParts) {
			right = navParts[i+1]
		}
		sb.WriteString(" " + fmt.Sprintf("%-45s %s", left, right) + "\n")
	}

	sb.WriteString(div + "\n")
	sb.WriteString(" " + dimStyle.Render("↵=open chat  r=refresh  ctrl+c=quit"))
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Usage screen
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderUsageScreen() string {
	banner := renderBanner(m.width)
	div := dividerStyle.Render(strings.Repeat("─", m.width))

	var sb strings.Builder
	sb.WriteString(banner + "\n")
	sb.WriteString(" " + headerStyle.Render(fmt.Sprintf("Token Usage  ·  this month: $%.4f", m.usageData.TotalCostUSD)) + "\n")
	sb.WriteString(div + "\n")
	sb.WriteString(m.usageTable.View() + "\n")
	sb.WriteString(div + "\n")
	sb.WriteString(" " + m.help.ShortHelpView([]key.Binding{keys.Up, keys.Down, keys.Refresh, keys.Back}))
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Room chat screen
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderRoomChatScreen() string {
	banner := renderBanner(m.width)
	div := dividerStyle.Render(strings.Repeat("─", m.width))

	prompt := lipgloss.NewStyle().Foreground(purple).Bold(true).Render(" ❯ ")
	inputLine := prompt + m.roomChatInput.View()

	content := renderRoomMessages(m.roomMessages, m.width-4)
	m.roomChatViewport.SetContent(content)

	var sb strings.Builder
	sb.WriteString(banner + "\n")
	sb.WriteString(" " + headerStyle.Render("Room: "+m.roomChatName) + "  " + dimStyle.Render(fmt.Sprintf("%d messages  ·  esc=back  r=refresh", len(m.roomMessages))) + "\n")
	sb.WriteString(div + "\n")
	sb.WriteString(m.roomChatViewport.View() + "\n")
	sb.WriteString(div + "\n")
	sb.WriteString(inputLine + "\n")
	sb.WriteString(" " + dimStyle.Render("enter=send  esc=back"))
	return sb.String()
}

func renderRoomMessages(msgs []RoomMessage, width int) string {
	if len(msgs) == 0 {
		return "\n  " + dimStyle.Render("No messages yet — send the first one below") + "\n"
	}
	var sb strings.Builder
	for _, msg := range msgs {
		ts := msg.CreatedAt
		if len(ts) > 10 {
			ts = ts[11:16] // HH:MM
		}
		var label string
		switch msg.Role {
		case "user":
			label = userLabel.Render("  " + msg.Author)
		case "soul", "assistant":
			label = agentLabel.Render("  " + msg.Author)
		default:
			label = systemLabel.Render("  " + msg.Author)
		}
		sb.WriteString("\n " + label + "  " + dimStyle.Render(ts) + "\n")
		for _, line := range wrapText(msg.Content, width) {
			sb.WriteString("   " + line + "\n")
		}
	}
	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Workers view — tabbed: Agents / Tasks / Plans
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderWorkersView() string {
	banner := renderBanner(m.width)
	div := dividerStyle.Render(strings.Repeat("─", m.width))

	tabLabels := []string{
		fmt.Sprintf("Agents (%d)", len(m.workersAgents)),
		fmt.Sprintf("Tasks (%d)", len(m.workersTasks)),
		fmt.Sprintf("Plans (%d)", len(m.workersPlans)),
	}
	var tabBar strings.Builder
	tabBar.WriteString(" ")
	for i, label := range tabLabels {
		if i == m.workersSubView {
			tabBar.WriteString(lipgloss.NewStyle().Foreground(logoMagenta).Bold(true).Underline(true).Render(label))
		} else {
			tabBar.WriteString(dimStyle.Render(label))
		}
		if i < len(tabLabels)-1 {
			tabBar.WriteString(dimStyle.Render("  ·  "))
		}
	}

	hints := []string{
		"tab=switch  r=refresh",
		"tab=switch  d=cancel  r=refresh",
		"tab=switch  ↵=approve  d=reject  r=refresh",
	}

	var body string
	switch m.workersSubView {
	case 0:
		body = m.workersAgentTable.View()
	case 1:
		body = m.tasksTable.View()
	case 2:
		body = m.workersPlanTable.View()
	}

	pending := 0
	for _, p := range m.workersPlans {
		if p.Status == "pending" {
			pending++
		}
	}
	title := "Workers"
	if pending > 0 {
		title += fmt.Sprintf("  · !! %d pending approval", pending)
	}
	titleLine := " " + headerStyle.Render(title)

	return strings.Join([]string{
		banner,
		titleLine,
		tabBar.String(),
		div,
		body,
		div,
		" " + dimStyle.Render(hints[m.workersSubView]),
	}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// GitHub view
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderGitHubView() string {
	banner := renderBanner(m.width)
	div := dividerStyle.Render(strings.Repeat("─", m.width))

	if m.githubOwner == "" || m.githubRepo == "" {
		var sb strings.Builder
		sb.WriteString(banner + "\n")
		sb.WriteString(" " + headerStyle.Render("GitHub") + "\n")
		sb.WriteString(div + "\n\n")
		sb.WriteString("  " + dimStyle.Render("No GitHub repository connected.") + "\n\n")
		sb.WriteString("  Connect one using:\n\n")
		sb.WriteString("  " + lipgloss.NewStyle().Foreground(purple).Render("/github owner/repo") + "  " + dimStyle.Render("— e.g. /github acme/api") + "\n\n")
		sb.WriteString("  " + dimStyle.Render("Or set GITHUB_OWNER and GITHUB_REPO environment variables.") + "\n")
		sb.WriteString("\n" + div + "\n")
		sb.WriteString(" " + dimStyle.Render("esc=back"))
		return sb.String()
	}

	repoLabel := m.githubOwner + "/" + m.githubRepo

	tabs := []string{"PRs", "Issues", "Tasks"}
	var tabBar strings.Builder
	tabBar.WriteString(" ")
	for i, t := range tabs {
		if i == m.githubSubView {
			tabBar.WriteString(lipgloss.NewStyle().Foreground(logoMagenta).Bold(true).Underline(true).Render(t))
		} else {
			tabBar.WriteString(dimStyle.Render(t))
		}
		if i < len(tabs)-1 {
			tabBar.WriteString(dimStyle.Render("  ·  "))
		}
	}

	titleLine := " " + headerStyle.Render("GitHub  "+repoLabel)

	var content string
	switch m.githubSubView {
	case 0:
		content = m.githubPRTable.View()
	case 1:
		content = m.githubIssueTable.View()
	case 2:
		content = m.githubTaskTable.View()
	}

	hintMap := map[int]string{
		0: "↵=merge  o=open in browser  tab=switch",
		1: "↵=assign to Prime  d=close  o=open in browser  tab=switch",
		2: "o=open PR  tab=switch",
	}
	hint := dimStyle.Render(hintMap[m.githubSubView])

	return strings.Join([]string{banner, titleLine, tabBar.String(), div, content, div, " " + hint}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// Utilities
// ─────────────────────────────────────────────────────────────────────────────

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	sec := int(d / time.Second)
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	mins := sec / 60
	s := sec % 60
	return fmt.Sprintf("%dm%02ds", mins, s)
}

func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		runes := []rune(paragraph)
		for len(runes) > 0 {
			if len(runes) <= maxWidth {
				result = append(result, string(runes))
				break
			}
			breakAt := maxWidth
			for i := maxWidth; i > maxWidth/2; i-- {
				if runes[i] == ' ' {
					breakAt = i
					break
				}
			}
			result = append(result, string(runes[:breakAt]))
			runes = runes[breakAt:]
			for len(runes) > 0 && runes[0] == ' ' {
				runes = runes[1:]
			}
		}
	}
	return result
}
