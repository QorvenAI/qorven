// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) selectListItem() tea.Cmd {
	switch m.route {
	case routeAgents:
		if a, ok := m.agentsList.SelectedItem().(agentItem); ok {
			m.agentID = a.info.ID
			m.agentName = a.info.Name
			m.historyReady = false
			m.historyMsgs = nil
			m.discussions = nil
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Switched to agent: " + a.info.Name})
			m.route = routeChat
			return m.loadAgentHistoryAsync(a.info.ID)
		}
	case routeProjects:
		if p, ok := m.projectsList.SelectedItem().(projectItem); ok {
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Project: %s · path: %s · phase: %s", p.Title(), p.info.Path, p.info.Phase),
			})
			m.route = routeChat
		}
	case routeTools:
		// Tools are view-only
	}
	return nil
}

func (m *Model) selectTableRow() {
	switch m.route {
	case routeSessions:
		row := m.sessionsTable.SelectedRow()
		if len(row) == 0 {
			return
		}
		idx := m.sessionsTable.Cursor()
		if idx < 0 || idx >= len(m.sessionsData) {
			return
		}
		s := m.sessionsData[idx]
		// Use full session ID stored on struct (FullID).
		if s.FullID != "" {
			m.sessionID = s.FullID
		} else {
			m.sessionID = s.ID
		}
		// Load persisted messages; fall back to a system notice.
		if msgs := loadSession(m.sessionID); len(msgs) > 0 {
			m.messages = msgs
		} else {
			m.messages = []ChatMessage{{Role: "system", Content: "Resumed session: " + m.sessionID}}
		}
		m.updateViewport()
		m.route = routeChat
	case routeProviders:
		// Enter on a provider → open its key pool
		idx := m.providersTable.Cursor()
		if idx >= 0 && idx < len(m.providersData) {
			m.formKeyID = m.providersData[idx].FullID
			m.enterProviderKeysTable(m.providersData[idx].FullID)
		}
	case routeModels:
		idx := m.modelsTable.Cursor()
		if idx >= 0 && idx < len(m.modelsData) {
			e := m.modelsData[idx]
			var err error
			if e.Selected {
				err = m.api.deselectModel(e.ProviderID, e.ModelID)
				if err == nil {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deselected: " + e.ModelID})
				}
			} else {
				err = m.api.selectModel(e.ProviderID, e.ModelID)
				if err == nil {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Selected: " + e.ModelID})
				}
			}
			if err != nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Error: " + err.Error()})
			}
			m.modelsData = m.api.listModelHub()
			m.modelsTable = m.newModelsTable()
		}
	case routeWorkers:
		if m.workersSubView == 2 {
			// Plans tab — approve selected
			idx := m.workersPlanTable.Cursor()
			if idx >= 0 && idx < len(m.workersPlans) {
				plan := m.workersPlans[idx]
				if plan.Status == "pending" {
					if err := m.api.approveDaemonPlan(plan.ID); err == nil {
						m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ Approved plan: " + plan.Title})
					} else {
						m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Approve failed: " + err.Error()})
					}
					m.refreshActiveList()
				}
			}
		}
	case routePlans:
		idx := m.plansTable.Cursor()
		if idx >= 0 && idx < len(m.plansData) {
			p := m.plansData[idx]
			if p.Status == "pending" {
				if err := m.api.approvePlan(p.FullID); err == nil {
					if m.homeData.PendingPlans > 0 {
						m.homeData.PendingPlans--
					}
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ Approved plan: " + p.Title})
				} else {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Approve failed: " + err.Error()})
				}
				m.plansData = m.api.listPlans()
				m.plansTable = m.newPlansTable()
			}
		}
	case routeChannels:
		idx := m.channelsTable.Cursor()
		if idx >= 0 && idx < len(m.channelsData) {
			c := m.channelsData[idx]
			var err error
			if c.Status == "running" {
				err = m.api.stopChannel(c.FullID)
			} else {
				err = m.api.startChannel(c.FullID)
			}
			if err == nil {
				m.channelsData = m.api.listChannels()
				m.channelsTable = m.newChannelsTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Channel action failed: " + err.Error()})
			}
		}
	case routeCron:
		idx := m.cronTable.Cursor()
		if idx >= 0 && idx < len(m.cronData) {
			j := m.cronData[idx]
			if err := m.api.toggleCronJob(j.FullID); err == nil {
				m.cronData = m.api.listCronJobs()
				m.cronTable = m.newCronTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Toggle failed: " + err.Error()})
			}
		}
	case routeDiscovered:
		idx := m.discoveredTable.Cursor()
		if idx >= 0 && idx < len(m.discoveredData) {
			d := m.discoveredData[idx]
			if err := m.api.actionDiscoveredModel(d.ID, "add"); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Added model: " + d.ModelID})
				m.discoveredData = m.api.listDiscoveredModels()
				m.discoveredTable = m.newDiscoveredTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Add failed: " + err.Error()})
			}
		}
	case routeSupervisor:
		idx := m.supervisorTable.Cursor()
		if idx >= 0 && idx < len(m.escalationsData) {
			e := m.escalationsData[idx]
			if e.Status == "pending" {
				if err := m.api.approveEscalation(e.FullID, ""); err == nil {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ Approved escalation: " + e.ID})
				} else {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Approve failed: " + err.Error()})
				}
				m.escalationsData = m.api.listEscalations()
				m.supervisorTable = m.newSupervisorTable()
			}
		}
	case routeWorkflows:
		idx := m.workflowsTable.Cursor()
		if idx >= 0 && idx < len(m.workflowsData) {
			wf := m.workflowsData[idx]
			if err := m.api.runWorkflow(wf.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "▶ Triggered workflow: " + wf.Name})
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Run failed: " + err.Error()})
			}
		}
	case routeNotifications:
		idx := m.notificationsTable.Cursor()
		if idx >= 0 && idx < len(m.notificationsData) {
			n := m.notificationsData[idx]
			if err := m.api.markNotificationRead(n.FullID); err == nil {
				if m.homeData.UnreadNotifications > 0 {
					m.homeData.UnreadNotifications--
				}
				m.notificationsData = m.api.listNotifications()
				m.notificationsTable = m.newNotificationsTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Mark read failed: " + err.Error()})
			}
		}
	case routeRooms:
		// Enter on a room row → open room chat
		idx := m.roomsTable.Cursor()
		if idx >= 0 && idx < len(m.roomsData) {
			r := m.roomsData[idx]
			m.enterRoomChatScreen(r.FullID, r.Name)
		}
	case routeMemory:
		// Enter on a memory row → paste content into chat textarea
		idx := m.memoryTable.Cursor()
		if idx >= 0 && idx < len(m.memoryData) {
			mem := m.memoryData[idx]
			m.textarea.SetValue(mem.Content)
			m.textarea.CursorEnd()
			m.route = routeChat
		}
	case routeProviderKeys:
		// Enter on a key row → set budget (reuse budget form)
		idx := m.providerKeysTable.Cursor()
		if idx >= 0 && idx < len(m.providerKeysData.Keys) {
			k := m.providerKeysData.Keys[idx]
			m.formKeyID = k.ID
			m.enterFormKeyBudget()
		}
	}
}

