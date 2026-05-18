// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/qorvenai/qorven/cmd/tui/steps"
)

// ─────────────────────────────────────────────────────────────────────────────
// Root View
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) View() tea.View {
	var v tea.View
	v.AltScreen = true

	if !m.ready {
		v.Content = "\n\n   Loading…"
		return v
	}

	switch m.route {
	// ── full-screen list routes ──────────────────────────────────────────────
	case routeAgents:
		v.Content = m.renderListScreen("Agents", "n=create  e=edit  d=delete  r=refresh", m.agentsList.View(), keys.listHelp())
		return v
	case routeTools:
		v.Content = m.renderListScreen("Tools", "r=refresh", m.toolsList.View(), keys.listHelp())
		return v
	case routeProjects:
		v.Content = m.renderListScreen("Projects", "r=refresh", m.projectsList.View(), keys.listHelp())
		return v

	// ── full-screen table routes ─────────────────────────────────────────────
	case routeSessions:
		v.Content = m.renderTableScreen("Sessions", "↵=resume  d=delete  r=refresh", m.sessionsTable.View())
		return v
	case routeProviders:
		v.Content = m.renderTableScreen("Providers", "↵=manage keys  n=add  s=strategy  r=refresh", m.providersTable.View())
		return v
	case routeVoice:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Voice Providers  (%d)", len(m.voiceData)),
			"n=add  e=edit  d=delete  r=refresh",
			m.voiceTable.View(),
		)
		return v
	case routeMedia:
		v.Content = m.renderTableScreen("Media Providers", "r=refresh", m.mediaTable.View())
		return v
	case routeModels:
		discovered := m.api.listDiscoveredModels()
		hint := "↵=select/deselect  r=refresh  /discovered=new"
		if len(discovered) > 0 {
			hint += fmt.Sprintf("  !! %d new", len(discovered))
		}
		body := m.modelsTable.View()
		if summary := steps.DiscoveredSummary(len(discovered)); summary != "" {
			body = summary + "\n" + body
		}
		v.Content = m.renderTableScreen("Models Hub", hint, body)
		return v
	case routeWorkers:
		v.Content = m.renderWorkersView()
		return v
	case routeTasks:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Tasks  (%d)", len(m.tasksData)),
			"n=new  d=cancel  r=refresh",
			m.tasksTable.View(),
		)
		return v
	case routeCron:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Cron Jobs  (%d)", len(m.cronData)),
			"n=new  ↵=toggle  d=delete  r=refresh",
			m.cronTable.View(),
		)
		return v
	case routeMCP:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("MCP Servers  (%d)", len(m.mcpData)),
			"n=add  d=delete  r=refresh",
			m.mcpTable.View(),
		)
		return v
	case routeRooms:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Rooms / Hubs  (%d)", len(m.roomsData)),
			"n=new  d=delete  r=refresh",
			m.roomsTable.View(),
		)
		return v
	case routeChannels:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Channels  (%d)", len(m.channelsData)),
			"n=new  e=edit  ↵=start/stop  d=delete  r=refresh",
			m.channelsTable.View(),
		)
		return v
	case routeSkills:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Skills  (%d)", len(m.skillsData)),
			"i=marketplace  d=delete  r=refresh",
			m.skillsTable.View(),
		)
		return v
	case routePlans:
		pending := 0
		for _, p := range m.plansData {
			if p.Status == "pending" {
				pending++
			}
		}
		title := fmt.Sprintf("Plans  (%d)", len(m.plansData))
		hint := "↵=approve  d=reject  r=refresh"
		if pending > 0 {
			title += fmt.Sprintf("  · !! %d pending", pending)
		}
		v.Content = m.renderTableScreen(title, hint, m.plansTable.View())
		return v
	case routeMemory:
		title := "Memory"
		hint := "↵=paste to chat  r=refresh  · /memory <query> to search"
		if m.memoryQuery != "" {
			title += "  · " + m.memoryQuery
			hint = "↵=paste to chat  r=refresh"
		} else {
			title += "  (recent)"
		}
		v.Content = m.renderTableScreen(
			fmt.Sprintf("%s  (%d results)", title, len(m.memoryData)),
			hint,
			m.memoryTable.View(),
		)
		return v
	case routeDrive:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Drive  (%d files)", len(m.driveData)),
			"u=upload  d=delete  r=refresh",
			m.driveTable.View(),
		)
		return v
	case routeDriveUpload:
		v.Content = m.renderListScreen("Upload File", "↵=select  Esc=back", m.driveUploadPicker.View(), []key.Binding{keys.Back})
		return v
	case routeDiscovered:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Discovered Models  (%d new)", len(m.discoveredData)),
			"↵=add  d=ignore  r=refresh",
			m.discoveredTable.View(),
		)
		return v
	case routeSupervisor:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Supervisor  (%d escalation(s))", len(m.escalationsData)),
			"↵=approve  d=reject  r=refresh",
			m.supervisorTable.View(),
		)
		return v
	case routeRouter:
		v.Content = m.renderRouterView()
		return v
	case routeSettings:
		v.Content = m.renderTableScreen("Settings", "n=set integration key  r=refresh", m.settingsTable.View())
		return v
	case routeWorkflows:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Workflows  (%d)", len(m.workflowsData)),
			"n=new  ↵=run  d=delete  r=refresh",
			m.workflowsTable.View(),
		)
		return v
	case routeGitHub:
		v.Content = m.renderGitHubView()
		return v

	case routeHome:
		v.Content = m.renderHomeScreen()
		return v
	case routeVault:
		v.Content = m.renderTableScreen(
			fmt.Sprintf("Vault  (%d entries)", len(m.vaultData)),
			"n=new  d=delete  r=refresh",
			m.vaultTable.View(),
		)
		return v
	case routeUsage:
		v.Content = m.renderUsageScreen()
		return v
	case routeNotifications:
		unread := 0
		for _, n := range m.notificationsData {
			if !n.Read {
				unread++
			}
		}
		title := fmt.Sprintf("Notifications  (%d total, %d unread)", len(m.notificationsData), unread)
		v.Content = m.renderTableScreen(title, "↵=mark read  r=refresh", m.notificationsTable.View())
		return v
	case routeProviderKeys:
		title := fmt.Sprintf("Keys — %s  (%d keys)", m.providerKeysID, len(m.providerKeysData.Keys))
		hint := fmt.Sprintf("pool: %s · failover: %s  |  n=add key  ↵=set budget  t=test  d=delete  r=refresh", m.providerKeysData.Strategy, m.providerKeysData.FailoverMode)
		v.Content = m.renderTableScreen(title, hint, m.providerKeysTable.View())
		return v
	case routeRoomChat:
		v.Content = m.renderRoomChatScreen()
		return v

	// ── forms ────────────────────────────────────────────────────────────────
	case routeFormProvider, routeFormKey, routeFormAgent, routeFormChannel,
		routeFormCron, routeFormMCP, routeFormRoom, routeFormTask,
		routeFormKeyBudget, routeFormPoolStrategy, routeFormAgentEdit,
		routeFormIntegration, routeFormWorkflow, routeFormVoice, routeFormVault,
		routeFormChannelEdit, routeFormVoiceEdit:
		v.Content = m.renderFormScreen()
		return v

	// ── pickers ──────────────────────────────────────────────────────────────
	case routeModelPicker:
		v.Content = m.renderPickerScreen("Choose a Model", m.modelPicker.View())
		return v
	case routeAgentPicker:
		v.Content = m.renderPickerScreen("Switch Agent", m.agentPicker.View())
		return v
	case routeSkillMarket:
		v.Content = m.renderPickerScreen("Install Skill", m.skillMarketPicker.View())
		return v
	case routeHelp:
		v.Content = m.renderPickerScreen("Commands", m.helpList.View())
		return v
	case routeCode:
		v.Content = m.renderCodeView()
		return v
	}

	// ── chat (default) ───────────────────────────────────────────────────────
	sideW := 0
	if m.sidebarOpen && m.width > 90 {
		sideW = 40
	}
	mainW := m.width - sideW

	divLine := dividerStyle.Render(strings.Repeat("─", mainW))

	prompt := lipgloss.NewStyle().Foreground(purple).Bold(true).Render(" ❯ ")
	input := prompt + m.textarea.View()

	statusLine := m.renderStatusBar(mainW)

	var popup string
	if m.slashPopupOpen {
		popup = m.renderSlashPopup()
	} else if m.mentionPopupOpen {
		popup = m.renderMentionPopup()
	}

	pills := m.renderProgressPills(mainW)

	popupLines := 0
	if popup != "" {
		popupLines = strings.Count(popup, "\n") + 1
	}
	pillLines := 0
	if pills != "" {
		pillLines = 1
	}
	_, chatH := m.chatDimensions()
	vpH := chatH - popupLines - pillLines
	if vpH < 1 {
		vpH = 1
	}
	m.viewport.SetHeight(vpH)

	chat := m.viewport.View()

	mainPane := lipgloss.JoinVertical(lipgloss.Left, chat, divLine, popup, pills, input, statusLine)

	if sideW == 0 {
		v.Content = mainPane
		return v
	}

	sidebar := m.renderSidebar(sideW, m.height)
	v.Content = lipgloss.JoinHorizontal(lipgloss.Top, mainPane, sidebar)
	return v
}

