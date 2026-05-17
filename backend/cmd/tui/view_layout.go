// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// ─────────────────────────────────────────────────────────────────────────────
// Layout
// ─────────────────────────────────────────────────────────────────────────────

func (m *Model) updateLayout() {
	if !m.ready {
		return
	}
	chatW, chatH := m.chatDimensions()
	m.viewport = viewport.New(viewport.WithWidth(chatW-2), viewport.WithHeight(chatH))
	m.textarea.SetWidth(chatW - 6)
	m.help.SetWidth(m.width)
	m.buildProgress.SetWidth(chatW - 20)
	m.updateViewport()

	w, h := m.listDimensions()
	if m.agentsList.Items() != nil {
		m.agentsList.SetSize(w, h)
	}
	if m.toolsList.Items() != nil {
		m.toolsList.SetSize(w, h)
	}
	if m.projectsList.Items() != nil {
		m.projectsList.SetSize(w, h)
	}
	m.sessionsTable.SetWidth(w)
	m.sessionsTable.SetHeight(h)
	m.providersTable.SetWidth(w)
	m.providersTable.SetHeight(h)
	m.voiceTable.SetWidth(w)
	m.voiceTable.SetHeight(h)
	m.mediaTable.SetWidth(w)
	m.mediaTable.SetHeight(h)
	m.workersAgentTable.SetWidth(w)
	m.workersAgentTable.SetHeight(h)
	m.workersPlanTable.SetWidth(w)
	m.workersPlanTable.SetHeight(h)
	m.tasksTable.SetWidth(w)
	m.tasksTable.SetHeight(h)
	m.cronTable.SetWidth(w)
	m.cronTable.SetHeight(h)
	m.mcpTable.SetWidth(w)
	m.mcpTable.SetHeight(h)
	m.roomsTable.SetWidth(w)
	m.roomsTable.SetHeight(h)
	m.channelsTable.SetWidth(w)
	m.channelsTable.SetHeight(h)
	m.skillsTable.SetWidth(w)
	m.skillsTable.SetHeight(h)
	m.plansTable.SetWidth(w)
	m.plansTable.SetHeight(h)
	m.memoryTable.SetWidth(w)
	m.memoryTable.SetHeight(h)
	m.driveTable.SetWidth(w)
	m.driveTable.SetHeight(h)
	m.discoveredTable.SetWidth(w)
	m.discoveredTable.SetHeight(h)
	m.supervisorTable.SetWidth(w)
	m.supervisorTable.SetHeight(h)
	m.routerTable.SetWidth(w)
	m.routerTable.SetHeight(h)
	m.settingsTable.SetWidth(w)
	m.settingsTable.SetHeight(h)
	m.workflowsTable.SetWidth(w)
	m.workflowsTable.SetHeight(h)
	m.modelsTable.SetWidth(w)
	m.modelsTable.SetHeight(h)
	m.homeSessionTable.SetWidth(w)
	m.homeSessionTable.SetHeight(h)
	m.vaultTable.SetWidth(w)
	m.vaultTable.SetHeight(h)
	m.usageTable.SetWidth(w)
	m.usageTable.SetHeight(h)
	m.notificationsTable.SetWidth(w)
	m.notificationsTable.SetHeight(h)
	m.providerKeysTable.SetWidth(w)
	m.providerKeysTable.SetHeight(h)
	m.githubPRTable.SetWidth(w)
	m.githubPRTable.SetHeight(h)
	m.githubIssueTable.SetWidth(w)
	m.githubIssueTable.SetHeight(h)
	m.githubTaskTable.SetWidth(w)
	m.githubTaskTable.SetHeight(h)
}

func (m *Model) chatDimensions() (w, h int) {
	sideW := 0
	if m.sidebarOpen && m.width > 90 {
		sideW = 40
	}
	w = m.width - sideW
	if w < 20 {
		w = 20
	}
	// banner (5 lines) + prompt (1) + divider (1) + status (1) = 8
	h = m.height - m.textarea.Height() - 5
	if h < 3 {
		h = 3
	}
	return w, h
}

func (m *Model) listDimensions() (w, h int) {
	w = m.width
	if w < 40 {
		w = 40
	}
	// banner (~5) + title (1) + div (1) + help (1) = 8
	h = m.height - 8
	if h < 10 {
		h = 10
	}
	return w, h
}

// ─────────────────────────────────────────────────────────────────────────────
// Viewport (chat message rendering)
// ─────────────────────────────────────────────────────────────────────────────