func (m *Model) deleteListItem() {
	switch m.route {
	case routeAgents:
		if a, ok := m.agentsList.SelectedItem().(agentItem); ok {
			if err := m.api.deleteAgent(a.info.ID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted agent: " + a.info.Name})
				m.enterAgentsList()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Delete failed: " + err.Error()})
			}
		}
	}
}

func (m *Model) deleteTableRow() {
	switch m.route {
	case routeWorkers:
		if m.workersSubView == 2 {
			// Plans tab — reject selected
			idx := m.workersPlanTable.Cursor()
			if idx >= 0 && idx < len(m.workersPlans) {
				plan := m.workersPlans[idx]
				if plan.Status == "pending" {
					if err := m.api.rejectDaemonPlan(plan.ID); err == nil {
						m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Rejected plan: " + plan.Title})
					}
					m.workersPlans = m.api.listDaemonPlans()
					m.workersPlanTable = m.newWorkersPlanTable()
				}
			}
		} else if m.workersSubView == 1 {
			// Tasks tab — cancel selected
			idx := m.tasksTable.Cursor()
			if idx >= 0 && idx < len(m.tasksData) {
				t := m.tasksData[idx]
				if err := m.api.cancelTask(t.FullID); err == nil {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Cancelled task: " + t.Title})
				}
				m.tasksData = m.api.listTasks("")
				m.tasksTable = m.newTasksTable()
			}
		}
	case routePlans:
		idx := m.plansTable.Cursor()
		if idx >= 0 && idx < len(m.plansData) {
			p := m.plansData[idx]
			if p.Status == "pending" {
				if err := m.api.rejectPlan(p.FullID); err == nil {
					if m.homeData.PendingPlans > 0 {
						m.homeData.PendingPlans--
					}
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Rejected plan: " + p.Title})
				}
				m.plansData = m.api.listPlans()
				m.plansTable = m.newPlansTable()
			}
		}
	case routeTasks:
		idx := m.tasksTable.Cursor()
		if idx >= 0 && idx < len(m.tasksData) {
			t := m.tasksData[idx]
			if err := m.api.cancelTask(t.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Cancelled task: " + t.Title})
				m.tasksData = m.api.listTasks("")
				m.tasksTable = m.newTasksTable()
			}
		}
	case routeCron:
		idx := m.cronTable.Cursor()
		if idx >= 0 && idx < len(m.cronData) {
			j := m.cronData[idx]
			if err := m.api.deleteCronJob(j.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted cron job: " + j.ID})
				m.cronData = m.api.listCronJobs()
				m.cronTable = m.newCronTable()
			}
		}
	case routeMCP:
		idx := m.mcpTable.Cursor()
		if idx >= 0 && idx < len(m.mcpData) {
			s := m.mcpData[idx]
			if err := m.api.deleteMCPServer(s.Name); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted MCP server: " + s.Name})
				m.mcpData = m.api.listMCPServers()
				m.mcpTable = m.newMCPTable()
			}
		}
	case routeRooms:
		idx := m.roomsTable.Cursor()
		if idx >= 0 && idx < len(m.roomsData) {
			r := m.roomsData[idx]
			if err := m.api.deleteRoom(r.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted room: " + r.Name})
				m.roomsData = m.api.listRooms()
				m.roomsTable = m.newRoomsTable()
			}
		}
	case routeChannels:
		idx := m.channelsTable.Cursor()
		if idx >= 0 && idx < len(m.channelsData) {
			c := m.channelsData[idx]
			if err := m.api.deleteChannel(c.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted channel: " + c.Name})
				m.channelsData = m.api.listChannels()
				m.channelsTable = m.newChannelsTable()
			}
		}
	case routeSkills:
		idx := m.skillsTable.Cursor()
		if idx >= 0 && idx < len(m.skillsData) {
			s := m.skillsData[idx]
			if err := m.api.deleteSkill(s.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted skill: " + s.Name})
				m.skillsData = m.api.listSkills()
				m.skillsTable = m.newSkillsTable()
			}
		}
	case routeDrive:
		idx := m.driveTable.Cursor()
		if idx >= 0 && idx < len(m.driveData) {
			f := m.driveData[idx]
			if err := m.api.deleteDriveFile(f.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted file: " + f.Name})
				m.driveData = m.api.listDriveFiles()
				m.driveTable = m.newDriveTable()
			}
		}
	case routeDiscovered:
		idx := m.discoveredTable.Cursor()
		if idx >= 0 && idx < len(m.discoveredData) {
			d := m.discoveredData[idx]
			if err := m.api.actionDiscoveredModel(d.ID, "ignore"); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Ignored model: " + d.ModelID})
				m.discoveredData = m.api.listDiscoveredModels()
				m.discoveredTable = m.newDiscoveredTable()
			}
		}
	case routeSupervisor:
		idx := m.supervisorTable.Cursor()
		if idx >= 0 && idx < len(m.escalationsData) {
			e := m.escalationsData[idx]
			if e.Status == "pending" {
				if err := m.api.rejectEscalation(e.FullID, ""); err == nil {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Rejected escalation: " + e.ID})
				}
				m.escalationsData = m.api.listEscalations()
				m.supervisorTable = m.newSupervisorTable()
			}
		}
	case routeSessions:
		idx := m.sessionsTable.Cursor()
		if idx < 0 || idx >= len(m.sessionsData) {
			return
		}
		s := m.sessionsData[idx]
		m.api.deleteSession(s.FullID)
		m.sessionsData = append(m.sessionsData[:idx], m.sessionsData[idx+1:]...)
		m.sessionsTable = m.newSessionsTable()
	case routeWorkflows:
		idx := m.workflowsTable.Cursor()
		if idx >= 0 && idx < len(m.workflowsData) {
			wf := m.workflowsData[idx]
			if err := m.api.deleteWorkflow(wf.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted workflow: " + wf.Name})
				m.workflowsData = m.api.listWorkflows()
				m.workflowsTable = m.newWorkflowsTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Delete failed: " + err.Error()})
			}
		}
	case routeVoice:
		idx := m.voiceTable.Cursor()
		if idx >= 0 && idx < len(m.voiceData) {
			p := m.voiceData[idx]
			if err := m.api.deleteVoiceProvider(p.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted voice provider: " + p.Name})
				m.voiceData = m.api.listVoiceProviders()
				m.voiceTable = m.newVoiceTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Delete failed: " + err.Error()})
			}
		}
	case routeVault:
		idx := m.vaultTable.Cursor()
		if idx >= 0 && idx < len(m.vaultData) {
			e := m.vaultData[idx]
			if err := m.api.deleteVaultEntry(e.FullID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Deleted vault entry: " + e.Name})
				m.vaultData = m.api.listVaultEntries()
				m.vaultTable = m.newVaultTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Delete failed: " + err.Error()})
			}
		}
	case routeProviderKeys:
		idx := m.providerKeysTable.Cursor()
		if idx >= 0 && idx < len(m.providerKeysData.Keys) {
			k := m.providerKeysData.Keys[idx]
			if err := m.api.deleteProviderKey(k.ID); err == nil {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Removed key: " + k.Label})
				m.providerKeysData = m.api.getPoolConfig(m.providerKeysID)
				m.providerKeysTable = m.newProviderKeysTable()
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Delete failed: " + err.Error()})
			}
		}
	}
}
