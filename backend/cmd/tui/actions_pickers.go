// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"strings"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/list"
)

func (m *Model) applyPickerSelection() {
	switch m.route {
	case routeModelPicker:
		if p, ok := m.modelPicker.SelectedItem().(pickerItem); ok {
			m.modelName = p.label
			if p.id != "" && strings.Contains(p.id, "/") {
				parts := strings.SplitN(p.id, "/", 2)
				if len(parts) == 2 {
					go m.api.selectModel(parts[0], parts[1]) //nolint:errcheck
				}
			}
		}
	case routeAgentPicker:
		if p, ok := m.agentPicker.SelectedItem().(pickerItem); ok {
			m.agentName = p.label
			if p.id != "" {
				m.agentID = p.id
			}
		}
	case routeHelp:
		// Help is informational — enter just closes.
	}
	m.route = routeChat
}

func (m *Model) openModelPicker() {
	selected := m.api.listSelectedModels()
	var items []list.Item
	if len(selected) > 0 {
		hub := m.api.listModelHub()
		providerName := make(map[string]string)
		for _, e := range hub {
			providerName[e.ProviderID] = e.ProviderName
		}
		seen := make(map[string]bool)
		for _, s := range selected {
			pname := providerName[s.ProviderID]
			if pname == "" {
				pname = s.ProviderID
			}
			key := s.ProviderID + "/" + s.ModelID
			if !seen[key] {
				seen[key] = true
				hint := pname
				if s.IsDefault {
					hint = pname + " · default"
				}
				items = append(items, pickerItem{label: s.ModelID, id: s.ProviderID + "/" + s.ModelID, hint: hint})
			}
		}
	}
	if len(items) == 0 {
		hub := m.api.listModelHub()
		for _, e := range hub {
			hint := e.ProviderName
			if e.IsDefault {
				hint += " · default"
			}
			items = append(items, pickerItem{label: e.ModelID, id: e.ProviderID + "/" + e.ModelID, hint: hint})
		}
	}
	if len(items) == 0 {
		items = []list.Item{pickerItem{label: "(no models configured)", id: "", hint: "go to /modelshub to add models"}}
	}
	m.modelPicker = m.newList("Switch Model", items, "model", "models")
	for i, it := range items {
		if p, ok := it.(pickerItem); ok && p.label == m.modelName {
			m.modelPicker.Select(i)
			break
		}
	}
	m.route = routeModelPicker
}

func (m *Model) openAgentPicker() {
	agents := m.api.listAgents()
	items := make([]list.Item, len(agents))
	for i, a := range agents {
		items[i] = pickerItem{label: a.Name, id: a.ID, hint: a.Model}
	}
	m.agentPicker = m.newList("Switch Agent", items, "agent", "agents")
	for i, it := range items {
		if p, ok := it.(pickerItem); ok && p.label == m.agentName {
			m.agentPicker.Select(i)
			break
		}
	}
	m.route = routeAgentPicker
}

func (m *Model) openSkillMarket() {
	skills := m.api.listMarketplaceSkills()
	items := make([]list.Item, len(skills))
	for i, s := range skills {
		items[i] = pickerItem{label: s.Name, id: s.Slug, hint: s.Description}
	}
	if len(items) == 0 {
		items = []list.Item{pickerItem{label: "No marketplace skills available", id: "", hint: ""}}
	}
	m.skillMarketPicker = m.newList("Install Skill from Marketplace  (Enter = install, Esc = back)", items, "skill", "skills")
	m.route = routeSkillMarket
}

func (m *Model) selectSkillMarketItem() {
	if sel, ok := m.skillMarketPicker.SelectedItem().(pickerItem); ok && sel.id != "" {
		if err := m.api.installSkill(sel.id); err == nil {
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✓ Installed skill: " + sel.label})
		} else {
			m.messages = append(m.messages, ChatMessage{Role: "system", Content: "✗ Install failed: " + err.Error()})
		}
		m.skillsData = m.api.listSkills()
		m.skillsTable = m.newSkillsTable()
	}
	m.route = routeSkills
}

