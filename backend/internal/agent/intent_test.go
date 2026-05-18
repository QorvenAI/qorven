// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

// allTools is a large synthetic tool set that approximates production (52+ tools).
var allTools = func() []providers.ToolDefinition {
	names := []string{
		// coding
		"exec", "read_file", "write_file", "edit", "list_files", "glob", "grep",
		"diagnostics", "apply_patch", "undo", "lsp", "project", "prime_coder",
		"project_manager", "self_knowledge", "self_patch", "self_test", "self_improve",
		// web
		"web_search", "web_fetch", "qor_crawl", "research",
		// connectors
		"execute_action", "list_connector_actions", "list_mcp_tools",
		// memory
		"memory_search", "memory_get",
		// social / media
		"email_send", "email_read", "send_telegram", "room_post", "room_list",
		"room_assign", "room_decide", "create_image", "tts",
		// workspace / team
		"delegate", "delegate_to_soul", "create_soul", "list_souls",
		"workspace_builder", "qorven_social",
		// scheduling / finance
		"flight_search", "datetime", "cron", "create_calendar_event",
		// admin tools that should never appear in casual chat
		"sql_query", "sql_schema", "shell_exec", "delete_file", "kv_set", "kv_get",
	}
	defs := make([]providers.ToolDefinition, len(names))
	for i, n := range names {
		defs[i] = providers.ToolDefinition{
			Type: "function",
			Function: providers.ToolFunctionSchema{Name: n},
		}
	}
	return defs
}()

func toolNames(defs []providers.ToolDefinition) map[string]bool {
	m := make(map[string]bool, len(defs))
	for _, d := range defs {
		m[d.Function.Name] = true
	}
	return m
}

// TestGateToolsByIntent_StructuralEnforcement verifies that GateToolsByIntent
// produces a wire tool list (tools:[...]) restricted to the intent's allowlist.
// These are structural gates — a tool absent from the result cannot be called
// by the LLM because its schema is never sent.
func TestGateToolsByIntent_StructuralEnforcement(t *testing.T) {
	tests := []struct {
		intent        ChatIntent
		mustInclude   []string // must be in the filtered set
		mustExclude   []string // must NOT be in the filtered set
		maxTools      int      // upper bound on gated set size
	}{
		{
			intent: ChatIntentChat,
			// cron + send_dm + workspace_builder are core chat capabilities
			mustInclude: []string{"web_search", "memory_search", "execute_action", "list_connector_actions", "cron", "send_telegram", "workspace_builder"},
			mustExclude: []string{"qorven_social", "sql_query", "shell_exec"},
			maxTools:    35,
		},
		{
			intent: ChatIntentCode,
			// cron + send_dm added for scheduling status updates during coding
			mustInclude: []string{"exec", "edit", "read_file", "list_connector_actions", "cron", "send_telegram"},
			mustExclude: []string{"web_search", "qor_crawl", "flight_search", "create_image", "tts"},
			maxTools:    28,
		},
		{
			intent: ChatIntentResearch,
			// cron added so agents can schedule recurring research briefs
			mustInclude: []string{"web_search", "web_fetch", "qor_crawl", "list_connector_actions", "cron"},
			mustExclude: []string{"exec", "create_image", "tts", "workspace_builder"},
			maxTools:    18,
		},
		{
			intent: ChatIntentCreative,
			// cron added for "post a poem every morning" patterns
			mustInclude: []string{"web_search", "create_image", "tts", "cron"},
			mustExclude: []string{"exec", "sql_query", "shell_exec", "workspace_builder"},
			maxTools:    18,
		},
	}

	for _, tc := range tests {
		t.Run(string(tc.intent), func(t *testing.T) {
			gated := GateToolsByIntent(allTools, tc.intent)
			names := toolNames(gated)

			for _, name := range tc.mustInclude {
				if !names[name] {
					t.Errorf("intent=%s: expected %q in wire tools but it was excluded", tc.intent, name)
				}
			}
			for _, name := range tc.mustExclude {
				if names[name] {
					t.Errorf("intent=%s: %q should be excluded from wire tools (structural gate)", tc.intent, name)
				}
			}
			if len(gated) > tc.maxTools {
				t.Errorf("intent=%s: %d tools on wire, want ≤%d (spec cap to reduce schema bloat)",
					tc.intent, len(gated), tc.maxTools)
			}
		})
	}
}

// TestGateToolsByIntent_JITToolsAlwaysPresent verifies that the JIT discovery
// tools are included in every intent — agents must always be able to look up
// what connectors/MCP servers are available.
func TestGateToolsByIntent_JITToolsAlwaysPresent(t *testing.T) {
	jitTools := []string{"list_connector_actions", "list_mcp_tools"}
	intents := []ChatIntent{ChatIntentChat, ChatIntentCode, ChatIntentResearch, ChatIntentCreative}

	for _, intent := range intents {
		gated := GateToolsByIntent(allTools, intent)
		names := toolNames(gated)
		for _, jit := range jitTools {
			if !names[jit] {
				t.Errorf("intent=%s: JIT tool %q must always be on the wire", intent, jit)
			}
		}
	}
}

// TestGateToolsByIntent_SafetyFallback verifies the safety net: if the intent
// filter would remove every tool, the original set is returned unchanged.
func TestGateToolsByIntent_SafetyFallback(t *testing.T) {
	onlyUnknown := []providers.ToolDefinition{
		{Type: "function", Function: providers.ToolFunctionSchema{Name: "tool_not_in_any_list"}},
	}
	gated := GateToolsByIntent(onlyUnknown, ChatIntentCode)
	if len(gated) == 0 {
		t.Error("GateToolsByIntent returned empty set — safety fallback must return original tools")
	}
}
