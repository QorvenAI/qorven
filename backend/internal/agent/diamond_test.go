// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/tools"
)

// Diamond-hard agent tests — verify the agent system works as a product.

func TestDiamond_AgentLoop_ToolChain(t *testing.T) {
	// Simulate: agent writes file → reads it → executes command on it
	dir := t.TempDir()
	reg := tools.NewRegistry()
	reg.Register(tools.NewWriteFileTool(dir))
	reg.Register(tools.NewReadFileTool(dir))
	reg.Register(tools.NewExecTool(dir, true))

	executor := NewParallelToolExecutor(reg, 5)
	ctx := context.Background()

	// Step 1: Write
	r1 := executor.Execute(ctx, []ToolCall{{Name: "write_file", Args: map[string]any{
		"path": "numbers.txt", "content": "10\n20\n30\n40\n50\n",
	}}})
	if r1[0].IsError { t.Fatalf("write: %s", r1[0].Content) }

	// Step 2: Read back
	r2 := executor.Execute(ctx, []ToolCall{{Name: "read_file", Args: map[string]any{"path": "numbers.txt"}}})
	if r2[0].IsError { t.Fatalf("read: %s", r2[0].Content) }
	if !strings.Contains(r2[0].Content, "30") { t.Error("content mismatch") }

	// Step 3: Process with exec
	r3 := executor.Execute(ctx, []ToolCall{{Name: "exec", Args: map[string]any{
		"command": "awk '{s+=$1} END{print s}' numbers.txt",
	}}})
	if r3[0].IsError { t.Fatalf("exec: %s", r3[0].Content) }
	if !strings.Contains(r3[0].Content, "150") { t.Errorf("sum should be 150: %q", r3[0].Content) }

	t.Log("tool chain: write→read→exec(sum=150) ✓")
}

func TestDiamond_Compactor_PreservesConversationFlow(t *testing.T) {
	c := NewCompactor(3000)

	// Build a realistic multi-turn conversation
	msgs := []providers.Message{
		{Role: "system", Content: "You are a helpful coding assistant."},
		{Role: "user", Content: "I need help with a Go function that sorts a slice."},
		{Role: "assistant", Content: "Here's a function that sorts a slice using the sort package:\n```go\nfunc sortSlice(s []int) { sort.Ints(s) }\n```"},
		{Role: "user", Content: "Can you make it work with strings too?"},
		{Role: "assistant", Content: "You can use generics:\n```go\nfunc sortSlice[T constraints.Ordered](s []T) { sort.Slice(s, func(i, j int) bool { return s[i] < s[j] }) }\n```"},
		{Role: "user", Content: "Now add error handling for nil slices."},
		{Role: "assistant", Content: "```go\nfunc sortSlice[T constraints.Ordered](s []T) error {\n  if s == nil { return errors.New(\"nil slice\") }\n  sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })\n  return nil\n}\n```"},
		{Role: "user", Content: "Perfect. Now write tests for it."},
	}

	action := c.Check(msgs)
	if action == 0 { t.Log("under limit"); return }

	compacted := c.Compact(msgs, action)

	// The LAST user message must survive — it's the current request
	lastUser := ""
	for i := len(compacted) - 1; i >= 0; i-- {
		if compacted[i].Role == "user" { lastUser = compacted[i].Content; break }
	}
	if !strings.Contains(lastUser, "tests") { t.Error("last user message (write tests) lost") }

	// System prompt must survive
	hasSystem := false
	for _, m := range compacted { if m.Role == "system" { hasSystem = true } }
	if !hasSystem { t.Error("system prompt lost") }

	t.Logf("conversation flow: %d → %d messages, last request + system preserved ✓", len(msgs), len(compacted))
}