func (m *Model) openHelp() {
	items := make([]list.Item, len(slashCommands))
	for i, c := range slashCommands {
		items[i] = slashItem{cmd: c}
	}
	m.helpList = m.newList("Commands", items, "command", "commands")
	m.route = routeHelp
}

func (m *Model) syncSlashPopup() {
	val := m.textarea.Value()
	trimmed := strings.TrimSpace(val)

	if !strings.HasPrefix(trimmed, "/") || m.isStreaming {
		m.slashPopupOpen = false
		return
	}
	if strings.Contains(trimmed, " ") {
		base := strings.SplitN(trimmed, " ", 2)[0]
		matched := false
		for _, c := range slashCommands {
			if strings.EqualFold(c.name, base) {
				matched = true
				break
			}
		}
		if !matched {
			m.slashPopupOpen = false
			return
		}
	}

	items := buildSlashItems(trimmed)
	if len(items) == 0 {
		m.slashPopupOpen = false
		return
	}

	if !m.slashPopupOpen {
		m.slashPopup = m.newSlashPopup(items)
		m.slashPopupOpen = true
		return
	}

	m.slashPopup.SetItems(items)
}

func (m *Model) syncMentionPopup() {
	val := m.textarea.Value()
	atIdx := strings.LastIndex(val, "@")
	if atIdx < 0 || m.isStreaming {
		m.mentionPopupOpen = false
		return
	}
	if atIdx > 0 && val[atIdx-1] != ' ' && val[atIdx-1] != '\n' {
		m.mentionPopupOpen = false
		return
	}
	partial := val[atIdx+1:]
	if strings.Contains(partial, " ") {
		m.mentionPopupOpen = false
		return
	}

	agents := m.agentsData
	if len(agents) == 0 {
		agents = m.api.listAgents()
		m.agentsData = agents
	}
	var items []list.Item
	partialLower := strings.ToLower(partial)
	for _, a := range agents {
		if partialLower == "" || strings.Contains(strings.ToLower(a.Key), partialLower) ||
			strings.Contains(strings.ToLower(a.Name), partialLower) {
			items = append(items, pickerItem{label: a.Key, id: a.ID, hint: a.Name + " · " + a.Model})
		}
	}
	if len(items) == 0 {
		m.mentionPopupOpen = false
		return
	}
	if !m.mentionPopupOpen {
		m.mentionPopup = m.newSlashPopup(items)
		m.mentionPopupOpen = true
	} else {
		m.mentionPopup.SetItems(items)
	}
}

func (m *Model) newSlashPopup(items []list.Item) list.Model {
	popupHeight := 6
	if len(items) < popupHeight {
		popupHeight = len(items)
	}
	if popupHeight < 1 {
		popupHeight = 1
	}

	maxH := m.height - 10
	if maxH < 2 {
		maxH = 2
	}
	listH := popupHeight * 2
	if listH > maxH {
		listH = maxH
	}

	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)

	chatW, _ := m.chatDimensions()
	l := list.New(items, delegate, chatW-4, listH)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	return l
}

func newCodePicker(dir string, height int) filepicker.Model {
	fp := filepicker.New()
	fp.CurrentDirectory = dir
	fp.AllowedTypes = []string{
		".go", ".ts", ".tsx", ".js", ".jsx",
		".md", ".json", ".yaml", ".yml", ".toml",
		".py", ".rs", ".sh", ".sql", ".html", ".css",
	}
	fp.DirAllowed = false
	fp.FileAllowed = true
	fp.ShowHidden = false
	fp.ShowPermissions = false
	fp.ShowSize = false
	fp.SetHeight(height)
	return fp
}

func (m *Model) codeViewHeight() int {
	h := m.height - 4
	if h < 10 {
		h = 10
	}
	return h
}
