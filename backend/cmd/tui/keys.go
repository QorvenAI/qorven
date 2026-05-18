// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import "charm.land/bubbles/v2/key"

// keyMap is the single source of truth for every keybinding the TUI accepts.
// Each binding carries its help string so help.Model can render documentation
// without us maintaining a second list.
type keyMap struct {
	// Chat mode
	Send            key.Binding
	NewLine         key.Binding
	Cancel          key.Binding
	Quit            key.Binding
	ToggleSidebar   key.Binding
	Help            key.Binding
	Slash           key.Binding
	ToggleThinking  key.Binding

	// Navigation — lists, tables, pickers
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	PgUp   key.Binding
	PgDown key.Binding
	Home   key.Binding
	End    key.Binding

	// Actions inside lists/tables
	Select  key.Binding
	Delete  key.Binding
	Refresh key.Binding
	Back    key.Binding
	Filter  key.Binding

	// Autocomplete / popups
	AcceptSuggestion key.Binding
	DismissPopup     key.Binding
}

// keys is the global keymap. Immutable — treat as a constant.
var keys = keyMap{
	Send:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
	NewLine:       key.NewBinding(key.WithKeys("shift+enter", "alt+enter"), key.WithHelp("shift+enter", "newline")),
	Cancel:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel")),
	Quit:          key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "quit")),
	ToggleSidebar:  key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("ctrl+b", "sidebar")),
	Help:           key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Slash:          key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "command")),
	ToggleThinking: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "thinking")),

	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Left:   key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
	Right:  key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),
	PgUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
	PgDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
	Home:   key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	End:    key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),

	Select:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Delete:  key.NewBinding(key.WithKeys("d", "x"), key.WithHelp("d", "delete")),
	Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Back:    key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),

	AcceptSuggestion: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "accept")),
	DismissPopup:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "dismiss")),
}

// ShortHelp returns the bindings shown on the bottom status bar.
// Tuned for chat mode — the most frequent screen.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Send, k.Slash, k.ToggleSidebar, k.ToggleThinking, k.Help, k.Cancel}
}

// FullHelp returns every binding grouped into logical columns,
// rendered when the user toggles the help overlay.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.NewLine, k.Slash, k.AcceptSuggestion},
		{k.Up, k.Down, k.PgUp, k.PgDown, k.Home, k.End},
		{k.Select, k.Delete, k.Refresh, k.Filter, k.Back},
		{k.ToggleSidebar, k.Help, k.Cancel, k.Quit},
	}
}

// listHelp is the bindings shown inside list/table/picker routes.
func (k keyMap) listHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Delete, k.Refresh, k.Filter, k.Back}
}
