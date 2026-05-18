// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"github.com/sahilm/fuzzy"
)

// agentItem, toolItem, projectItem, slashItem implement list.Item.
// FilterValue is what the built-in fuzzy filter matches against.
// Title / Description drive the DefaultDelegate renderer.

type agentItem struct{ info AgentInfo }

func (a agentItem) Title() string {
	role := a.info.Key
	if role == "chief" {
		role = "Prime"
	}
	return a.info.Name + "  " + role
}
func (a agentItem) Description() string {
	return "◇ " + a.info.Model
}
func (a agentItem) FilterValue() string { return a.info.Name + " " + a.info.Key + " " + a.info.Model }

type toolItem struct{ info ToolInfo }

func (t toolItem) Title() string       { return t.info.Name }
func (t toolItem) Description() string { return t.info.Desc }
func (t toolItem) FilterValue() string { return t.info.Name + " " + t.info.Desc }

type projectItem struct{ info ProjectInfo }

func (p projectItem) Title() string {
	name := p.info.DisplayName
	if name == "" {
		name = p.info.Name
	}
	return name
}

func (p projectItem) Description() string {
	phase := p.info.Phase
	if phase == "" {
		phase = "ready"
	}
	return fmt.Sprintf("%s · %s", phase, p.info.Path)
}

func (p projectItem) FilterValue() string {
	return p.info.Name + " " + p.info.DisplayName + " " + p.info.Path
}

type slashItem struct{ cmd slashCmd }

func (s slashItem) Title() string       { return s.cmd.name }
func (s slashItem) Description() string { return s.cmd.desc }
func (s slashItem) FilterValue() string { return s.cmd.name + " " + s.cmd.desc }

// pickerItem is a minimal list.Item for model/agent pickers.
// id is carried alongside the label because agent names aren't unique IDs.
type pickerItem struct {
	label string
	id    string
	hint  string
}

func (p pickerItem) Title() string       { return p.label }
func (p pickerItem) Description() string { return p.hint }
func (p pickerItem) FilterValue() string { return p.label + " " + p.hint }

// buildAgentItems converts agent infos to list items.
func buildAgentItems(agents []AgentInfo) []list.Item {
	items := make([]list.Item, len(agents))
	for i, a := range agents {
		items[i] = agentItem{info: a}
	}
	return items
}

func buildToolItems(tools []ToolInfo) []list.Item {
	items := make([]list.Item, len(tools))
	for i, t := range tools {
		items[i] = toolItem{info: t}
	}
	return items
}

func buildProjectItems(projects []ProjectInfo) []list.Item {
	items := make([]list.Item, len(projects))
	for i, p := range projects {
		items[i] = projectItem{info: p}
	}
	return items
}

// resolveSlashCommand maps a user-typed slash token to a real command name.
// Exact matches win immediately. Otherwise the best fuzzy match is returned
// — but only when the query is distinctive enough to avoid surprising the
// user (length ≥ 2 and at least one alphanumeric character). Returns "" when
// nothing reasonable resolves.
func resolveSlashCommand(token string) string {
	t := strings.TrimSpace(token)
	if t == "" || t == "/" {
		return ""
	}
	if !strings.HasPrefix(t, "/") {
		t = "/" + t
	}
	// Exact match
	for _, c := range slashCommands {
		if strings.EqualFold(c.name, t) {
			return c.name
		}
	}
	query := strings.ToLower(strings.TrimPrefix(t, "/"))
	if len(query) < 2 {
		return ""
	}
	targets := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		targets[i] = strings.TrimPrefix(c.name, "/")
	}
	matches := fuzzy.Find(query, targets)
	if len(matches) == 0 {
		return ""
	}
	return slashCommands[matches[0].Index].name
}

// buildSlashItems returns the slash command list filtered by `matching`.
// Uses the same fuzzy matcher as bubbles/list, so typos like "/researh"
// still surface "/research" as a match. Results are ranked by match quality;
// the leading `/` is stripped before matching so "/mod" → "/model" wins.
func buildSlashItems(matching string) []list.Item {
	query := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(matching), "/"))

	if query == "" {
		items := make([]list.Item, len(slashCommands))
		for i, c := range slashCommands {
			items[i] = slashItem{cmd: c}
		}
		return items
	}

	targets := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		targets[i] = strings.TrimPrefix(c.name, "/")
	}

	matches := fuzzy.Find(query, targets)
	items := make([]list.Item, 0, len(matches))
	for _, m := range matches {
		items = append(items, slashItem{cmd: slashCommands[m.Index]})
	}
	return items
}
