// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
)

// BuiltinToolSettings maps tool names to their JSON settings.
type BuiltinToolSettings map[string]json.RawMessage

// HasReadImageProvider checks if the read_image builtin tool has a dedicated provider configured.
// When true, images should NOT be attached inline to the main LLM — instead the agent
// uses the read_image tool which routes to the configured vision provider.
func HasReadImageProvider(settings BuiltinToolSettings) bool {
	if settings == nil {
		return false
	}
	raw, ok := settings["read_image"]
	if !ok || len(raw) == 0 {
		return false
	}

	// Try chain format first: {"providers":[{"provider":"X",...}]}
	var chain struct {
		Providers []struct {
			Provider string `json:"provider"`
			Enabled  *bool  `json:"enabled,omitempty"`
		} `json:"providers"`
	}
	if json.Unmarshal(raw, &chain) == nil && len(chain.Providers) > 0 {
		for _, p := range chain.Providers {
			if p.Provider != "" && (p.Enabled == nil || *p.Enabled) {
				return true
			}
		}
	}

	// Fallback: legacy flat format {"provider":"X"}
	var legacy struct {
		Provider string `json:"provider"`
	}
	if json.Unmarshal(raw, &legacy) == nil && legacy.Provider != "" {
		return true
	}

	return false
}

// LoadHistoricalImagesForTool collects image MediaRefs from historical messages
// and loads them into context for the read_image tool.
func LoadHistoricalImagesForTool(ctx context.Context, currentImages []string, historyRefs []MediaRef, maxMessages int) context.Context {
	if len(historyRefs) == 0 {
		return ctx
	}

	// Collect image paths from historical refs
	var histPaths []string
	count := 0
	for _, ref := range historyRefs {
		if ref.Kind != "image" || ref.Path == "" {
			continue
		}
		histPaths = append(histPaths, ref.Path)
		count++
		if count >= maxMessages {
			break
		}
	}

	if len(histPaths) == 0 {
		return ctx
	}

	// Merge: existing (current turn) + historical
	merged := make([]string, 0, len(currentImages)+len(histPaths))
	merged = append(merged, currentImages...)
	merged = append(merged, histPaths...)
	slog.Debug("vision: loaded historical images for read_image tool",
		"current", len(currentImages), "historical", len(histPaths))
	return WithMediaImages(ctx, merged)
}

// MediaToolRouting determines how media should be routed to tools.
type MediaToolRouting struct {
	DeferToReadImage bool // images accessed via read_image tool, not inline
	DeferToReadAudio bool // audio accessed via read_audio tool
	DeferToReadVideo bool // video accessed via read_video tool
}

// ResolveMediaToolRouting determines routing based on builtin tool settings.
func ResolveMediaToolRouting(settings BuiltinToolSettings) MediaToolRouting {
	return MediaToolRouting{
		DeferToReadImage: HasReadImageProvider(settings),
		DeferToReadAudio: hasToolProvider(settings, "read_audio"),
		DeferToReadVideo: hasToolProvider(settings, "read_video"),
	}
}

func hasToolProvider(settings BuiltinToolSettings, toolName string) bool {
	if settings == nil {
		return false
	}
	raw, ok := settings[toolName]
	if !ok || len(raw) == 0 {
		return false
	}
	var cfg struct {
		Provider string `json:"provider"`
	}
	if json.Unmarshal(raw, &cfg) == nil && cfg.Provider != "" {
		return true
	}
	return false
}
