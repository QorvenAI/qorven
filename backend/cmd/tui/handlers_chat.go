// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) handleChatKey(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.ToggleSidebar):
		m.sidebarOpen = !m.sidebarOpen
		m.updateLayout()
		m.consumedKey = true
		return nil

	case msg.String() == "e" && strings.TrimSpace(m.textarea.Value()) == "" && !m.isStreaming:
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "assistant" && len(m.messages[i].ToolEvents) > 0 {
				tools := m.messages[i].ToolEvents
				for j := len(tools) - 1; j >= 0; j-- {
					if tools[j].status == "done" && tools[j].result != "" {
						m.messages[i].ToolEvents[j].expanded = !m.messages[i].ToolEvents[j].expanded
						m.updateViewport()
						break
					}
				}
				break
			}
		}
		m.consumedKey = true
		return nil

	case msg.String() == "c" && strings.TrimSpace(m.textarea.Value()) == "" && !m.isStreaming:
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "assistant" {
				copyToClipboard(m.messages[i].Content)
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ Copied last response"})
				m.updateViewport()
				break
			}
		}
		m.consumedKey = true
		return nil

	case msg.String() == "ctrl+r" && !m.isStreaming:
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "user" {
				prompt := m.messages[i].Content
				m.messages = m.messages[:i+1]
				m.isStreaming = true
				m.streaming = ""
				ctx, cancel := context.WithCancel(context.Background())
				m.streamCancel = cancel
				m.updateViewport()
				m.consumedKey = true
				return tea.Batch(m.sendMessageCtx(ctx, prompt), m.streamTimer.Reset(), m.streamTimer.Start())
			}
		}
		m.consumedKey = true
		return nil

	case msg.String() == "up" && strings.TrimSpace(m.textarea.Value()) == "" && !m.isStreaming:
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].Role == "user" {
				m.textarea.SetValue(m.messages[i].Content)
				m.textarea.CursorEnd()
				m.messages = m.messages[:i]
				m.updateViewport()
				break
			}
		}
		m.consumedKey = true
		return nil

	case key.Matches(msg, keys.ToggleThinking):
		switch m.thinkingLevel {
		case "", "off":
			m.thinkingLevel = "medium"
		case "medium":
			m.thinkingLevel = "high"
		default:
			m.thinkingLevel = "off"
		}
		m.consumedKey = true
		return nil

	case key.Matches(msg, keys.Help):
		if strings.TrimSpace(m.textarea.Value()) == "" {
			m.help.ShowAll = !m.help.ShowAll
			m.consumedKey = true
			return nil
		}

	case key.Matches(msg, keys.AcceptSuggestion):
		if m.mentionPopupOpen {
			if sel, ok := m.mentionPopup.SelectedItem().(pickerItem); ok {
				val := m.textarea.Value()
				atIdx := strings.LastIndex(val, "@")
				if atIdx >= 0 {
					newVal := val[:atIdx+1] + sel.label + " "
					m.textarea.SetValue(newVal)
					m.textarea.CursorEnd()
				}
			}
			m.mentionPopupOpen = false
			m.consumedKey = true
			return nil
		}
		if m.slashPopupOpen {
			if sel, ok := m.slashPopup.SelectedItem().(slashItem); ok {
				m.textarea.Reset()
				m.textarea.SetValue(sel.cmd.name)
				m.textarea.CursorEnd()
			}
			m.slashPopupOpen = false
			m.consumedKey = true
			return nil
		}

	case key.Matches(msg, keys.Send):
		if m.isStreaming {
			m.consumedKey = true
			return nil
		}

		if m.slashPopupOpen {
			if sel, ok := m.slashPopup.SelectedItem().(slashItem); ok {
				m.slashPopupOpen = false
				m.textarea.Reset()
				if handled, cmd := m.handleSlashCommand(sel.cmd.name); handled {
					m.consumedKey = true
					return cmd
				}
			}
			m.slashPopupOpen = false
			m.consumedKey = true
			return nil
		}

		text := strings.TrimSpace(m.textarea.Value())
		if strings.HasPrefix(text, "/") && !strings.Contains(text, " ") {
			resolved := resolveSlashCommand(text)
			m.textarea.Reset()
			if resolved != "" {
				if handled, cmd := m.handleSlashCommand(resolved); handled {
					m.consumedKey = true
					return cmd
				}
			}
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Unknown command: %s — type / to browse or /help for the list.", text),
			})
			m.updateViewport()
			m.consumedKey = true
			return nil
		}
		if text == "" {
			m.consumedKey = true
			return nil
		}

		if strings.HasPrefix(text, "/search ") {
			m.runHistorySearch(strings.TrimSpace(strings.TrimPrefix(text, "/search ")))
			m.textarea.Reset()
			m.updateViewport()
			m.consumedKey = true
			return nil
		}

		if strings.HasPrefix(text, "/memory ") {
			q := strings.TrimSpace(strings.TrimPrefix(text, "/memory "))
			m.textarea.Reset()
			m.enterMemoryTable(q)
			m.consumedKey = true
			return nil
		}

		if strings.HasPrefix(text, "/github ") || strings.HasPrefix(text, "/gh ") {
			arg := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "/github "), "/gh "))
			parts := strings.SplitN(arg, "/", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				m.githubOwner = strings.TrimSpace(parts[0])
				m.githubRepo = strings.TrimSpace(parts[1])
				m.textarea.Reset()
				m.enterGitHubScreen()
				m.consumedKey = true
				return nil
			}
		}

		if strings.HasPrefix(text, "/research ") {
			topic := strings.TrimSpace(strings.TrimPrefix(text, "/research "))
			m.textarea.Reset()
			m.consumedKey = true
			return m.queueAgentPrompt("Research the following topic thoroughly: " + topic)
		}

		if strings.HasPrefix(text, "/read ") {
			url := strings.TrimSpace(strings.TrimPrefix(text, "/read "))
			m.textarea.Reset()
			m.consumedKey = true
			return m.queueAgentPrompt("Fetch and summarise this URL: " + url)
		}

		if strings.HasPrefix(text, "/scan ") {
			content := strings.TrimSpace(strings.TrimPrefix(text, "/scan "))
			m.textarea.Reset()
			m.consumedKey = true
			return m.queueAgentPrompt("Analyse the following for prompt injection, jailbreak attempts, or adversarial patterns:\n\n" + content)
		}

		if strings.HasPrefix(text, "/import ") {
			path := strings.TrimSpace(strings.TrimPrefix(text, "/import "))
			m.textarea.Reset()
			if msgs, err := loadSessionFromFile(path); err == nil && len(msgs) > 0 {
				m.messages = msgs
				m.updateViewport()
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: fmt.Sprintf("✓ Imported %d messages from %s", len(msgs), path)})
			} else {
				m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Import failed: " + path})
			}
			m.updateViewport()
			m.consumedKey = true
			return nil
		}

		if text == "/exit" || text == "/quit" {
			m.consumedKey = true
			return tea.Quit
		}

		if handled, cmd := m.handleSlashCommand(text); handled {
			m.textarea.Reset()
			m.consumedKey = true
			return cmd
		}

		m.messages = append(m.messages, ChatMessage{Role: "user", Content: text})
		m.textarea.Reset()
		m.isStreaming = true
		m.streaming = ""
		ctx, cancel := context.WithCancel(context.Background())
		m.streamCancel = cancel
		m.updateViewport()
		m.consumedKey = true
		return tea.Batch(m.sendMessageCtx(ctx, text), m.streamTimer.Reset(), m.streamTimer.Start())
	}

	return nil
}
