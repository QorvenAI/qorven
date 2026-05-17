// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderCodeView composes the three-panel code layout: filepicker on the left,
// editor in the middle, agent chat on the right. Widths adapt to terminal size
// — below 120 cols the chat pane collapses.
func (m Model) renderCodeView() string {
	sideW := 32
	chatW := 34
	if m.width < 120 {
		chatW = 0
	}
	editorW := m.width - sideW - chatW
	if editorW < 30 {
		editorW = 30
	}
	h := m.codeViewHeight()

	side := m.renderCodeExplorer(sideW, h)
	editor := m.renderCodeEditor(editorW, h)
	chat := m.renderCodeChat(chatW, h)

	vDiv := dividerStyle.Render("│")
	sideLines := strings.Split(side, "\n")
	editorLines := strings.Split(editor, "\n")
	chatLines := strings.Split(chat, "\n")

	rows := make([]string, h)
	for i := 0; i < h; i++ {
		s := safeLine(sideLines, i)
		e := safeLine(editorLines, i)
		c := safeLine(chatLines, i)
		if chatW > 0 {
			rows[i] = s + vDiv + e + vDiv + c
		} else {
			rows[i] = s + vDiv + e
		}
	}

	// Status bar: shared with chat, plus phase progress when active.
	statusLine := m.renderCodeStatus()
	return strings.Join(rows, "\n") + "\n" + statusLine
}

func (m Model) renderCodeExplorer(width, height int) string {
	pickerView := m.codePicker.View()
	header := headerStyle.Render(" EXPLORER")
	projectLine := dimStyle.Render(" " + m.codeProject)
	sep := dividerStyle.Render(strings.Repeat("─", width-1))

	changes := ""
	if len(m.codeChanges) > 0 {
		var sb strings.Builder
		sb.WriteString("\n" + headerStyle.Render(" CHANGES") + "\n")
		for _, c := range m.codeChanges {
			name := c
			if i := strings.LastIndex(c, "/"); i >= 0 {
				name = c[i+1:]
			}
			sb.WriteString(" " + lipgloss.NewStyle().Foreground(amber).Render("✎ "+name) + "\n")
		}
		changes = sb.String()
	}

	body := strings.Join([]string{header, projectLine, sep, pickerView, changes}, "\n")
	return lipgloss.NewStyle().Width(width - 1).Height(height).Render(body)
}

func (m Model) renderCodeEditor(width, height int) string {
	if m.codePath == "" {
		pad := height / 3
		empty := strings.Repeat("\n", pad) +
			dimStyle.Render("        No file open") + "\n" +
			dimStyle.Render("   Select one in the explorer") + "\n" +
			dimStyle.Render("   or ask Prime Coder to open it") + "\n"
		return lipgloss.NewStyle().Width(width - 1).Height(height).Render(empty)
	}

	name := m.codePath
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	hdr := lipgloss.NewStyle().Foreground(purple).Bold(true).Render(" " + name)
	sub := dimStyle.Render("  " + m.codePath)
	sep := dividerStyle.Render(strings.Repeat("─", width-2))

	lineNum := lipgloss.NewStyle().Foreground(dimText).Width(4).Align(lipgloss.Right)
	codeStyle := cellNormal

	lines := strings.Split(m.codeContent, "\n")
	maxLines := height - 4
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	var body strings.Builder
	body.WriteString(hdr + sub + "\n")
	body.WriteString(sep + "\n")
	for i, l := range lines {
		if len(l) > width-8 {
			l = l[:width-8]
		}
		body.WriteString(lineNum.Render(fmt.Sprintf("%d", i+1)) + " " + codeStyle.Render(l) + "\n")
	}
	return lipgloss.NewStyle().Width(width - 1).Height(height).Render(body.String())
}

func (m Model) renderCodeChat(width, height int) string {
	if width <= 0 {
		return ""
	}

	header := headerStyle.Render(" ● PRIME CODER")
	sep := dividerStyle.Render(strings.Repeat("─", width-1))

	var chat strings.Builder
	chat.WriteString(header + "\n")
	chat.WriteString(sep + "\n")

	maxMsgs := height - 6
	start := 0
	if len(m.messages) > maxMsgs {
		start = len(m.messages) - maxMsgs
	}
	for _, msg := range m.messages[start:] {
		switch msg.Role {
		case "user":
			chat.WriteString(userLabel.Render(" You: "))
			content := msg.Content
			if len(content) > width-8 {
				content = content[:width-8] + "…"
			}
			chat.WriteString(content + "\n")
		case "assistant":
			for _, t := range msg.ToolEvents {
				chat.WriteString(cellGreen.Render(" ✓ "+t.name) + "\n")
			}
			for _, l := range strings.Split(msg.Content, "\n") {
				if len(l) > width-4 {
					l = l[:width-4] + "…"
				}
				chat.WriteString(" " + l + "\n")
			}
		default:
			chat.WriteString(dimStyle.Render(" "+msg.Content) + "\n")
		}
	}

	return lipgloss.NewStyle().Width(width).Height(height).Render(chat.String())
}

func (m Model) renderCodeStatus() string {
	bar := m.renderStatusBar(m.width)
	if m.buildPhase == "" || m.buildPhase == "idle" {
		return bar
	}
	phaseLine := " " + headerStyle.Render(m.buildPhase) + "  " + m.buildProgress.View()
	return phaseLine + "\n" + bar
}

func safeLine(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}
