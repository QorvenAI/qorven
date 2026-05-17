// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package steps holds screen-level helpers for the TUI.
// Each file exposes data structures and rendering helpers that app.go
// imports; they are not standalone Bubbletea models.
package steps

import (
	"fmt"
	"strings"
)

// ModelHubCategory is one of the 6 provider categories.
type ModelHubCategory struct {
	ID    string
	Label string
}

// ModelHubCategories in sidebar order.
var ModelHubCategories = []ModelHubCategory{
	{ID: "generative", Label: "Generative AI"},
	{ID: "image", Label: "Images"},
	{ID: "video", Label: "Video"},
	{ID: "tts", Label: "TTS"},
	{ID: "stt", Label: "STT"},
	{ID: "search", Label: "Search"},
	{ID: "router", Label: "Model Router"},
}

// ProviderRow is one row in the models-hub table.
type ProviderRow struct {
	ID       string
	Name     string
	KeyCount int
	Strategy string
	Budget   string // e.g. "$12/$50"
}

// FormatBudget formats spent/budget as a short string.
func FormatBudget(spent, budget float64) string {
	if budget <= 0 {
		return fmt.Sprintf("$%.2f/∞", spent)
	}
	return fmt.Sprintf("$%.2f/$%.0f", spent, budget)
}

// DiscoveredSummary is a one-line summary of unactioned discovered models.
func DiscoveredSummary(count int) string {
	if count == 0 {
		return ""
	}
	noun := "model"
	if count != 1 {
		noun = "models"
	}
	return fmt.Sprintf("⚡ %d new %s discovered — press D to review", count, noun)
}

// StrategyLabel returns a short display label for a rotation strategy.
func StrategyLabel(strategy string) string {
	switch strategy {
	case "priority":
		return "priority"
	case "round_robin":
		return "round-robin"
	case "random":
		return "random"
	case "least_used":
		return "least-used"
	default:
		return strategy
	}
}

// KeyStatusDot returns a single-character indicator for a key status.
func KeyStatusDot(status string) string {
	switch strings.ToLower(status) {
	case "verified":
		return "●"
	case "invalid":
		return "✗"
	default:
		return "○"
	}
}
