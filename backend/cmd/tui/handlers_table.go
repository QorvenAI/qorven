// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleListRouteKey(msg tea.KeyPressMsg) tea.Cmd {
	var activeList *list.Model
	switch m.route {
	case routeAgents:
		activeList = &m.agentsList
	case routeTools:
		activeList = &m.toolsList
	case routeProjects:
		activeList = &m.projectsList
	}
	if activeList != nil && activeList.SettingFilter() {
		return nil
	}

	switch {
	case key.Matches(msg, keys.Back):
		m.route = routeChat
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Refresh):
		m.refreshActiveList()
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Select):
		cmd := m.selectListItem()
		m.consumedKey = true
		return cmd
	case key.Matches(msg, keys.Delete):
		m.deleteListItem()
		m.consumedKey = true
		return nil
	case msg.String() == "n":
		m.openCreateForm()
		m.consumedKey = true
		return nil
	case msg.String() == "e" && m.route == routeAgents:
		if a, ok := m.agentsList.SelectedItem().(agentItem); ok {
			m.formEditID = a.info.ID
			m.enterFormAgentEdit(a.info.Name, a.info.Model)
		}
		m.consumedKey = true
		return nil
	}
	return nil
}

func (m *Model) handleTableRouteKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Back):
		m.route = routeChat
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Refresh):
		m.refreshActiveList()
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Select):
		m.selectTableRow()
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Delete):
		m.deleteTableRow()
		m.consumedKey = true
		return nil
	case msg.String() == "n":
		m.openCreateForm()
		m.consumedKey = true
		return nil
	case msg.String() == "u" && m.route == routeDrive:
		m.driveUploadPicker = newCodePicker("/home", m.codeViewHeight())
		m.driveUploadPicker.AllowedTypes = nil
		m.route = routeDriveUpload
		m.consumedKey = true
		return m.driveUploadPicker.Init()
	case msg.String() == "s" && m.route == routeProviders:
		idx := m.providersTable.Cursor()
		if idx >= 0 && idx < len(m.providersData) {
			m.formKeyID = m.providersData[idx].FullID
			m.enterFormPoolStrategy()
		}
		m.consumedKey = true
		return nil
	case msg.String() == "i" && m.route == routeSkills:
		m.openSkillMarket()
		m.consumedKey = true
		return nil
	case msg.String() == "e" && m.route == routeChannels:
		idx := m.channelsTable.Cursor()
		if idx >= 0 && idx < len(m.channelsData) {
			c := m.channelsData[idx]
			m.formEditID = c.FullID
			m.enterFormChannelEdit(c.Name, c.Kind)
		}
		m.consumedKey = true
		return nil
	case msg.String() == "e" && m.route == routeVoice:
		idx := m.voiceTable.Cursor()
		if idx >= 0 && idx < len(m.voiceData) {
			p := m.voiceData[idx]
			m.formEditID = p.FullID
			m.enterFormVoiceEdit(p.Name)
		}
		m.consumedKey = true
		return nil
	case msg.String() == "t" && m.route == routeProviderKeys:
		idx := m.providerKeysTable.Cursor()
		if idx >= 0 && idx < len(m.providerKeysData.Keys) {
			k := m.providerKeysData.Keys[idx]
			if models, err := m.api.testKey(k.ID); err == nil {
				if len(models) > 0 {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: fmt.Sprintf("✓ Key OK — %d models: %s", len(models), strings.Join(models[:min(5, len(models))], ", "))})
				} else {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ Key valid"})
				}
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Key test failed: " + err.Error()})
			}
			m.updateViewport()
		}
		m.consumedKey = true
		return nil
	case (msg.String() == "tab" || msg.String() == "\t") && m.route == routeWorkers:
		m.workersSubView = (m.workersSubView + 1) % 3
		switch m.workersSubView {
		case 0:
			m.workersAgents = m.api.listDaemonAgents()
			m.workersAgentTable = m.newWorkersAgentTable()
		case 1:
			m.workersTasks = m.api.listDaemonTasks()
			m.tasksData = m.api.listTasks("")
			m.tasksTable = m.newTasksTable()
		case 2:
			m.workersPlans = m.api.listDaemonPlans()
			m.workersPlanTable = m.newWorkersPlanTable()
		}
		m.consumedKey = true
		return nil
	}
	return nil
}

func (m *Model) handlePickerKey(msg tea.KeyPressMsg) tea.Cmd {
	if m.route == routeSkillMarket {
		if !m.skillMarketPicker.SettingFilter() {
			switch {
			case key.Matches(msg, keys.Back):
				m.route = routeSkills
				m.consumedKey = true
				return nil
			case key.Matches(msg, keys.Select):
				m.selectSkillMarketItem()
				m.consumedKey = true
				return nil
			}
		}
		return nil
	}

	var activeList *list.Model
	switch m.route {
	case routeModelPicker:
		activeList = &m.modelPicker
	case routeAgentPicker:
		activeList = &m.agentPicker
	case routeHelp:
		activeList = &m.helpList
	}
	if activeList != nil && activeList.SettingFilter() {
		return nil
	}

	switch {
	case key.Matches(msg, keys.Back):
		m.route = routeChat
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Select):
		m.applyPickerSelection()
		m.consumedKey = true
		return nil
	}
	return nil
}

