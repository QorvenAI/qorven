// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"fmt"
)

// PinnedTileInput mirrors the fields needed to create a dashboard tile.
// Using a local type avoids an import cycle between tools and dashboard packages.
type PinnedTileInput struct {
	SourceSlug         string
	ToolName           string
	ToolArgs           map[string]any
	WidgetType         string
	Label              string
	Position           int
	RefreshIntervalSec int
}

// PinToDashboardTool pins a data source tile to the main dashboard.
type PinToDashboardTool struct {
	pin      func(ctx context.Context, t PinnedTileInput) (string, error)
	tenantID string
}

// NewPinToDashboardTool creates a PinToDashboardTool with the given pin callback.
func NewPinToDashboardTool(pin func(ctx context.Context, t PinnedTileInput) (string, error), tenantID string) *PinToDashboardTool {
	return &PinToDashboardTool{pin: pin, tenantID: tenantID}
}

func (t *PinToDashboardTool) Name() string { return "pin_to_dashboard" }

func (t *PinToDashboardTool) Description() string {
	return "Pin a data tile to the main dashboard. The tile will auto-refresh and display live connector data."
}

func (t *PinToDashboardTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_slug": map[string]any{
				"type":        "string",
				"description": "The connector app slug that provides the data for this tile.",
			},
			"tool_name": map[string]any{
				"type":        "string",
				"description": "The tool name within the connector to call for data.",
			},
			"tool_args": map[string]any{
				"type":        "object",
				"description": "Arguments to pass to the tool when fetching data.",
			},
			"widget_type": map[string]any{
				"type":        "string",
				"description": "How to render the tile on the dashboard.",
				"enum":        []string{"stat-card", "data-table", "feed", "list", "chart"},
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Human-readable label for the tile (optional).",
			},
			"refresh_interval_sec": map[string]any{
				"type":        "integer",
				"description": "How often to refresh the tile data in seconds (default: 300).",
			},
		},
		"required": []string{"source_slug", "tool_name", "widget_type"},
	}
}

func (t *PinToDashboardTool) Execute(ctx context.Context, args map[string]any) *Result {
	sourceSlug, _ := args["source_slug"].(string)
	toolName, _ := args["tool_name"].(string)
	widgetType, _ := args["widget_type"].(string)
	label, _ := args["label"].(string)

	if sourceSlug == "" {
		return ErrorResult("source_slug is required")
	}
	if toolName == "" {
		return ErrorResult("tool_name is required")
	}
	switch widgetType {
	case "stat-card", "data-table", "feed", "list", "chart":
	case "":
		return ErrorResult("widget_type is required")
	default:
		return ErrorResult("widget_type must be one of: stat-card, data-table, feed, list, chart")
	}

	toolArgs, _ := args["tool_args"].(map[string]any)
	if toolArgs == nil {
		toolArgs = map[string]any{}
	}

	refreshSec := 300
	if v, ok := args["refresh_interval_sec"].(float64); ok && v > 0 {
		refreshSec = int(v)
	}

	input := PinnedTileInput{
		SourceSlug:         sourceSlug,
		ToolName:           toolName,
		ToolArgs:           toolArgs,
		WidgetType:         widgetType,
		Label:              label,
		RefreshIntervalSec: refreshSec,
	}

	id, err := t.pin(ctx, input)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to pin tile: %v", err))
	}

	return TextResult(fmt.Sprintf("Pinned to dashboard. Tile ID: %s", id))
}

// UnpinFromDashboardTool removes a tile from the main dashboard.
type UnpinFromDashboardTool struct {
	unpin    func(ctx context.Context, tenantID, id string) error
	tenantID string
}

// NewUnpinFromDashboardTool creates an UnpinFromDashboardTool with the given unpin callback.
func NewUnpinFromDashboardTool(unpin func(ctx context.Context, tenantID, id string) error, tenantID string) *UnpinFromDashboardTool {
	return &UnpinFromDashboardTool{unpin: unpin, tenantID: tenantID}
}

func (t *UnpinFromDashboardTool) Name() string { return "unpin_from_dashboard" }

func (t *UnpinFromDashboardTool) Description() string {
	return "Remove a tile from the main dashboard by its tile ID."
}

func (t *UnpinFromDashboardTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tile_id": map[string]any{
				"type":        "string",
				"description": "The ID of the tile to remove from the dashboard.",
			},
		},
		"required": []string{"tile_id"},
	}
}

func (t *UnpinFromDashboardTool) Execute(ctx context.Context, args map[string]any) *Result {
	tileID, _ := args["tile_id"].(string)
	if tileID == "" {
		return ErrorResult("tile_id is required")
	}

	if err := t.unpin(ctx, t.tenantID, tileID); err != nil {
		return ErrorResult(fmt.Sprintf("failed to unpin tile: %v", err))
	}

	return TextResult(fmt.Sprintf("Tile %s removed from dashboard", tileID))
}