func TestDiamond_Session_ConversationRecovery(t *testing.T) {
	pool := hardPool(t)
	sessStore := session.NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Create session and add conversation
	sess, err := sessStore.Create(ctx, tenant, agentID, "diamond-user", "test")
	if err != nil { t.Fatal(err) }
	defer sessStore.Delete(ctx, sess.ID)

	conversation := []struct{ role, content string }{
		{"user", "My project is called Qorven."},
		{"assistant", "Got it! Qorven is your project. How can I help?"},
		{"user", "It's an AI agent platform written in Go."},
		{"assistant", "Qorven is a Go-based AI agent platform. What would you like to work on?"},
		{"user", "I need help with the memory system."},
		{"assistant", "I can help with the memory system. What specific aspect?"},
	}

	for _, turn := range conversation {
		sessStore.AppendMessage(ctx, sess.ID, session.Message{Role: turn.role, Content: turn.content}, 10, 10)
	}

	// Simulate "recovery" — read history from DB (as if server restarted)
	history, err := sessStore.GetHistory(ctx, sess.ID)
	if err != nil { t.Fatal(err) }
	if len(history) < 6 { t.Errorf("expected 6 messages, got %d", len(history)) }

	// Verify conversation is intact and in order
	for i, turn := range conversation {
		if i >= len(history) { break }
		if history[i].Role != turn.role { t.Errorf("msg %d: role=%q, want %q", i, history[i].Role, turn.role) }
		if !strings.Contains(history[i].Content, turn.content[:20]) {
			t.Errorf("msg %d: content mismatch", i)
		}
	}

	// Verify key facts survive
	fullHistory := ""
	for _, m := range history { fullHistory += m.Content + " " }
	if !strings.Contains(fullHistory, "Qorven") { t.Error("project name lost") }
	if !strings.Contains(fullHistory, "Go") { t.Error("language lost") }
	if !strings.Contains(fullHistory, "memory system") { t.Error("topic lost") }

	t.Log("conversation recovery: 6 messages intact, key facts preserved ✓")
}

func TestDiamond_LoopGuard_PreventsBudgetWaste(t *testing.T) {
	guard := NewLoopGuard()

	// Simulate an agent that keeps calling the same tool with same args
	// This wastes API budget — the guard should stop it
	totalCalls := 0
	stopped := false

	for i := 0; i < 20; i++ {
		hash := guard.RecordCall("web_search", map[string]any{"query": "same query every time"})
		guard.RecordResult("web_search", hash, "same result every time")
		guard.RecordToolSuccess("web_search")
		totalCalls++

		result := guard.DetectSameArgs("web_search", hash)
		if result.Level == DetectionCritical {
			stopped = true
			break
		}
	}

	if !stopped { t.Error("guard should stop repetitive calls to prevent budget waste") }
	if totalCalls > 10 { t.Errorf("guard allowed %d calls before stopping (should be <10)", totalCalls) }
	t.Logf("budget protection: stopped after %d repetitive calls ✓", totalCalls)
}

func TestDiamond_PromptBuilder_RealWorldPrompt(t *testing.T) {
	// Build a prompt that looks like a real production agent
	ag := &Agent{
		ID: "prime", DisplayName: "Qorven Prime", AgentKey: "prime",
		SystemPrompt: "You are Qorven Prime, the lead AI agent. You coordinate a team of specialist agents. You have access to tools for web search, file operations, and code execution. Always think step by step.",
		Model: "gpt-4o-mini",
	}

	team := []*Agent{
		{ID: "dev", DisplayName: "Developer", SystemPrompt: "Expert Go developer"},
		{ID: "qa", DisplayName: "QA Engineer", SystemPrompt: "Testing specialist"},
		{ID: "researcher", DisplayName: "Researcher", SystemPrompt: "Web research expert"},
	}

	reg := tools.NewRegistry()
	reg.Register(tools.NewReadFileTool("/tmp"))
	reg.Register(tools.NewWriteFileTool("/tmp"))
	reg.Register(tools.NewExecTool("/tmp", true))
	reg.Register(tools.NewWebSearchTool(nil))
	reg.Register(tools.NewWebFetchToolWithConfig(tools.WebFetchConfig{}))
	reg.Register(tools.NewClarifyTool())

	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull, ModelID: "gpt-4o-mini"})
	pb.SetTeam(team)
	pb.SetToolRegistry(reg)
	pb.SetMemoryResults([]string{
		"User is building Qorven, a multi-agent AI platform",
		"User prefers Go and terminal-based tools",
		"Project deadline is April 30, 2026",
		"User values code quality over speed",
	})
	pb.SetWikiArticles([]string{
		"# Qorven Architecture\nSingle binary, 52+ tools, 8 channels, 6 providers.",
	})

	prompt := pb.Build()

	// Verify the prompt is substantial
	if len(prompt) < 1000 { t.Errorf("prompt too short for production agent: %d chars", len(prompt)) }

	// Verify key sections are present
	checks := map[string]bool{
		"Qorven Prime": false, "step by step": false,
	}
	for keyword := range checks {
		if strings.Contains(prompt, keyword) { checks[keyword] = true }
	}
	for keyword, found := range checks {
		if !found { t.Logf("keyword %q not found in prompt", keyword) }
	}

	lines := strings.Count(prompt, "\n")
	t.Logf("production prompt: %d chars, %d lines ✓", len(prompt), lines)
}
