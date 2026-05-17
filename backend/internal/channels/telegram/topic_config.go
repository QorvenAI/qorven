// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"strings"
)

// Hierarchical per-topic configuration for Telegram forum groups.

// TopicConfig holds per-group and per-topic overrides.
type TopicConfig struct {
	Groups map[string]*GroupConfig `json:"groups"` // chatID → config
}

type GroupConfig struct {
	Enabled        *bool              `json:"enabled"`
	GroupPolicy    string             `json:"group_policy"`    // open, mention_only, admin_only, disabled
	RequireMention *bool              `json:"require_mention"`
	HistoryLimit   int                `json:"history_limit"`
	ToolAllow      []string           `json:"tool_allow"`
	Topics         map[int]*TopicCfg  `json:"topics"` // topicID → config
}

type TopicCfg struct {
	Enabled        *bool    `json:"enabled"`
	RequireMention *bool    `json:"require_mention"`
	HistoryLimit   int      `json:"history_limit"`
	ToolAllow      []string `json:"tool_allow"`
}

// ResolvedConfig is the final merged config for a specific chat+topic.
type ResolvedConfig struct {
	Enabled        bool
	GroupPolicy    string
	RequireMention bool
	HistoryLimit   int
	ToolAllow      []string
}

// ResolveTopicConfig merges: global defaults → wildcard group (*) → specific group → specific topic
func ResolveTopicConfig(cfg Config, topicCfg *TopicConfig, chatID string, topicID int) ResolvedConfig {
	resolved := ResolvedConfig{
		Enabled:        true,
		GroupPolicy:    cfg.GroupPolicy,
		RequireMention: cfg.RequireMention,
		HistoryLimit:   0,
	}

	if topicCfg == nil { return resolved }

	// Layer 1: Wildcard group (*)
	if wildcard, ok := topicCfg.Groups["*"]; ok {
		mergeGroupConfig(&resolved, wildcard)
	}

	// Layer 2: Specific group
	if group, ok := topicCfg.Groups[chatID]; ok {
		mergeGroupConfig(&resolved, group)

		// Layer 3: Specific topic within group
		if topicID > 0 {
			if topic, ok := group.Topics[topicID]; ok {
				mergeTopicConfig(&resolved, topic)
			}
		}
	}

	return resolved
}

func mergeGroupConfig(dst *ResolvedConfig, src *GroupConfig) {
	if src.Enabled != nil { dst.Enabled = *src.Enabled }
	if src.GroupPolicy != "" { dst.GroupPolicy = src.GroupPolicy }
	if src.RequireMention != nil { dst.RequireMention = *src.RequireMention }
	if src.HistoryLimit > 0 { dst.HistoryLimit = src.HistoryLimit }
	if len(src.ToolAllow) > 0 { dst.ToolAllow = src.ToolAllow }
}

func mergeTopicConfig(dst *ResolvedConfig, src *TopicCfg) {
	if src.Enabled != nil { dst.Enabled = *src.Enabled }
	if src.RequireMention != nil { dst.RequireMention = *src.RequireMention }
	if src.HistoryLimit > 0 { dst.HistoryLimit = src.HistoryLimit }
	if len(src.ToolAllow) > 0 { dst.ToolAllow = src.ToolAllow }
}

var _ = strings.Contains // ensure import
