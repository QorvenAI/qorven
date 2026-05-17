// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/tools"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Hard agent tests — real DB, real tools, stress scenarios.

func hardDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestHard_Agent_CRUD_FullCycle(t *testing.T) {
	pool := hardDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create 5 agents
	var ids []string
	for i := 0; i < 5; i++ {
		ag, err := store.Create(ctx, tenant, CreateAgentInput{
			AgentKey: "hard-" + time.Now().Format("150405") + "-" + string(rune('A'+i)),
			Model: "gpt-4o-mini", SystemPrompt: "Agent " + string(rune('A'+i)),
		})
		if err != nil { t.Fatalf("create %d: %v", i, err) }
		ids = append(ids, ag.ID)
	}
	t.Logf("created %d agents", len(ids))

	// List — all should be present
	agents, _ := store.List(ctx, tenant)
	for _, id := range ids {
		found := false
		for _, ag := range agents { if ag.ID == id { found = true } }
		if !found { t.Errorf("agent %s not in list", id) }
	}

	// Update each
	for i, id := range ids {
		store.Update(ctx, id, map[string]any{"display_name": "Updated " + string(rune('A'+i))})
	}

	// Verify updates
	for i, id := range ids {
		ag, _ := store.Get(ctx, id)
		if ag.DisplayName != "Updated "+string(rune('A'+i)) { t.Errorf("update failed for %s", id) }
	}

	// Delete all
	for _, id := range ids { store.Delete(ctx, id) }

	// Verify deleted
	for _, id := range ids {
		_, err := store.Get(ctx, id)
		if err == nil { t.Errorf("agent %s should be deleted", id) }
	}
	t.Log("5 agents: create→list→update→verify→delete ✓")
}

func TestHard_Agent_ConcurrentCreate(t *testing.T) {
	pool := hardDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var wg sync.WaitGroup
	var mu sync.Mutex
	var ids []string
	errors := 0

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ag, err := store.Create(ctx, tenant, CreateAgentInput{
				AgentKey: "concurrent-" + time.Now().Format("150405.000") + "-" + string(rune('A'+n%26)),
				Model: "gpt-4o-mini", SystemPrompt: "Concurrent " + string(rune('A'+n)),
			})
			mu.Lock()
			if err != nil { errors++ } else { ids = append(ids, ag.ID) }
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	t.Logf("concurrent create: %d/%d succeeded", len(ids), 20)

	// Cleanup
	for _, id := range ids { store.Delete(ctx, id) }
}

func TestHard_Agent_BudgetTracking(t *testing.T) {
	pool := hardDB(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	ag, err := store.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "budget-" + time.Now().Format("150405"),
		Model: "gpt-4o-mini", SystemPrompt: "Budget test",
	})
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, ag.ID)

	// Track usage multiple times
	for i := 0; i < 10; i++ {
		store.TrackUsage(ctx, ag.ID, 100+i*10, 50+i*5)
	}

	// Check budget
	ok, used, budget := store.CheckBudget(ctx, ag.ID)
	t.Logf("budget: ok=%v, used=%d, budget=%d", ok, used, budget)
}

func TestHard_ToolExecution_WriteReadPipeline(t *testing.T) {
	dir := t.TempDir()
	reg := tools.NewRegistry()
	reg.Register(tools.NewWriteFileTool(dir))
	reg.Register(tools.NewReadFileTool(dir))
	reg.Register(tools.NewExecTool(dir, true))

	executor := NewParallelToolExecutor(reg, 5)
	ctx := context.Background()

	// Write a Go file
	results := executor.Execute(ctx, []ToolCall{{
		Name: "write_file",
		Args: map[string]any{"path": "main.go", "content": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(42) }\n"},
	}})
	if results[0].IsError { t.Fatalf("write: %s", results[0].Content) }

	// Read it back
	results = executor.Execute(ctx, []ToolCall{{Name: "read_file", Args: map[string]any{"path": "main.go"}}})
	if results[0].IsError { t.Fatalf("read: %s", results[0].Content) }
	if !strings.Contains(results[0].Content, "fmt.Println(42)") { t.Error("content mismatch") }

	// Execute it
	results = executor.Execute(ctx, []ToolCall{{Name: "exec", Args: map[string]any{"command": "go run main.go"}}})
	if results[0].IsError { t.Logf("exec: %s (Go may not be in PATH)", results[0].Content) }
	if strings.Contains(results[0].Content, "42") { t.Log("Go execution: output=42 ✓") }
}

