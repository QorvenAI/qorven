// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ── Colour palette ────────────────────────────────────────────────────────────

var (
	cPrimary  = lipgloss.Color("#7C3AED")
	cEmerald  = lipgloss.Color("#34D399")
	cAmber    = lipgloss.Color("#FBBF24")
	cRed      = lipgloss.Color("#F87171")
	cMuted    = lipgloss.Color("#9CA3AF")
	cBorder   = lipgloss.Color("#374151")
	cFgNormal = lipgloss.Color("#F9FAFB")
)

// ── Shared styles ─────────────────────────────────────────────────────────────

var (
	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cBorder).
			Padding(1, 2).
			Width(64)

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(cPrimary)
	mutedStyle = lipgloss.NewStyle().Foreground(cMuted)
	successSt  = lipgloss.NewStyle().Foreground(cEmerald)
	errorSt    = lipgloss.NewStyle().Foreground(cRed)
	warnSt     = lipgloss.NewStyle().Foreground(cAmber)
)

func wSuccess(s string) string { return successSt.Render("✓ " + s) }
func wError(s string) string   { return errorSt.Render("✗ " + s) }
func wInfo(s string) string    { return mutedStyle.Render("→ " + s) }

// ── Brand header ──────────────────────────────────────────────────────────────

func renderHeader() string {
	badge := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(cPrimary).
		Padding(0, 1).
		Render("⚡ Qorven")
	tag := mutedStyle.Render("  Setup Wizard")
	return lipgloss.JoinHorizontal(lipgloss.Center, badge, tag)
}

// ── Step indicator (5 dots) ───────────────────────────────────────────────────

var visualStepLabels = []string{"Account", "Your Agent", "Provider", "Channels", "Done"}

// toVisualStep maps internal step (1-9) → visual step (1-5).
func toVisualStep(step int) int {
	switch {
	case step <= 1:
		return 1
	case step <= 3:
		return 2
	case step == 4:
		return 3
	case step <= 7:
		return 4
	default:
		return 5
	}
}

func renderStepIndicator(currentStep int) string {
	vs := toVisualStep(currentStep)
	// Build each column: dot + label
	cols := make([]string, len(visualStepLabels))
	for i, label := range visualStepLabels {
		n := i + 1
		var dot, lbl string
		switch {
		case n < vs:
			dot = lipgloss.NewStyle().Foreground(cPrimary).Render("●")
			lbl = lipgloss.NewStyle().Foreground(cPrimary).Render(label)
		case n == vs:
			dot = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render("◉")
			lbl = lipgloss.NewStyle().Foreground(cPrimary).Bold(true).Render(label)
		default:
			dot = lipgloss.NewStyle().Foreground(cBorder).Render("○")
			lbl = mutedStyle.Render(label)
		}
		cols[i] = lipgloss.NewStyle().Width(12).Align(lipgloss.Center).
			Render(dot + "\n" + lbl)
	}

	// Interleave connector lines
	var parts []string
	for i, col := range cols {
		parts = append(parts, col)
		if i < len(cols)-1 {
			var lineStyle lipgloss.Style
			if i+1 < vs {
				lineStyle = lipgloss.NewStyle().Foreground(cPrimary)
			} else {
				lineStyle = lipgloss.NewStyle().Foreground(cBorder)
			}
			line := lineStyle.Render(strings.Repeat("─", 3))
			parts = append(parts, lipgloss.NewStyle().PaddingTop(0).Render(line))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// ── Nav bar ───────────────────────────────────────────────────────────────────

func renderNav(step, total int, backDisabled, nextDisabled bool, nextLabel string) string {
	back := "← Back"
	if backDisabled {
		back = mutedStyle.Render("  Back")
	} else {
		back = mutedStyle.Render(back)
	}

	counter := mutedStyle.Render(fmt.Sprintf("%d / %d", step, total))

	if nextLabel == "" {
		nextLabel = "Continue →"
	}
	var next string
	if nextDisabled {
		next = lipgloss.NewStyle().Foreground(cMuted).Background(cBorder).Padding(0, 1).Render(nextLabel)
	} else {
		next = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(cPrimary).Padding(0, 1).Render(nextLabel)
	}

	w := 64
	return lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Width(w/3).Render(back),
		lipgloss.NewStyle().Width(w/3).Align(lipgloss.Center).Render(counter),
		lipgloss.NewStyle().Width(w/3).Align(lipgloss.Right).Render(next),
	)
}
