// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// formField describes a single field in a TUI form.
type formField struct {
	label       string
	placeholder string
	secret      bool     // mask with *
	choices     []string // non-empty → picker, not free-text
}

// formModel is a multi-field text form driven by textinput.
// Tab/↓ advances, ↑ goes back, Enter on last field submits, Esc aborts.
type formModel struct {
	title   string
	fields  []formField
	inputs  []textinput.Model
	cursor  int
	done    bool
	aborted bool
	err     string
	width   int
}

func newFormModel(title string, fields []formField, width int) formModel {
	inputs := make([]textinput.Model, len(fields))
	for i, f := range fields {
		ti := textinput.New()
		ti.Placeholder = f.placeholder
		ti.CharLimit = 512
		ti.SetWidth(width - 20)
		if f.secret {
			ti.EchoMode = textinput.EchoPassword
		}
		inputs[i] = ti
	}
	if len(inputs) > 0 {
		inputs[0].Focus()
	}
	return formModel{
		title:  title,
		fields: fields,
		inputs: inputs,
		width:  width,
	}
}

func (f formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	if f.done || f.aborted {
		return f, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			f.aborted = true
			return f, nil

		case "tab", "down":
			f.err = ""
			f.inputs[f.cursor].Blur()
			f.cursor++
			if f.cursor >= len(f.inputs) {
				f.cursor = 0
			}
			cmd := f.inputs[f.cursor].Focus()
			return f, cmd

		case "shift+tab", "up":
			f.err = ""
			f.inputs[f.cursor].Blur()
			f.cursor--
			if f.cursor < 0 {
				f.cursor = len(f.inputs) - 1
			}
			cmd := f.inputs[f.cursor].Focus()
			return f, cmd

		case "enter":
			if f.cursor == len(f.inputs)-1 {
				// Submit — validate required fields
				for i, inp := range f.inputs {
					isOptional := strings.HasPrefix(f.fields[i].placeholder, "(optional")
					if strings.TrimSpace(inp.Value()) == "" && !isOptional {
						f.err = f.fields[i].label + " is required"
						f.inputs[f.cursor].Blur()
						f.cursor = i
						cmd := f.inputs[f.cursor].Focus()
						return f, cmd
					}
				}
				f.done = true
				return f, nil
			}
			// Advance to next
			f.inputs[f.cursor].Blur()
			f.cursor++
			cmd := f.inputs[f.cursor].Focus()
			return f, cmd
		}

		// Handle number shortcuts for choice fields
		if len(f.fields[f.cursor].choices) > 0 {
			key := msg.String()
			if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
				idx := int(key[0]-'1')
				if idx < len(f.fields[f.cursor].choices) {
					f.inputs[f.cursor].SetValue(f.fields[f.cursor].choices[idx])
					return f, nil
				}
			}
		}
	}

	// Forward to focused input
	var cmd tea.Cmd
	f.inputs[f.cursor], cmd = f.inputs[f.cursor].Update(msg)
	return f, cmd
}

// values returns a map of field label → value after the form is done.
func (f formModel) values() map[string]string {
	m := make(map[string]string, len(f.inputs))
	for i, inp := range f.inputs {
		m[f.fields[i].label] = inp.Value()
	}
	return m
}

func (f formModel) View() string {
	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(logoMagenta).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(white)
	errStyle := lipgloss.NewStyle().Foreground(red)
	hintStyle := lipgloss.NewStyle().Foreground(dimText)
	choiceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7B7B9A"))
	focusMarker := lipgloss.NewStyle().Foreground(purple).Render("▸ ")
	blurMarker := "  "

	sb.WriteString("\n  " + titleStyle.Render(f.title) + "\n\n")

	for i, field := range f.fields {
		focused := i == f.cursor
		marker := blurMarker
		if focused {
			marker = focusMarker
		}
		sb.WriteString(marker + labelStyle.Render(field.label) + "\n")

		if len(field.choices) > 0 {
			cols := 3
			for j, c := range field.choices {
				prefix := fmt.Sprintf("    %d) ", j+1)
				entry := choiceStyle.Render(prefix + c)
				if (j+1)%cols == 0 || j == len(field.choices)-1 {
					sb.WriteString(entry + "\n")
				} else {
					sb.WriteString(entry + "  ")
				}
			}
		}

		inputStyle := lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(borderClr).
			Padding(0, 1)
		if focused {
			inputStyle = inputStyle.BorderForeground(purple)
		}
		sb.WriteString("  " + inputStyle.Render(f.inputs[i].View()) + "\n\n")
	}

	if f.err != "" {
		sb.WriteString("  " + errStyle.Render("✗ "+f.err) + "\n\n")
	}

	sb.WriteString("  " + hintStyle.Render("tab=next  ↑↓=navigate  enter=submit  esc=cancel") + "\n")

	return sb.String()
}
