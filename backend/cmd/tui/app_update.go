// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/stopwatch"
	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case stopwatch.TickMsg, stopwatch.StartStopMsg, stopwatch.ResetMsg:
		var cmd tea.Cmd
		m.streamTimer, cmd = m.streamTimer.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		bp, cmd := m.buildProgress.Update(msg)
		m.buildProgress = bp
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.updateLayout()
		return m, nil

	case buildPhaseMsg:
		m.buildPhase = msg.phase
		if frac, ok := buildPhaseFraction[msg.phase]; ok {
			return m, m.buildProgress.SetPercent(frac)
		}
		return m, nil

	case codeFileLoadedMsg:
		m.codePath = msg.path
		m.codeContent = msg.content
		return m, nil

	case codeFilesListedMsg:
		m.codeFiles = msg.files
		m.codeCursor = 0
		return m, nil

	case streamDoneMsg:
		m.isStreaming = false
		content := strings.TrimSpace(msg.content)
		if content == "" && len(msg.widgets) == 0 {
			content = "(no response)"
		}
		var toolNames []string
		for _, t := range msg.tools {
			toolNames = append(toolNames, t.name)
		}
		m.messages = append(m.messages, ChatMessage{
			Role: "assistant", Content: content, Tools: toolNames, ToolEvents: msg.tools,
			Widgets: msg.widgets,
		})
		m.streaming = ""
		m.updateViewport()
		go saveSession(m.sessionID, m.agentID, m.agentName, m.modelName, m.messages) //nolint:errcheck
		return m, m.streamTimer.Stop()

	case streamErrorMsg:
		m.isStreaming = false
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Error: " + msg.err})
		m.streaming = ""
		m.updateViewport()
		return m, m.streamTimer.Stop()

	case homeLoadedMsg:
		m.homeData = msg.data
		m.homeSessionTable = m.newHomeSessionTable()
		return m, nil

	case pingResultMsg:
		m.lastPingOK = msg.ok
		m.gatewayOnline = msg.ok
		m.gatewayLatency = msg.latency
		return m, nil

	case agentHistoryLoadedMsg:
		if msg.agentID == m.agentID {
			m.historyMsgs = msg.messages
			m.discussions = msg.discussions
			m.historyReady = true
			m.updateViewport()
		}
		return m, nil

	case roomMessagesLoadedMsg:
		if msg.roomID == m.roomChatID {
			m.roomMessages = msg.messages
			content := renderRoomMessages(m.roomMessages, m.roomChatViewport.Width()-2)
			m.roomChatViewport.SetContent(content)
			m.roomChatViewport.GotoBottom()
		}
		return m, nil

	case vaultLoadedMsg:
		m.vaultData = msg.entries
		m.vaultTable = m.newVaultTable()
		return m, nil

	case usageLoadedMsg:
		m.usageData = msg.data
		m.usageTable = m.newUsageTable()
		return m, nil

	case notificationsLoadedMsg:
		m.notificationsData = msg.data
		m.notificationsTable = m.newNotificationsTable()
		return m, nil

	case providerKeysLoadedMsg:
		if msg.providerID == m.providerKeysID {
			m.providerKeysData = msg.pool
			m.providerKeysTable = m.newProviderKeysTable()
		}
		return m, nil

	case pingTickMsg:
		return m, tea.Batch(m.pingGatewayAsync(), m.scheduleNextPing())

	case rtConnectedMsg:
		m.rtSubActive = true
		return m, m.subscribeRealTime()

	case realTimeMsg:
		switch msg.kind {
		case "channel_message", "notification":
			badge := "📨"
			if msg.kind == "notification" {
				badge = "🔔"
				m.homeData.UnreadNotifications++
			}
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: badge + " " + msg.payload,
			})
			m.updateViewport()
		case "plan_pending":
			m.homeData.PendingPlans++
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: "!! New plan pending approval — /plans to review",
			})
			m.updateViewport()
		case "task_update":
			m.homeData.ActiveTasks++
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: "⚙ " + msg.payload,
			})
			m.updateViewport()
		case "task_iteration_start":
			m.taskCount++
		case "task_done", "task_blocked":
			if m.taskCount > 0 {
				m.taskCount--
			}
		}
		return m, m.subscribeRealTime()
	}

	if wmsg, ok := msg.(tea.MouseWheelMsg); ok && m.route == routeChat {
		w := wmsg.Mouse()
		down := w.Button == tea.MouseWheelDown
		switch {
		case w.Mod&tea.ModCtrl != 0:
			if down {
				m.viewport.GotoBottom()
			} else {
				m.viewport.GotoTop()
			}
		case w.Mod&tea.ModShift != 0:
			if down {
				m.viewport.ScrollDown(5)
			} else {
				m.viewport.ScrollUp(5)
			}
		default:
			if down {
				m.viewport.ScrollDown(1)
			} else {
				m.viewport.ScrollUp(1)
			}
		}
		return m, nil
	}

	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if cmd := m.handleKey(kmsg); cmd != nil || m.consumedKey {
			m.consumedKey = false
			return m, cmd
		}
	}

	var cmd tea.Cmd
	switch m.route {
	case routeAgents:
		m.agentsList, cmd = m.agentsList.Update(msg)
		cmds = append(cmds, cmd)
	case routeTools:
		m.toolsList, cmd = m.toolsList.Update(msg)
		cmds = append(cmds, cmd)
	case routeProjects:
		m.projectsList, cmd = m.projectsList.Update(msg)
		cmds = append(cmds, cmd)
	case routeSessions:
		m.sessionsTable, cmd = m.sessionsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeProviders:
		m.providersTable, cmd = m.providersTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeModelPicker:
		m.modelPicker, cmd = m.modelPicker.Update(msg)
		cmds = append(cmds, cmd)
	case routeAgentPicker:
		m.agentPicker, cmd = m.agentPicker.Update(msg)
		cmds = append(cmds, cmd)
	case routeSkillMarket:
		m.skillMarketPicker, cmd = m.skillMarketPicker.Update(msg)
		cmds = append(cmds, cmd)
	case routeHelp:
		m.helpList, cmd = m.helpList.Update(msg)
		cmds = append(cmds, cmd)
	case routeVoice:
		m.voiceTable, cmd = m.voiceTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeMedia:
		m.mediaTable, cmd = m.mediaTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeModels:
		m.modelsTable, cmd = m.modelsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeWorkers:
		switch m.workersSubView {
		case 0:
			m.workersAgentTable, cmd = m.workersAgentTable.Update(msg)
		case 1:
			m.tasksTable, cmd = m.tasksTable.Update(msg)
		case 2:
			m.workersPlanTable, cmd = m.workersPlanTable.Update(msg)
		}
		cmds = append(cmds, cmd)
	case routeTasks:
		m.tasksTable, cmd = m.tasksTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeCron:
		m.cronTable, cmd = m.cronTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeMCP:
		m.mcpTable, cmd = m.mcpTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeRooms:
		m.roomsTable, cmd = m.roomsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeChannels:
		m.channelsTable, cmd = m.channelsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeSkills:
		m.skillsTable, cmd = m.skillsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routePlans:
		m.plansTable, cmd = m.plansTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeMemory:
		m.memoryTable, cmd = m.memoryTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeDrive:
		m.driveTable, cmd = m.driveTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeDiscovered:
		m.discoveredTable, cmd = m.discoveredTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeSupervisor:
		m.supervisorTable, cmd = m.supervisorTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeRouter:
		m.routerTable, cmd = m.routerTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeSettings:
		m.settingsTable, cmd = m.settingsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeWorkflows:
		m.workflowsTable, cmd = m.workflowsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeGitHub:
		switch m.githubSubView {
		case 0:
			m.githubPRTable, cmd = m.githubPRTable.Update(msg)
		case 1:
			m.githubIssueTable, cmd = m.githubIssueTable.Update(msg)
		case 2:
			m.githubTaskTable, cmd = m.githubTaskTable.Update(msg)
		}
		cmds = append(cmds, cmd)
	case routeDriveUpload:
		m.driveUploadPicker, cmd = m.driveUploadPicker.Update(msg)
		cmds = append(cmds, cmd)
		if selected, path := m.driveUploadPicker.DidSelectFile(msg); selected {
			if err := m.api.uploadDriveFile(path); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Uploaded: " + path})
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Upload failed: " + err.Error()})
			}
			m.route = routeDrive
			m.driveData = m.api.listDriveFiles()
			m.driveTable = m.newDriveTable()
		}
	case routeFormProvider, routeFormKey, routeFormAgent, routeFormChannel,
		routeFormCron, routeFormMCP, routeFormRoom, routeFormTask,
		routeFormKeyBudget, routeFormPoolStrategy, routeFormAgentEdit,
		routeFormIntegration, routeFormWorkflow, routeFormVoice, routeFormVault,
		routeFormChannelEdit, routeFormVoiceEdit:
		var formCmd tea.Cmd
		m.form, formCmd = m.form.Update(msg)
		cmds = append(cmds, formCmd)
		if m.form.aborted {
			m.route = m.formReturn
			m.refreshActiveList()
		} else if m.form.done {
			m.handleFormSubmit()
		}
	case routeHome:
		m.homeSessionTable, cmd = m.homeSessionTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeVault:
		m.vaultTable, cmd = m.vaultTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeProviderKeys:
		m.providerKeysTable, cmd = m.providerKeysTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeNotifications:
		m.notificationsTable, cmd = m.notificationsTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeUsage:
		m.usageTable, cmd = m.usageTable.Update(msg)
		cmds = append(cmds, cmd)
	case routeRoomChat:
		m.roomChatViewport, cmd = m.roomChatViewport.Update(msg)
		cmds = append(cmds, cmd)
		m.roomChatInput, cmd = m.roomChatInput.Update(msg)
		cmds = append(cmds, cmd)
	case routeCode:
		m.codePicker, cmd = m.codePicker.Update(msg)
		cmds = append(cmds, cmd)
		if selected, path := m.codePicker.DidSelectFile(msg); selected {
			m.codePath = path
			cmds = append(cmds, m.loadFileCmd(path))
		}
	case routeChat:
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
		m.syncSlashPopup()
		m.syncMentionPopup()
		if m.slashPopupOpen {
			m.slashPopup, cmd = m.slashPopup.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.mentionPopupOpen {
			m.mentionPopup, cmd = m.mentionPopup.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}
