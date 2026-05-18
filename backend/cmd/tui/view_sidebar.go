// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ─────────────────────────────────────────────────────────────────────────────
// Progress pills — active daemon tasks shown above the input
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderProgressPills(width int) string {
	var pills []string
	for _, t := range m.workersTasks {
		if t.Status == "in_progress" || t.Status == "running" {
			label := t.Title
			if len(label) > 20 {
				label = label[:19] + "…"
			}
			pill := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0B0B0F")).
				Background(amber).
				Padding(0, 1).
				Render("⚙ " + label)
			pills = append(pills, pill)
		}
	}
	for _, p := range m.workersPlans {
		if p.Status == "pending" {
			label := p.Title
			if len(label) > 18 {
				label = label[:17] + "…"
			}
			pill := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0B0B0F")).
				Background(purple).
				Padding(0, 1).
				Render("!! " + label)
			pills = append(pills, pill)
		}
	}
	if m.homeData.UnreadNotifications > 0 {
		pill := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0B0B0F")).
			Background(cyan).
			Padding(0, 1).
			Render(fmt.Sprintf("🔔 %d unread", m.homeData.UnreadNotifications))
		pills = append(pills, pill)
	}
	if len(pills) == 0 {
		return ""
	}
	line := " " + strings.Join(pills, "  ")
	if lipgloss.Width(line) > width {
		line = " " + dimStyle.Render(fmt.Sprintf("%d active  /workers to see all", len(pills)))
	}
	return line
}

// ─────────────────────────────────────────────────────────────────────────────
// Status bar
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderStatusBar(width int) string {
	dot := dimStyle.Render(" · ")

	var parts []string

	connDot := lipgloss.NewStyle().Foreground(green).Render("●")
	latency := m.gatewayLatency
	if !m.gatewayOnline {
		connDot = lipgloss.NewStyle().Foreground(red).Render("●")
		latency = "offline"
	} else if latency == "" {
		connDot = lipgloss.NewStyle().Foreground(amber).Render("●")
		latency = "…"
	}
	rtDot := ""
	if m.rtSubActive {
		rtDot = lipgloss.NewStyle().Foreground(green).Render("⇌")
	} else {
		rtDot = lipgloss.NewStyle().Foreground(dimText).Render("⇌")
	}
	parts = append(parts, connDot+" "+dimStyle.Render(latency)+" "+rtDot)
	parts = append(parts, statusKey.Render("esc")+" "+statusDesc.Render("cancel"))
	parts = append(parts, statusKey.Render("/")+" "+statusDesc.Render("commands"))
	if m.thinkingLevel != "" && m.thinkingLevel != "off" {
		label := "think:on"
		if m.thinkingLevel == "high" {
			label = "think:high"
		}
		parts = append(parts, statusKey.Render("ctrl+t")+" "+dimStyle.Render(label))
	}
	parts = append(parts, statusKey.Render("ctrl+b")+" "+statusDesc.Render("sidebar"))
	parts = append(parts, statusKey.Render("ctrl+c")+" "+statusDesc.Render("quit"))
	if m.taskCount > 0 {
		parts = append(parts, fmt.Sprintf("⚡ %d running", m.taskCount))
	}

	left := strings.Join(parts, dot)

	chars := 0
	for _, msg := range m.messages {
		chars += len(msg.Content)
	}
	est := chars / 4
	tokenStr := fmt.Sprintf("%d tok", est)
	if est >= 1000 {
		tokenStr = fmt.Sprintf("%.1fk tok", float64(est)/1000)
	}
	ver := m.homeData.GatewayVersion
	if ver == "" {
		ver = "dev"
	}
	right := dimStyle.Render(tokenStr) + dot + dimStyle.Render(ver)

	if m.buildPhase != "" && m.buildPhase != "idle" && m.buildPhase != "done" {
		right = dimStyle.Render(m.buildPhase) + " " + m.buildProgress.View() + "  " + right
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}
	line := " " + left + strings.Repeat(" ", gap) + right
	return statusBar.Width(width).Render(line)
}

// ─────────────────────────────────────────────────────────────────────────────
// Slash / mention popups
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderSlashPopup() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderClr).
		Padding(0, 1).
		Width(lipgloss.Width(m.slashPopup.View()) + 4)
	return box.Render(m.slashPopup.View())
}

func (m Model) renderMentionPopup() string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyan).
		Padding(0, 1).
		Width(lipgloss.Width(m.mentionPopup.View()) + 4)
	header := lipgloss.NewStyle().Foreground(cyan).Render("@agent") + "  " + dimStyle.Render("tab=select  esc=close")
	return header + "\n" + box.Render(m.mentionPopup.View())
}

// ─────────────────────────────────────────────────────────────────────────────
// Sidebar
// ─────────────────────────────────────────────────────────────────────────────

func sectionDivider(label string, w int) string {
	title := sectionTitle.Render(label)
	divW := w - lipgloss.Width(title) - 1
	if divW < 1 {
		divW = 1
	}
	return title + " " + dividerStyle.Render(strings.Repeat("─", divW))
}