func (m *Model) handleHomeKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Select), msg.String() == "\r", msg.String() == "\n":
		idx := m.homeSessionTable.Cursor()
		if idx >= 0 && idx < len(m.homeData.RecentSessions) {
			s := m.homeData.RecentSessions[idx]
			if s.FullID != "" && s.ID != "—" {
				fullID := s.FullID
				m.sessionID = fullID
				m.messages = loadSession(fullID)
				if len(m.messages) == 0 {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Resumed session " + s.ID})
				}
				m.route = routeChat
				m.updateViewport()
				m.consumedKey = true
				return nil
			}
		}
		m.route = routeChat
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Refresh):
		m.homeData = m.api.getHomeDashboard()
		m.homeSessionTable = m.newHomeSessionTable()
		m.consumedKey = true
		return nil
	}
	return nil
}

func (m *Model) handleRoomChatKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Back):
		m.route = routeRooms
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Refresh):
		m.roomMessages = m.api.listRoomMessages(m.roomChatID)
		m.consumedKey = true
		return nil
	case key.Matches(msg, keys.Send):
		text := strings.TrimSpace(m.roomChatInput.Value())
		if text == "" {
			m.consumedKey = true
			return nil
		}
		if err := m.api.postRoomMessage(m.roomChatID, text); err == nil {
			m.roomChatInput.Reset()
			m.roomMessages = m.api.listRoomMessages(m.roomChatID)
		} else {
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Send failed: " + err.Error()})
		}
		m.consumedKey = true
		return nil
	}
	return nil
}

func (m *Model) handleCodeKey(msg tea.KeyPressMsg) tea.Cmd {
	if key.Matches(msg, keys.Back) {
		m.route = routeChat
		m.consumedKey = true
		return nil
	}
	return nil
}

func (m *Model) handleGitHubKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Back):
		m.route = routeChat
		m.consumedKey = true
		return nil

	case key.Matches(msg, keys.Refresh):
		m.refreshGitHubData()
		m.consumedKey = true
		return nil

	case msg.String() == "tab", msg.String() == "\t":
		m.githubSubView = (m.githubSubView + 1) % 3
		m.consumedKey = true
		return nil

	case key.Matches(msg, keys.Select):
		m.consumedKey = true
		switch m.githubSubView {
		case 0:
			idx := m.githubPRTable.Cursor()
			if idx >= 0 && idx < len(m.githubPRs) {
				pr := m.githubPRs[idx]
				if err := m.api.mergeGitHubPR(m.githubOwner, m.githubRepo, pr.Number); err == nil {
					m.refreshGitHubData()
				}
			}
		case 1:
			idx := m.githubIssueTable.Cursor()
			if idx >= 0 && idx < len(m.githubIssues) {
				iss := m.githubIssues[idx]
				m.textarea.SetValue("@prime Triage and fix GitHub issue #" + fmt.Sprintf("%d", iss.Number) + ": " + iss.Title)
				m.route = routeChat
			}
		}
		return nil

	case key.Matches(msg, keys.Delete):
		m.consumedKey = true
		switch m.githubSubView {
		case 0:
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "To close a PR, open it in browser (o) and close from GitHub"})
			m.updateViewport()
		case 1:
			idx := m.githubIssueTable.Cursor()
			if idx >= 0 && idx < len(m.githubIssues) {
				iss := m.githubIssues[idx]
				if err := m.api.closeGitHubIssue(m.githubOwner, m.githubRepo, iss.Number); err == nil {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: fmt.Sprintf("✓ Closed issue #%d", iss.Number)})
					m.updateViewport()
					m.refreshGitHubData()
				} else {
					m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Close failed: " + err.Error()})
					m.updateViewport()
				}
			}
		}
		return nil

	case msg.String() == "o":
		m.consumedKey = true
		var url string
		switch m.githubSubView {
		case 0:
			idx := m.githubPRTable.Cursor()
			if idx >= 0 && idx < len(m.githubPRs) {
				url = m.githubPRs[idx].HTMLURL
			}
		case 1:
			idx := m.githubIssueTable.Cursor()
			if idx >= 0 && idx < len(m.githubIssues) {
				url = m.githubIssues[idx].HTMLURL
			}
		case 2:
			idx := m.githubTaskTable.Cursor()
			if idx >= 0 && idx < len(m.githubTasks) {
				url = m.githubTasks[idx].PRURL
			}
		}
		if url != "" {
			_ = openBrowser(url)
		}
		return nil
	}
	return nil
}
