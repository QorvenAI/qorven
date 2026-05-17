// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import "strings"

// Tool groups map group names to tool names.
var ToolGroups = map[string][]string{
	"fs":         {"read_file", "write_file", "list_files", "edit"},
	"runtime":    {"exec"},
	"web":        {"web_search", "web_fetch"},
	"memory":     {"memory_search", "memory_get"},
	"knowledge":  {"knowledge_graph_search"},
	"sessions":   {"sessions_list", "sessions_history", "session_status", "spawn"},
	"automation": {"cron", "datetime"},
	"messaging":  {"message"},
	"media":      {"read_image", "read_audio", "read_video", "read_document", "create_image", "create_video", "create_audio", "tts"},
	"team":       {"team_tasks", "team_message"},
	"skills":     {"skill_search"},
	// Platform management — build and modify workspaces, agents, dashboards
	"platform": {"workspace_builder", "qorven_social", "social_monitor"},
	// GitHub — autonomous dev loop: read issues, branch, push, PR, review, merge
	"github": {
		"gh_repo_info",
		"gh_list_issues",
		"gh_read_issue",
		"gh_create_issue",
		"gh_create_branch",
		"gh_push_file",
		"gh_open_pr",
		"gh_post_comment",
		"gh_list_pr_checks",
		"gh_merge_pr",
		"gh_task_register",
		"gh_create_repo",
	},
}

// Tool profiles — preset allow sets.
var ToolProfiles = map[string][]string{
	"minimal":   {"session_status", "datetime"},
	"coding":    {"group:fs", "group:runtime", "group:sessions", "group:memory", "group:web", "group:skills", "group:github", "read_image", "create_image"},
	"messaging": {"group:messaging", "group:web", "group:sessions", "group:skills", "session_status", "read_image"},
	// supervisor profile: Prime and chief agents — full platform management
	"supervisor": {"group:platform", "group:web", "group:memory", "group:knowledge", "group:sessions", "group:automation", "group:team", "group:skills"},
	"full":       {}, // empty = no restrictions
}

// Subagent deny list — tools subagents cannot use.
var SubagentDenyList = []string{"exec", "cron", "memory_search", "memory_get", "message"}

// FilterTools returns allowed ToolDefinitions based on policy.
func FilterTools(reg *Registry, allow, deny []string, profile string, isSubagent bool) []ToolDefinition {
	allDefs := reg.Definitions()

	// If profile specified, resolve it
	if profile != "" {
		if profileAllow, ok := ToolProfiles[profile]; ok && len(profileAllow) > 0 {
			allow = append(allow, profileAllow...)
		}
	}

	// Resolve group references in allow list
	resolved := resolveGroups(allow)

	// Build allow set (empty = allow all)
	allowSet := make(map[string]bool)
	for _, name := range resolved { allowSet[name] = true }

	// Build deny set
	denySet := make(map[string]bool)
	for _, name := range deny { denySet[name] = true }
	if isSubagent {
		for _, name := range SubagentDenyList { denySet[name] = true }
	}

	var filtered []ToolDefinition
	for _, def := range allDefs {
		name := def.Function.Name
		if denySet[name] { continue }
		if len(allowSet) > 0 && !allowSet[name] { continue }
		filtered = append(filtered, def)
	}
	return filtered
}

// resolveGroups expands "group:fs" → ["read_file", "write_file", "list_files", "edit"]
func resolveGroups(names []string) []string {
	var resolved []string
	for _, name := range names {
		if strings.HasPrefix(name, "group:") {
			groupName := strings.TrimPrefix(name, "group:")
			if members, ok := ToolGroups[groupName]; ok {
				resolved = append(resolved, members...)
			}
		} else {
			resolved = append(resolved, name)
		}
	}
	return resolved
}
