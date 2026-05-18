// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
)

type ChatIntent string

const (
	ChatIntentChat     ChatIntent = "chat"
	ChatIntentCode     ChatIntent = "code"
	ChatIntentResearch ChatIntent = "research"
	ChatIntentCreative ChatIntent = "creative"
)

func ClassifyChatIntent(msg string) ChatIntent {
	lower := strings.ToLower(msg)

	selfKeywords := []string{"qorven", "this platform", "our platform",
		"your capabilities", "what can you do", "your tools", "your agents",
		"how do you work", "what are you", "who are you", "about yourself"}
	for _, kw := range selfKeywords {
		if strings.Contains(lower, kw) {
			return ChatIntentChat
		}
	}

	// Scheduling / reminder keywords — must be checked before the code
	// keywords because "set up", "remind", "schedule", "daily", "every day"
	// contain words ("set", "run", etc.) that would otherwise trigger ChatIntentCode.
	schedulingKeywords := []string{
		"remind me", "reminder", "set a reminder", "set up a reminder",
		"schedule", "scheduled", "scheduling",
		"daily", "every day", "every morning", "every night", "every hour",
		"every week", "every monday", "recurring", "repeat", "at noon",
		"at midnight", "at 9am", "at 6pm", "motivational", "quote",
		"send me", "message me", "notify me", "notification",
		"cron", "alarm", "wake me",
	}
	for _, kw := range schedulingKeywords {
		if strings.Contains(lower, kw) {
			return ChatIntentChat
		}
	}

	switch {
	case strings.Contains(lower, "code") || strings.Contains(lower, "function") ||
		strings.Contains(lower, "debug") || strings.Contains(lower, "implement") ||
		strings.Contains(lower, "```") || strings.Contains(lower, "error") ||
		strings.Contains(lower, "run") || strings.Contains(lower, "execute") ||
		strings.Contains(lower, "server") || strings.Contains(lower, "script") ||
		strings.Contains(lower, "build") || strings.Contains(lower, "install") ||
		strings.Contains(lower, "npm") || strings.Contains(lower, "go build") ||
		strings.Contains(lower, "write a") || strings.Contains(lower, "create a") ||
		strings.Contains(lower, "fix") || strings.Contains(lower, "refactor") ||
		strings.Contains(lower, "file") || strings.Contains(lower, "directory"):
		return ChatIntentCode
	case strings.Contains(lower, "research") || strings.Contains(lower, "search for") ||
		strings.Contains(lower, "look up") || strings.Contains(lower, "latest") ||
		strings.Contains(lower, "what is") || strings.Contains(lower, "how does"):
		return ChatIntentResearch
	case strings.Contains(lower, "story") || strings.Contains(lower, "poem") ||
		strings.Contains(lower, "creative"):
		return ChatIntentCreative
	default:
		return ChatIntentChat
	}
}

// Tool sets by intent — like Cursor, only send relevant tools.
// These allowlists control what goes on the wire (tools:[...] in the API call).
// A tool not in this list is structurally unreachable for that intent —
// the LLM never sees its schema, so it cannot call it.
//
// JIT discovery tools (list_connector_actions, list_mcp_tools) are included in
// every intent set so agents can always discover what connectors/MCP servers exist.
// execute_action is included wherever connectors may be needed.
var intentTools = map[ChatIntent]map[string]bool{
	ChatIntentCode: {
		// Core coding tools
		"exec": true, "read_file": true, "write_file": true, "edit": true,
		"list_files": true, "glob": true, "grep": true, "diagnostics": true,
		"apply_patch": true, "undo": true, "lsp": true, "project": true,
		"prime_coder": true, "project_manager": true,
		// Web: only fetch (have URL), not search (no web browsing during coding)
		"web_fetch": true,
		// Self-improvement tools
		"self_knowledge": true, "self_patch": true, "self_test": true, "self_improve": true,
		// Memory
		"memory_search": true,
		// Scheduling — agents create reminders/crons during coding tasks too
		"cron": true, "datetime": true,
		// Messaging — agents send status updates
		"send_dm": true, "send_telegram": true,
		// JIT discovery (always available)
		"list_connector_actions": true, "list_mcp_tools": true, "execute_action": true,
	},
	ChatIntentResearch: {
		// Web tools — full set for research
		"web_search": true, "web_fetch": true, "research": true, "qor_crawl": true,
		// File: read/write for saving results
		"memory_search": true, "write_file": true, "read_file": true,
		// Scheduling — agents can set up recurring research briefs
		"cron": true, "datetime": true,
		// Messaging
		"send_dm": true, "send_telegram": true,
		// JIT discovery (always available)
		"list_connector_actions": true, "list_mcp_tools": true, "execute_action": true,
	},
	ChatIntentChat: {
		// Conversational + common utility tools
		"web_search": true, "web_fetch": true, "datetime": true,
		"memory_search": true, "memory_get": true, "flight_search": true,
		"email_send": true, "email_read": true,
		"send_dm": true, "send_telegram": true,
		"delegate": true, "exec": true, "read_file": true,
		// Scheduling — "remind me", "set a daily quote", "schedule X at noon" all land here
		"cron": true,
		// File tools — "create a file" is often chat-classified
		"write_file": true, "list_files": true, "edit": true,
		// Room + team tools always available
		"room_post": true, "room_list": true, "room_assign": true, "room_decide": true,
		"delegate_to_soul": true, "create_soul": true, "list_souls": true,
		// Project / workspace builder — "build me a CRM" is chat-intent
		"project": true, "project_manager": true, "workspace_builder": true,
		// Connector tools — users often ask agents to do things via integrations
		"execute_action": true,
		// JIT discovery (always available)
		"list_connector_actions": true, "list_mcp_tools": true,
	},
	ChatIntentCreative: {
		"web_search": true, "web_fetch": true, "write_file": true,
		"create_image": true, "tts": true, "memory_search": true,
		// Scheduling — "post a poem every morning" is creative + scheduling
		"cron": true, "datetime": true,
		"send_dm": true, "send_telegram": true,
		// JIT discovery (always available)
		"list_connector_actions": true, "list_mcp_tools": true, "execute_action": true,
	},
}

// GateToolsByIntent filters tools to only those relevant for the intent.
// This is the #1 fix for context bloat — 68 tools → 12-15 per intent.
func GateToolsByIntent(toolDefs []providers.ToolDefinition, intent ChatIntent) []providers.ToolDefinition {
	allowed, ok := intentTools[intent]
	if !ok {
		allowed = intentTools[ChatIntentChat]
	}

	var filtered []providers.ToolDefinition
	for _, td := range toolDefs {
		if allowed[td.Function.Name] {
			filtered = append(filtered, td)
		}
	}

	// Safety: if filtering removed everything, return a minimal set
	if len(filtered) == 0 {
		return toolDefs
	}
	return filtered
}

func EnrichMessages(_ context.Context, messages []providers.Message, userMsg string, _ ChatIntent, memResults []memory.SearchResult, _ []string) []providers.Message {
	if len(memResults) == 0 {
		return messages
	}
	// Cap memory to 5 results, 200 chars each
	if len(memResults) > 5 {
		memResults = memResults[:5]
	}
	for i := range memResults {
		if len(memResults[i].Memory.Content) > 200 {
			memResults[i].Memory.Content = memResults[i].Memory.Content[:200] + "..."
		}
	}
	memCtx := memory.FormatForContext(memResults)
	enriched := make([]providers.Message, 0, len(messages)+1)
	enriched = append(enriched, messages...)
	enriched = append(enriched, providers.Message{Role: "system", Content: "## Relevant Memories\n" + memCtx})
	return enriched
}