// ─────────────────────────────────────────────────────────────────────────────
// Banner
// ─────────────────────────────────────────────────────────────────────────────

func renderBanner(width int) string {
	if width < 10 {
		return logoStyle.Render(" QORVEN") + "\n"
	}

	hatch := strings.Repeat("╱", width)
	hatchLine := hatchStyle.Render(hatch)

	brand := brandStyle.Render("Qorven™")
	logo1 := logoStyle.Render(logoLine1)
	logo2 := logoStyle.Render(logoLine2)
	logo3 := logoStyle.Render(logoLine3)

	logoW := logoVisualWidth
	rightPad := width - logoW - 1
	if rightPad < 0 {
		rightPad = 0
	}
	rightHatch := hatchStyle.Render(strings.Repeat("╱", rightPad))

	l1 := logo1 + rightHatch
	l2 := logo2 + rightHatch
	l3 := logo3 + rightHatch
	_ = brand

	return strings.Join([]string{hatchLine, l1, l2, l3, hatchLine}, "\n")
}

func renderSidebarBanner(width int) string {
	if width < 8 {
		return logoStyle.Render(" Q\n") + "\n"
	}
	hatch := strings.Repeat("╱", width)
	h := hatchStyle.Render(hatch)
	brand := brandStyle.Render("Qorven™")
	logo := logoStyle.Render(" QORVEN")
	mid := fmt.Sprintf("%-*s", width, brand)
	return strings.Join([]string{h, mid, logo, h}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
// Screen templates
// ─────────────────────────────────────────────────────────────────────────────

func (m Model) renderListScreen(title, hint, body string, bindings []key.Binding) string {
	banner := renderBanner(m.width)
	titleLine := " " + headerStyle.Render(title)
	if hint != "" {
		titleLine += "  " + dimStyle.Render(hint)
	}
	div := dividerStyle.Render(strings.Repeat("─", m.width))
	help := " " + m.help.ShortHelpView(bindings)
	return strings.Join([]string{banner, titleLine, div, body, div, help}, "\n")
}

func (m Model) renderTableScreen(title, hint, body string) string {
	banner := renderBanner(m.width)
	titleLine := " " + headerStyle.Render(title)
	if hint != "" {
		titleLine += "  " + dimStyle.Render(hint)
	}
	div := dividerStyle.Render(strings.Repeat("─", m.width))
	helpLine := " " + m.help.ShortHelpView([]key.Binding{keys.Up, keys.Down, keys.Select, keys.Delete, keys.Refresh, keys.Back})
	return strings.Join([]string{banner, titleLine, div, body, div, helpLine}, "\n")
}

func (m Model) renderPickerScreen(title, body string) string {
	banner := renderBanner(m.width)
	titleLine := " " + headerStyle.Render(title)
	div := dividerStyle.Render(strings.Repeat("─", m.width))
	help := " " + dimStyle.Render("↑/↓ choose · enter accept · esc back")
	return strings.Join([]string{banner, titleLine, div, "", body, "", div, help}, "\n")
}

func (m Model) renderFormScreen() string {
	banner := renderBanner(m.width)
	div := dividerStyle.Render(strings.Repeat("─", m.width))
	return banner + "\n" + div + "\n" + m.form.View()
}
