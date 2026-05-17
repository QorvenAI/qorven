// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
	gloss "github.com/charmbracelet/lipgloss"
)

// mdRenderer is cached per-width. Glamour bakes word-wrap into its styles so
// re-using a renderer built for a different width produces mis-aligned output.
var (
	mdRenderer *glamour.TermRenderer
	mdWidth    int
)

func markdownRenderer(width int) *glamour.TermRenderer {
	if width <= 0 {
		width = 80
	}
	if mdRenderer != nil && mdWidth == width {
		return mdRenderer
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
		glamour.WithEmoji(),
	)
	if err != nil {
		return nil
	}
	mdRenderer = r
	mdWidth = width
	return r
}

// renderMarkdown converts markdown to styled terminal output using glamour.
// Returns the original content if rendering fails for any reason.
func renderMarkdown(content string, width int) string {
	r := markdownRenderer(width)
	if r == nil {
		return content
	}
	out, err := r.Render(content)
	if err != nil {
		return content
	}
	// Glamour adds trailing blank lines — strip them so chat spacing is tight.
	return strings.TrimRight(out, "\n")
}

// renderDiff colors diff output: green for +, red for -, blue for hunk markers.
// Keeps rendering synchronous and allocation-light — diffs can be large.
func renderDiff(content string) string {
	addStyle := gloss.NewStyle().Foreground(gloss.Color("#22c55e"))
	delStyle := gloss.NewStyle().Foreground(gloss.Color("#ef4444"))
	hunkStyle := gloss.NewStyle().Foreground(gloss.Color("#3b82f6"))
	patchStyle := gloss.NewStyle().Foreground(gloss.Color("#eab308")).Bold(true)

	var sb strings.Builder
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			sb.WriteString(delStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "***"):
			sb.WriteString(patchStyle.Render(line))
		default:
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// hasDiffContent detects patch/diff markers so we can fall back from glamour
// rendering (which does not handle unified diffs well).
func hasDiffContent(s string) bool {
	if strings.Contains(s, "--- a/") || strings.Contains(s, "+++ b/") ||
		strings.Contains(s, "*** Begin Patch") {
		return true
	}
	// heuristic: an assistant sometimes describes an edit as "edited X, replaced Y"
	return strings.Contains(s, "edited ") && strings.Contains(s, "replaced")
}

// renderContent picks a renderer based on content shape. Plain prose with no
// markdown or diff markers is returned untouched.
func renderContent(content string, width int) string {
	if hasDiffContent(content) {
		return renderDiff(content)
	}
	if strings.Contains(content, "```") ||
		strings.Contains(content, "## ") ||
		strings.Contains(content, "**") ||
		strings.Contains(content, "\n- ") {
		return renderMarkdown(content, width)
	}
	return content
}
