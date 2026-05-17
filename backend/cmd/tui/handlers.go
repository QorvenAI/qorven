// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"os/exec"
	"runtime"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// copyToClipboard writes text to the system clipboard (best-effort).
func copyToClipboard(text string) {
	tools := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
		{"pbcopy"},
	}
	for _, args := range tools {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return
		}
	}
}

// openBrowser opens a URL in the system default browser (best-effort).
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("cmd", "/c", "start", url)
	}
	return cmd.Start()
}

// handleKey dispatches key presses based on the current route.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Cancel):
		if m.isStreaming {
			if m.streamCancel != nil {
				m.streamCancel()
				m.streamCancel = nil
			}
			m.isStreaming = false
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "⊘ Cancelled"})
			m.updateViewport()
			m.consumedKey = true
			return m.streamTimer.Stop()
		}
		if m.mentionPopupOpen {
			m.mentionPopupOpen = false
			m.consumedKey = true
			return nil
		}
		if m.route == routeSkillMarket {
			m.route = routeSkills
			m.consumedKey = true
			return nil
		}
		if m.route != routeChat {
			m.route = routeChat
			m.consumedKey = true
			return nil
		}
		m.consumedKey = true
		return tea.Quit
	case key.Matches(msg, keys.Quit):
		m.consumedKey = true
		return tea.Quit
	}

	switch m.route {
	case routeChat:
		return m.handleChatKey(msg)
	case routeAgents, routeTools, routeProjects:
		return m.handleListRouteKey(msg)
	case routeSessions, routeProviders, routeVoice, routeMedia, routeModels, routeWorkers,
		routeTasks, routeCron, routeMCP, routeRooms, routeChannels, routeSkills, routePlans, routeMemory, routeDrive,
		routeDiscovered, routeSupervisor, routeRouter, routeSettings, routeWorkflows,
		routeVault, routeUsage, routeNotifications, routeProviderKeys:
		return m.handleTableRouteKey(msg)
	case routeHome:
		return m.handleHomeKey(msg)
	case routeRoomChat:
		return m.handleRoomChatKey(msg)
	case routeGitHub:
		return m.handleGitHubKey(msg)
	case routeModelPicker, routeAgentPicker, routeHelp, routeSkillMarket:
		return m.handlePickerKey(msg)
	case routeCode:
		return m.handleCodeKey(msg)
	case routeFormProvider, routeFormKey, routeFormAgent, routeFormChannel,
		routeFormCron, routeFormMCP, routeFormRoom, routeFormTask,
		routeFormKeyBudget, routeFormPoolStrategy, routeFormAgentEdit,
		routeFormIntegration, routeFormWorkflow, routeFormVoice, routeFormVault,
		routeFormChannelEdit, routeFormVoiceEdit:
		return nil
	case routeDriveUpload:
		if key.Matches(msg, keys.Back) {
			m.route = routeDrive
			m.consumedKey = true
		}
		return nil
	}
	return nil
}
