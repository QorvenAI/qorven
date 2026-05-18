// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleSlashCommand(text string) (bool, tea.Cmd) {
	switch text {
	case "/clear":
		m.messages = nil
		m.updateViewport()
		return true, nil

	case "/model", "/models":
		m.openModelPicker()
		return true, nil

	case "/agent":
		m.openAgentPicker()
		return true, nil

	case "/agents":
		m.enterAgentsList()
		return true, nil

	case "/sessions":
		m.enterSessionsTable()
		return true, nil

	case "/providers":
		m.enterProvidersTable()
		return true, nil

	case "/voice":
		m.enterVoiceTable()
		return true, nil

	case "/media":
		m.enterMediaTable()
		return true, nil

	case "/modelshub", "/mh":
		m.enterModelsTable()
		return true, nil

	case "/discovered", "/discover":
		m.enterDiscoveredTable()
		return true, nil

	case "/workers", "/daemon", "/w":
		m.enterWorkersTable()
		return true, nil

	case "/tasks":
		m.enterTasksTable("")
		return true, nil

	case "/cron":
		m.enterCronTable()
		return true, nil

	case "/mcp":
		m.enterMCPTable()
		return true, nil

	case "/rooms", "/hubs":
		m.enterRoomsTable()
		return true, nil

	case "/channels":
		m.enterChannelsTable()
		return true, nil

	case "/skills":
		m.enterSkillsTable()
		return true, nil

	case "/plans":
		m.enterPlansTable()
		return true, nil

	case "/drive":
		m.enterDriveTable()
		return true, nil

	case "/github", "/gh", "/prs", "/issues":
		m.enterGitHubScreen()
		return true, nil

	case "/provider add", "/providers add":
		m.enterFormProvider()
		return true, nil

	case "/key add", "/keys add":
		m.enterFormKey()
		return true, nil

	case "/agent new", "/agents new":
		m.enterFormAgent()
		return true, nil

	case "/channel new", "/channels new":
		m.enterFormChannel()
		return true, nil

	case "/cron new":
		m.enterFormCron()
		return true, nil

	case "/mcp add":
		m.enterFormMCP()
		return true, nil

	case "/room new", "/rooms new":
		m.enterFormRoom()
		return true, nil

	case "/task new", "/tasks new":
		m.enterFormTask()
		return true, nil

	case "/supervisor":
		m.enterSupervisorTable()
		return true, nil

	case "/router", "/routing":
		m.enterRouterTable()
		return true, nil

	case "/settings", "/setup":
		m.enterSettingsTable()
		return true, nil

	case "/workflows", "/workflow":
		m.enterWorkflowsTable()
		return true, nil

	case "/workflow new", "/workflows new":
		m.enterFormWorkflow()
		return true, nil

	case "/voice add", "/voices add":
		m.enterFormVoice()
		return true, nil

	case "/tools":
		m.enterToolsList()
		return true, nil

	case "/project", "/projects":
		m.enterProjectsList()
		return true, nil

	case "/help":
		m.openHelp()
		return true, nil

	case "/new":
		m.messages = []ChatMessage{{
			Role:    "system",
			Content: "Cleared local view. Canonical chat (server-side) is intact — your Qor still remembers.",
		}}
		m.updateViewport()
		return true, nil

	case "/compact":
		m.sidebarOpen = !m.sidebarOpen
		m.updateLayout()
		return true, nil

	case "/status":
		status := m.api.getStatus()
		var parts []string
		for k, v := range status {
			parts = append(parts, k+": "+v)
		}
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: strings.Join(parts, " | ")})
		m.updateViewport()
		return true, nil

	case "/keys":
		m.messages = append(m.messages, ChatMessage{
			Role:    "system",
			Content: "To manage provider keys: use /providers then press Enter on a provider row.",
		})
		for _, p := range m.api.listProviders() {
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "  " + p.Name + " (" + p.Type + ") — " + p.APIBase})
		}
		m.updateViewport()
		return true, nil

	case "/code":
		m.route = routeCode
		if m.codeProject == "" {
			m.codeProject = "."
		}
		m.codeFiles = listProjectFiles(m.codeProject)
		m.codeCursor = 0
		m.codePicker = newCodePicker(m.codeProject, m.codeViewHeight())
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Code mode — project: " + m.codeProject})
		m.updateViewport()
		return true, m.codePicker.Init()

	case "/whoami":
		m.messages = append(m.messages, ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("Agent: %s | Model: %s | Session: %s", m.agentID, m.modelName, m.sessionID),
		})
		m.updateViewport()
		return true, nil

	case "/tokens":
		chars := 0
		for _, msg := range m.messages {
			chars += len(msg.Content)
		}
		est := chars / 4
		m.messages = append(m.messages, ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("Messages: %d · Chars: %d · ~%d tokens · Session: %s", len(m.messages), chars, est, m.sessionID),
		})
		m.updateViewport()
		return true, nil

	case "/reset":
		m.messages = nil
		m.sessionID = ""
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Session reset. Starting fresh."})
		m.updateViewport()
		return true, nil

	case "/memory":
		m.enterMemoryTable("")
		return true, nil

	case "/approve":
		m.enterPlansTable()
		return true, nil

	case "/deny":
		m.enterPlansTable()
		return true, nil

	case "/team":
		m.enterAgentsList()
		return true, nil
	case "/mail":
		return true, m.queueAgentPrompt("Check my email inbox and summarise any unread messages")
	case "/notifications":
		m.enterNotificationsTable()
		return true, nil
	case "/usage", "/budget":
		m.enterUsageTable()
		return true, nil
	case "/home":
		m.enterHomeScreen()
		return true, nil

	case "/research":
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Usage: /research <topic>  — sends research prompt to agent"})
		m.textarea.SetValue("/research ")
		m.textarea.CursorEnd()
		m.updateViewport()
		return true, nil
	case "/read":
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Usage: /read <url>  — fetches and summarises a URL"})
		m.textarea.SetValue("/read ")
		m.textarea.CursorEnd()
		m.updateViewport()
		return true, nil
	case "/vault":
		m.enterVaultTable()
		return true, nil
	case "/scan":
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Usage: /scan <text>  — scans for prompt-injection patterns"})
		m.textarea.SetValue("/scan ")
		m.textarea.CursorEnd()
		m.updateViewport()
		return true, nil
	case "/export":
		return true, m.exportSessionToFile()
	case "/import":
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Usage: /import <path>  — imports a session JSON file"})
		m.textarea.SetValue("/import ")
		m.textarea.CursorEnd()
		m.updateViewport()
		return true, nil
	case "/config":
		m.enterSettingsTable()
		return true, nil

	case "/search":
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Usage: /search <keyword> — searches message history"})
		m.updateViewport()
		return true, nil
	}
	return false, nil
}

func (m *Model) queueAgentPrompt(prompt string) tea.Cmd {
	m.messages = append(m.messages, ChatMessage{Role: "user", Content: prompt})
	m.isStreaming = true
	m.streaming = ""
	ctx, cancel := context.WithCancel(context.Background())
	m.streamCancel = cancel
	m.updateViewport()
	return tea.Batch(m.sendMessageCtx(ctx, prompt), m.streamTimer.Reset(), m.streamTimer.Start())
}

func (m *Model) runHistorySearch(term string) {
	if term == "" {
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Usage: /search <keyword>"})
		return
	}
	var hits []string
	for i, msg := range m.messages {
		if strings.Contains(strings.ToLower(msg.Content), strings.ToLower(term)) {
			preview := msg.Content
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			hits = append(hits, fmt.Sprintf("[%d] %s: %s", i+1, msg.Role, preview))
		}
	}
	if len(hits) == 0 {
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: fmt.Sprintf("No messages contain %q", term)})
		return
	}
	m.messages = append(m.messages, ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("Found %d messages containing %q:\n%s", len(hits), term, strings.Join(hits, "\n")),
	})
}