func (m *Model) updateViewport() {
	var sb strings.Builder
	contentW := m.viewport.Width() - 3
	if contentW < 20 {
		contentW = 20
	}

	// Historical messages from previous sessions
	if m.historyReady && len(m.historyMsgs) > 0 {
		prevDiscID := ""
		for _, hmsg := range m.historyMsgs {
			// Show discussion separator on topic change
			if hmsg.DiscussionID != prevDiscID && prevDiscID != "" {
				disc := m.findDiscussion(hmsg.DiscussionID)
				label := disc.AILabel
				if disc.UserLabel != "" {
					label = disc.UserLabel
				}
				if label == "" {
					label = "discussion"
				}
				sep := fmt.Sprintf("── %s ", label)
				sb.WriteString("\n " + dimStyle.Render(sep) + "\n")
			}
			prevDiscID = hmsg.DiscussionID

			// Channel prefix
			chPrefix := channelPrefix(hmsg.SourceChannel)

			switch hmsg.Role {
			case "user":
				sb.WriteString("\n " + userLabel.Render("  You") + "\n")
				content := chPrefix + hmsg.Content
				for _, line := range wrapText(content, contentW) {
					sb.WriteString("   " + dimStyle.Render(line) + "\n")
				}
			default:
				sb.WriteString("\n " + dimStyle.Render("  "+m.agentName) + "\n")
				for _, line := range wrapText(hmsg.Content, contentW) {
					sb.WriteString("   " + dimStyle.Render(line) + "\n")
				}
			}
		}
		// Separator before current session
		sb.WriteString("\n " + dimStyle.Render("── current session ") + "\n\n")
	}

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString("\n " + userLabel.Render("  You") + "\n")
			for _, raw := range strings.Split(msg.Content, "\n") {
				for _, line := range wrapText(raw, contentW) {
					sb.WriteString("   " + line + "\n")
				}
			}
		case "system":
			sb.WriteString("\n " + systemLabel.Render("  System") + "\n")
			for _, line := range wrapText(msg.Content, contentW) {
				sb.WriteString("   " + dimStyle.Render(line) + "\n")
			}
		default:
			sb.WriteString("\n " + agentLabel.Render("  "+m.agentName) + " " + dimStyle.Render(m.modelName) + "\n")
			for _, t := range msg.ToolEvents {
				sb.WriteString(renderToolCall(t, contentW) + "\n")
			}
			if len(msg.ToolEvents) == 0 {
				for _, t := range msg.Tools {
					sb.WriteString("   " + toolStyle.Render("⎔ "+t) + "\n")
				}
			}
			for _, w := range msg.Widgets {
				sb.WriteString(renderWidgetRef(w, contentW) + "\n")
			}
			if msg.Content != "" {
				rendered := renderContent(msg.Content, contentW)
				for _, rawLine := range strings.Split(rendered, "\n") {
					for _, line := range wrapText(rawLine, contentW) {
						sb.WriteString("   " + line + "\n")
					}
				}
			}
		}
	}

	if m.isStreaming {
		elapsed := m.streamTimer.Elapsed()
		var elapsedStr string
		if elapsed > 0 {
			if m.thinkingLevel != "" && m.thinkingLevel != "off" {
				elapsedStr = "  " + dimStyle.Render("Thought for "+formatDuration(elapsed))
			} else {
				elapsedStr = "  " + dimStyle.Render(formatDuration(elapsed))
			}
		}
		sb.WriteString("\n " + agentLabel.Render("  "+m.agentName) + " " + m.spinner.View() + elapsedStr + "\n")
		if m.streaming != "" {
			for _, line := range wrapText(m.streaming, contentW) {
				sb.WriteString("   " + line + "\n")
			}
		}
	}

	if len(m.messages) == 0 && !m.isStreaming {
		connHint := ""
		if m.gatewayOnline {
			connHint = "  " + lipgloss.NewStyle().Foreground(green).Render("●") + " " + dimStyle.Render("connected")
		} else {
			connHint = "  " + lipgloss.NewStyle().Foreground(red).Render("●") + " " + dimStyle.Render("offline — check QORVEN_SERVER")
		}
		sb.WriteString("\n\n" + dimStyle.Render("   Type a message or / for commands. Press ctrl+b to toggle sidebar.") + connHint + "\n")
		sb.WriteString("   " + dimStyle.Render("c=copy  e=expand tool  ctrl+r=regenerate  ↑=edit last  @=mention agent") + "\n")
	}

	m.viewport.SetContent(sb.String())
	m.viewport.GotoBottom()
}

// findDiscussion returns the TUIDiscussion with the given ID, or a zero value.
func (m *Model) findDiscussion(id string) TUIDiscussion {
	for _, d := range m.discussions {
		if d.ID == id {
			return d
		}
	}
	return TUIDiscussion{}
}

// channelPrefix returns a short bracketed prefix for non-TUI channels.
func channelPrefix(ch string) string {
	switch ch {
	case "telegram":
		return "[TG] "
	case "whatsapp":
		return "[WA] "
	case "tui", "":
		return ""
	case "email":
		return "[✉] "
	default:
		return ""
	}
}
