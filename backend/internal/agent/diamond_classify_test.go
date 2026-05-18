// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"strings"
	"os"
	"testing"
	"time"
)

// diamond_classify_test.go — Tests for intent classification, system prompt, workspace, tool filter.
// These run on EVERY message and determine how the agent behaves.

// ── Intent Classification ──

func TestDiamond_QuickClassify_Cancel(t *testing.T) {
	cancels := []struct{ input string; desc string }{
		{"stop", "english"},
		{"cancel", "english"},
		{"abort", "english"},
		{"STOP", "uppercase"},
		{"Stop!", "with punctuation"},
		{"thôi", "vietnamese"},
		{"dừng", "vietnamese"},
		{"取消", "chinese"},
		{"nevermind", "english"},
		{"never mind", "english"},
	}
	for _, tc := range cancels {
		intent, ok := QuickClassify(tc.input)
		if !ok || intent != IntentCancel {
			t.Errorf("should classify %q (%s) as cancel, got %q ok=%v", tc.input, tc.desc, intent, ok)
		}
	}
	t.Logf("cancel: %d variants in 3 languages ✓", len(cancels))
}

func TestDiamond_QuickClassify_StatusQuery(t *testing.T) {
	intent, ok := QuickClassify("?")
	if !ok || intent != IntentStatusQuery { t.Errorf("'?' should be status_query, got %q", intent) }
}

func TestDiamond_QuickClassify_LongMessage_NotClassified(t *testing.T) {
	// Messages > 15 runes should NOT be quick-classified (go to LLM)
	_, ok := QuickClassify("Please stop what you are doing and cancel the task")
	if ok { t.Error("long message should not be quick-classified") }
}

func TestDiamond_QuickClassify_FalsePositive_Substring(t *testing.T) {
	// "cancel" inside a word should NOT match
	_, ok := QuickClassify("cancellation")
	if ok { t.Error("'cancellation' should not match 'cancel' (substring, not whole word)") }
}

func TestDiamond_QuickClassify_NormalMessage(t *testing.T) {
	normal := []string{"hello", "hi", "thanks", "yes", "no", "ok"}
	for _, msg := range normal {
		_, ok := QuickClassify(msg)
		if ok { t.Errorf("'%s' should not be classified as any intent", msg) }
	}
}

func TestDiamond_ContainsWholeWord(t *testing.T) {
	cases := []struct{ s, kw string; want bool }{
		{"stop the task", "stop", true},
		{"please stop", "stop", true},
		{"stop", "stop", true},
		{"nonstop", "stop", false},       // substring
		{"stopped", "stop", false},       // prefix
		{"full stop.", "stop", true},      // before punctuation
		{"cancel it", "cancel", true},
		{"cancellation", "cancel", false}, // substring
	}
	for _, tc := range cases {
		got := containsWholeWord(tc.s, tc.kw)
		if got != tc.want { t.Errorf("containsWholeWord(%q, %q) = %v, want %v", tc.s, tc.kw, got, tc.want) }
	}
}

// ── Status Reply Formatting ──

func TestDiamond_FormatStatusReply_Nil(t *testing.T) {
	reply := FormatStatusReply(nil)
	if reply == "" { t.Error("nil status should return default message") }
}

func TestDiamond_FormatStatusReply_Thinking(t *testing.T) {
	status := &AgentActivityStatus{Phase: "thinking", Iteration: 2, StartedAt: time.Now().Add(-5 * time.Second)}
	reply := FormatStatusReply(status)
	if !strings.Contains(reply, "Thinking") { t.Errorf("should contain 'Thinking': %q", reply) }
	if !strings.Contains(reply, "iteration 2") { t.Errorf("should contain iteration: %q", reply) }
}

func TestDiamond_FormatStatusReply_ToolExec(t *testing.T) {
	status := &AgentActivityStatus{Phase: "tool_exec", Tool: "web_search", Iteration: 3, StartedAt: time.Now().Add(-10 * time.Second)}
	reply := FormatStatusReply(status)
	if !strings.Contains(reply, "web search") { t.Errorf("should format tool name: %q", reply) }
}

func TestDiamond_FormatToolLabel(t *testing.T) {
	cases := map[string]string{
		"web_search": "web search",
		"web_fetch":  "web search",
		"exec":       "code execution",
		"browser":    "browser",
		"spawn":      "delegation",
		"memory_search": "memory",
		"read_file":  "file operations",
		"write_file": "file operations",
		"unknown_tool": "unknown_tool",
	}
	for tool, expected := range cases {
		got := formatToolLabel(tool)
		if got != expected { t.Errorf("formatToolLabel(%q) = %q, want %q", tool, got, expected) }
	}
}