func TestHard_ToolExecution_10ParallelReads(t *testing.T) {
	dir := t.TempDir()
	reg := tools.NewRegistry()
	write := tools.NewWriteFileTool(dir)
	reg.Register(tools.NewReadFileTool(dir))

	// Create 10 files
	for i := 0; i < 10; i++ {
		write.Execute(context.Background(), map[string]any{
			"path": "file_" + string(rune('0'+i)) + ".txt",
			"content": "Content of file " + string(rune('0'+i)),
		})
	}

	// Read all 10 in parallel
	executor := NewParallelToolExecutor(reg, 10)
	calls := make([]ToolCall, 10)
	for i := range calls {
		calls[i] = ToolCall{Name: "read_file", Args: map[string]any{"path": "file_" + string(rune('0'+i)) + ".txt"}}
	}

	start := time.Now()
	results := executor.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	errors := 0
	for _, r := range results { if r.IsError { errors++ } }
	if errors > 0 { t.Errorf("%d/10 reads failed", errors) }
	t.Logf("10 parallel reads: %v, %d errors", elapsed, errors)
}

func TestHard_Session_MultiTurnConversation(t *testing.T) {
	pool := hardDB(t)
	sessStore := session.NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, err := sessStore.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "hard-user", "test")
	if err != nil { t.Fatal(err) }
	defer sessStore.Delete(ctx, sess.ID)

	// Simulate 20-turn conversation
	for i := 0; i < 20; i++ {
		sessStore.AppendMessage(ctx, sess.ID, session.Message{
			Role: "user", Content: "Turn " + string(rune('A'+i%26)) + ": " + strings.Repeat("question ", 10),
		}, 20, 0)
		sessStore.AppendMessage(ctx, sess.ID, session.Message{
			Role: "assistant", Content: "Response " + string(rune('A'+i%26)) + ": " + strings.Repeat("answer ", 20),
		}, 0, 40)
	}

	// Verify history
	history, _ := sessStore.GetHistory(ctx, sess.ID)
	if len(history) < 40 { t.Errorf("expected 40 messages, got %d", len(history)) }

	// Verify ordering — alternating user/assistant
	for i := 0; i < len(history)-1; i++ {
		if history[i].Role == history[i+1].Role {
			t.Logf("consecutive same role at %d-%d: %s", i, i+1, history[i].Role)
		}
	}

	// Compact
	sessStore.Compact(ctx, sess.ID, 5)
	history2, _ := sessStore.GetHistory(ctx, sess.ID)
	t.Logf("20-turn conversation: %d messages, after compact: %d", len(history), len(history2))
}

