// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"slices"

	"github.com/qorvenai/qorven/internal/providers"
)

// ToolFilter filters tool definitions based on various criteria.
type ToolFilter struct {
	disabledTools     map[string]bool
	bootstrapAllowlist map[string]bool
	skillEvolve       bool
	agentType         string
}

// NewToolFilter creates a new tool filter.
func NewToolFilter() *ToolFilter {
	return &ToolFilter{
		bootstrapAllowlist: map[string]bool{
			"write_file": true,
			"read_file":  true,
			"list_files": true,
		},
	}
}

// SetDisabledTools sets tools that should be excluded.
func (f *ToolFilter) SetDisabledTools(disabled map[string]bool) {
	f.disabledTools = disabled
}

// SetSkillEvolve sets whether skill_manage should be visible.
func (f *ToolFilter) SetSkillEvolve(enabled bool) {
	f.skillEvolve = enabled
}

// SetAgentType sets the agent type for bootstrap filtering.
func (f *ToolFilter) SetAgentType(agentType string) {
	f.agentType = agentType
}

// FilterRequest holds parameters for tool filtering.
type FilterRequest struct {
	HadBootstrap bool
	Iteration    int
	MaxIteration int
	ChannelType  string
	ToolAllow    []string // per-group tool allow list
}

// FilterResult holds the filtered tools and any messages to inject.
type FilterResult struct {
	ToolDefs     []providers.ToolDefinition
	AllowedTools map[string]bool
	InjectMsg    *providers.Message // message to inject (e.g., final iteration hint)
}

// Filter applies all filtering rules to tool definitions.
func (f *ToolFilter) Filter(toolDefs []providers.ToolDefinition, req FilterRequest) FilterResult {
	result := FilterResult{
		AllowedTools: make(map[string]bool),
	}

	// Start with all tools
	filtered := make([]providers.ToolDefinition, 0, len(toolDefs))

	for _, td := range toolDefs {
		name := td.Function.Name

		// Skip disabled tools
		if f.disabledTools != nil && f.disabledTools[name] {
			continue
		}

		// Bootstrap mode: restrict to allowlist (open agents only)
		if req.HadBootstrap && f.agentType != "predefined" {
			if !f.bootstrapAllowlist[name] {
				continue
			}
		}

		// Hide skill_manage when skill_evolve is off
		if !f.skillEvolve && name == "skill_manage" {
			continue
		}

		// Per-group tool allow list
		if len(req.ToolAllow) > 0 && !slices.Contains(req.ToolAllow, name) {
			continue
		}

		filtered = append(filtered, td)
		result.AllowedTools[name] = true
	}

	// Final iteration: strip all tools to force text response
	if req.Iteration == req.MaxIteration && req.MaxIteration > 0 {
		result.ToolDefs = nil
		result.InjectMsg = &providers.Message{
			Role:    "user",
			Content: "[System] Final iteration reached. Summarize all findings and respond to the user now. No more tool calls allowed.",
		}
		return result
	}

	result.ToolDefs = filtered
	return result
}

// FilterByChannelType removes tools that don't match the channel type.
func FilterByChannelType(toolDefs []providers.ToolDefinition, channelType string, channelAwareTools map[string][]string) []providers.ToolDefinition {
	if channelType == "" || len(channelAwareTools) == 0 {
		return toolDefs
	}

	filtered := make([]providers.ToolDefinition, 0, len(toolDefs))
	for _, td := range toolDefs {
		requiredChannels, isChannelAware := channelAwareTools[td.Function.Name]
		if isChannelAware && !slices.Contains(requiredChannels, channelType) {
			continue
		}
		filtered = append(filtered, td)
	}
	return filtered
}

// FilterByIntent removes tools based on chat intent.
// For example, web_search is not needed for simple chat queries.
func FilterByIntent(toolDefs []providers.ToolDefinition, intent ChatIntent) []providers.ToolDefinition {
	if intent == "" {
		return toolDefs
	}

	// Tools to exclude by intent
	excludeByIntent := map[ChatIntent][]string{
		ChatIntentChat: {"web_search", "web_fetch", "qor_crawl"},
	}

	excluded := excludeByIntent[intent]
	if len(excluded) == 0 {
		return toolDefs
	}

	filtered := make([]providers.ToolDefinition, 0, len(toolDefs))
	for _, td := range toolDefs {
		if slices.Contains(excluded, td.Function.Name) {
			continue
		}
		filtered = append(filtered, td)
	}
	return filtered
}

// BuildAllowedToolsMap creates a map of allowed tool names from definitions.
func BuildAllowedToolsMap(toolDefs []providers.ToolDefinition) map[string]bool {
	allowed := make(map[string]bool, len(toolDefs))
	for _, td := range toolDefs {
		allowed[td.Function.Name] = true
	}
	return allowed
}