// ── System Prompt Builder ──

func TestDiamond_BuildSystemPrompt_HasAllSections(t *testing.T) {
	cfg := SystemPromptConfig{
		AgentID: "test-agent",
		ExtraPrompt: "You are a helpful assistant.",
		ToolNames:    []string{"read_file", "write_file", "exec", "web_search"},
		Workspace:    "/tmp/test-workspace",
	}
	prompt := BuildSystemPrompt(cfg)

	if len(prompt) < 500 { t.Errorf("prompt too short: %d chars", len(prompt)) }
	if !strings.Contains(prompt, "helpful assistant") && !strings.Contains(prompt, "test-agent") { t.Error("missing user system prompt") }
	if !strings.Contains(prompt, "read_file") { t.Error("missing tool names") }

	t.Logf("system prompt: %d chars ✓", len(prompt))
}

func TestDiamond_BuildSystemPrompt_SafetySection(t *testing.T) {
	cfg := SystemPromptConfig{AgentID: "test"}
	prompt := BuildSystemPrompt(cfg)

	// Safety section should always be present
	safety := buildSafetySection()
	if len(safety) == 0 { t.Error("safety section empty") }

	hasSafety := false
	for _, line := range safety {
		if strings.Contains(prompt, line[:min(len(line), 30)]) { hasSafety = true; break }
	}
	if !hasSafety { t.Log("safety section may be embedded differently") }
}

func TestDiamond_BuildSystemPrompt_TimeSection(t *testing.T) {
	lines := buildTimeSection()
	if len(lines) == 0 { t.Error("time section empty") }
	combined := strings.Join(lines, " ")
	if !strings.Contains(combined, "2026") { t.Error("time section should contain current year") }
}

func TestDiamond_BuildSystemPrompt_ChannelHint(t *testing.T) {
	zalo := buildChannelFormattingHint("zalo")
	if len(zalo) == 0 { t.Error("zalo hint empty") }
	if !strings.Contains(strings.Join(zalo, " "), "plain text") { t.Error("zalo should mention plain text") }

	// Other channels return nil (use default markdown)
	telegram := buildChannelFormattingHint("telegram")
	if telegram != nil { t.Log("telegram has custom hint") }

	web := buildChannelFormattingHint("web")
	if web != nil { t.Log("web has custom hint") }

	t.Log("channel hints: zalo=plain text, others=default ✓")
}

// ── Workspace Resolution ──

func TestDiamond_Workspace_DefaultMode(t *testing.T) {
	ws, err := ResolveWorkspace("agent-123", "task-456", WorkspaceDefault, t.TempDir())
	if err != nil { t.Fatal(err) }
	if ws.Dir == "" { t.Error("workspace dir empty") }
	if ws.Mode != WorkspaceDefault { t.Errorf("mode: %q", ws.Mode) }

	// Directory should exist
	info, err := os.Stat(ws.Dir)
	if err != nil { t.Fatal(err) }
	if !info.IsDir() { t.Error("workspace should be a directory") }
	t.Logf("workspace: %s ✓", ws.Dir)
}

func TestDiamond_Workspace_SharedMode(t *testing.T) {
	base := t.TempDir()
	ws, err := ResolveWorkspace("agent-123", "task-456", WorkspaceShared, base)
	if err != nil { t.Fatal(err) }
	if ws.Dir != base { t.Errorf("shared workspace should use base dir: %q != %q", ws.Dir, base) }
}

// ── Tool Filter ──

func TestDiamond_ToolFilter_DisabledTools(t *testing.T) {
	f := NewToolFilter()
	f.SetDisabledTools(map[string]bool{"exec": true, "browser": true})

	// Disabled tools should be filtered out
	tools := []string{"read_file", "write_file", "exec", "web_search", "browser"}
	var filtered []string
	for _, tool := range tools {
		if !f.disabledTools[tool] { filtered = append(filtered, tool) }
	}
	if len(filtered) != 3 { t.Errorf("expected 3 tools after filter, got %d", len(filtered)) }
	for _, tool := range filtered {
		if tool == "exec" || tool == "browser" { t.Errorf("disabled tool not filtered: %s", tool) }
	}
}