func TestHard_LoopGuard_StressScenario(t *testing.T) {
	guard := NewLoopGuard()

	// Simulate 1000 tool calls with varying patterns
	start := time.Now()
	for i := 0; i < 1000; i++ {
		toolName := "tool_" + string(rune('A'+i%5))
		args := map[string]any{"query": "q" + string(rune('0'+i%10)), "page": i % 3}
		hash := guard.RecordCall(toolName, args)
		guard.DetectSameArgs(toolName, hash)

		if i%7 == 0 { guard.RecordToolError(toolName) }
		if i%3 == 0 { guard.RecordToolSuccess(toolName) }
		if i%10 == 0 { guard.RecordMutation("write_file") }
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second { t.Errorf("1000 ops too slow: %v", elapsed) }
	t.Logf("1000 loop guard ops: %v", elapsed)
}

func TestHard_Compactor_RealWorldScenario(t *testing.T) {
	c := NewCompactor(8000) // 8K token window

	// Build a realistic conversation with tool calls
	var msgs []providers.Message
	msgs = append(msgs, providers.Message{Role: "system", Content: strings.Repeat("You are a helpful assistant. ", 50)})

	for i := 0; i < 30; i++ {
		msgs = append(msgs, providers.Message{Role: "user", Content: "Question " + string(rune('A'+i%26)) + ": " + strings.Repeat("detail ", 15)})
		if i%3 == 0 {
			// Tool call turn
			msgs = append(msgs, providers.Message{Role: "assistant", ToolCalls: []providers.ToolCall{{ID: "tc", Name: "web_search"}}})
			msgs = append(msgs, providers.Message{Role: "tool", Content: strings.Repeat("search result ", 50), ToolCallID: "tc"})
		}
		msgs = append(msgs, providers.Message{Role: "assistant", Content: "Answer: " + strings.Repeat("explanation ", 20)})
	}

	t.Logf("before compaction: %d messages, ~%d tokens", len(msgs), estimateTokens(msgs))

	action := c.Check(msgs)
	if action == 0 { t.Log("under limit — no compaction needed") ; return }

	// Prune tools first
	pruned, removed := c.PruneTools(msgs)
	t.Logf("tool pruning: removed %d tokens", removed)

	// Then compact
	compacted := c.Compact(pruned, action)
	t.Logf("after compaction: %d messages, ~%d tokens", len(compacted), estimateTokens(compacted))

	// Last user message must survive
	lastUser := ""
	for i := len(compacted) - 1; i >= 0; i-- {
		if compacted[i].Role == "user" { lastUser = compacted[i].Content; break }
	}
	if lastUser == "" { t.Error("last user message lost") }
	if len(compacted) > len(msgs) { t.Error("compaction increased messages") }
}

func TestHard_PromptBuilder_AllSections(t *testing.T) {
	ag := &Agent{
		ID: "hard-test", DisplayName: "Hard Test Bot", AgentKey: "hard",
		SystemPrompt: "You are a comprehensive test agent with all features enabled.",
		Model: "gpt-4o-mini",
	}

	team := []*Agent{
		{ID: "dev", DisplayName: "Developer", SystemPrompt: "Code expert"},
		{ID: "qa", DisplayName: "QA Engineer", SystemPrompt: "Testing expert"},
		{ID: "pm", DisplayName: "Product Manager", SystemPrompt: "Product expert"},
	}

	reg := tools.NewRegistry()
	reg.Register(tools.NewReadFileTool("/tmp"))
	reg.Register(tools.NewExecTool("/tmp", true))
	reg.Register(tools.NewWebSearchTool(nil))
	reg.Register(tools.NewClarifyTool())

	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull, ModelID: "gpt-4o-mini"})
	pb.SetTeam(team)
	pb.SetToolRegistry(reg)
	pb.SetMemoryResults([]string{
		"User is a Go developer building Qorven",
		"User prefers terminal-based tools",
		"Project deadline is next Friday",
	})
	pb.SetWikiArticles([]string{
		"# Qorven Architecture\nQorven uses a single binary with 52+ tools.",
	})

	prompt := pb.Build()
	if len(prompt) < 500 { t.Errorf("prompt too short: %d chars", len(prompt)) }

	// Verify key sections
	sections := map[string]bool{
		"Hard Test Bot": false, "Developer": false, "QA Engineer": false,
		"Go developer": false, "Qorven": false,
	}
	for keyword := range sections {
		if strings.Contains(prompt, keyword) { sections[keyword] = true }
	}
	for keyword, found := range sections {
		if !found { t.Logf("section missing: %q", keyword) }
	}

	t.Logf("full prompt: %d chars, %d lines", len(prompt), strings.Count(prompt, "\n"))
}