func (m Model) renderSidebar(width, height int) string {
	w := width - 1
	sep := func(label string) string { return sectionDivider(label, w) }

	var lines []string

	hatch := hatchStyle.Render(strings.Repeat("╱", w))
	lines = append(lines,
		hatch,
		brandStyle.Render("  Qorven™"),
		logoStyle.Render("  QORVEN"),
		hatch,
		"",
	)

	sessDisplay := m.sessionID
	if len(sessDisplay) > 16 {
		sessDisplay = sessDisplay[:16] + "…"
	}
	if sessDisplay == "" {
		sessDisplay = "new session"
	}
	lines = append(lines,
		dimStyle.Render("  "+sessDisplay),
		"",
	)

	lines = append(lines, sep("Health"))
	connDot := lipgloss.NewStyle().Foreground(green).Render("  ● ")
	connLabel := "online"
	if !m.gatewayOnline {
		connDot = lipgloss.NewStyle().Foreground(red).Render("  ● ")
		connLabel = "offline"
	}
	latencyStr := ""
	if m.gatewayLatency != "" && m.gatewayOnline {
		latencyStr = " " + dimStyle.Render(m.gatewayLatency)
	}
	lines = append(lines, connDot+dimStyle.Render(connLabel)+latencyStr)

	if m.homeData.UnreadNotifications > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(cyan).Render(fmt.Sprintf("  🔔 %d unread", m.homeData.UnreadNotifications)),
		)
	}
	if m.homeData.PendingPlans > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(amber).Render(fmt.Sprintf("  ⏳ %d pending plan(s)", m.homeData.PendingPlans)),
		)
	}
	if m.homeData.PendingEscalations > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(red).Render(fmt.Sprintf("  ⚠ %d escalation(s)", m.homeData.PendingEscalations)),
		)
	}
	lines = append(lines, "")

	lines = append(lines, sep("Agent"))

	modelDiamond := lipgloss.NewStyle().Foreground(purple).Render("  ◇ ")
	lines = append(lines,
		activeItem.Render("  "+m.agentName),
		modelDiamond+dimStyle.Render(m.modelName),
	)

	switch m.thinkingLevel {
	case "medium":
		lines = append(lines, lipgloss.NewStyle().Foreground(amber).Render("  Thinking On"))
	case "high":
		lines = append(lines, lipgloss.NewStyle().Foreground(amber).Bold(true).Render("  Thinking High"))
	}
	if m.isStreaming {
		lines = append(lines, lipgloss.NewStyle().Foreground(green).Render("  ● responding…"))
	}

	chars := 0
	for _, msg := range m.messages {
		chars += len(msg.Content)
	}
	est := chars / 4
	tokStr := fmt.Sprintf("%d", est)
	if est >= 1000 {
		tokStr = fmt.Sprintf("%.1fk", float64(est)/1000)
	}
	lines = append(lines, dimStyle.Render(fmt.Sprintf("  %d msgs · %s tok", len(m.messages), tokStr)))
	lines = append(lines, "")

	if m.isStreaming && len(m.messages) > 0 {
		last := m.messages[len(m.messages)-1]
		runningTools := []string{}
		for _, t := range last.ToolEvents {
			if t.status == "running" {
				runningTools = append(runningTools, t.name)
			}
		}
		if len(runningTools) > 0 {
			lines = append(lines, sep("Running"))
			for _, tn := range runningTools {
				name := tn
				if len(name) > w-4 {
					name = name[:w-5] + "…"
				}
				lines = append(lines,
					lipgloss.NewStyle().Foreground(amber).Render("  ● ")+
						lipgloss.NewStyle().Foreground(cyan).Render(name),
				)
			}
			lines = append(lines, "")
		}
	}

	if len(m.codeChanges) > 0 {
		lines = append(lines, sep("Modified Files"))
		for _, c := range m.codeChanges {
			name := c
			if i := strings.LastIndex(c, "/"); i >= 0 {
				name = c[i+1:]
			}
			if len(name) > w-4 {
				name = name[:w-5] + "…"
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(amber).Render("  ✎ "+name))
		}
		lines = append(lines, "")
	}

	lines = append(lines, sep("MCPs"))
	if len(m.mcpData) == 0 {
		lines = append(lines, dimStyle.Render("  None"))
	} else {
		for _, srv := range m.mcpData {
			dot := lipgloss.NewStyle().Foreground(dimText).Render("  ● ")
			if srv.Status == "connected" || srv.Status == "running" {
				dot = lipgloss.NewStyle().Foreground(green).Render("  ● ")
			}
			name := srv.Name
			if len(name) > w-5 {
				name = name[:w-6] + "…"
			}
			lines = append(lines, dot+dimStyle.Render(name))
		}
	}
	lines = append(lines, "")

	lines = append(lines, sep("Keys"))
	keyHints := []struct{ k, d string }{
		{"ctrl+b", "toggle sidebar"},
		{"ctrl+t", "thinking mode"},
		{"/help", "all commands"},
		{"/model", "switch model"},
		{"/agent", "switch agent"},
		{"ctrl+c", "quit"},
	}
	for _, kh := range keyHints {
		kw := lipgloss.NewStyle().Foreground(purple).Render("  " + kh.k)
		lines = append(lines, kw+"  "+dimStyle.Render(kh.d))
	}

	body := strings.Join(lines, "\n")
	return sidebarBg.
		Width(width).
		Height(height).
		Padding(0, 0).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(borderClr).
		Render(body)
}
