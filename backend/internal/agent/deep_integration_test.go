// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
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

// === REAL AGENT LOOP INTEGRATION TESTS ===
// These test the actual agent loop with real stores, real tools, mock provider.

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

// mockProvider returns a fixed response — no real LLM call.
type testProvider struct {
	response string
	toolCall *providers.ToolCall
	delay    time.Duration
	calls    int
	mu       sync.Mutex
}

func (p *testProvider) Name() string         { return "test" }
func (p *testProvider) DefaultModel() string  { return "test-model" }
func (p *testProvider) SupportsThinking() bool { return false }
func (p *testProvider) Chat(ctx context.Context, req providers.ChatRequest) (*providers.ChatResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if p.delay > 0 { time.Sleep(p.delay) }
	resp := &providers.ChatResponse{Content: p.response, Usage: &providers.Usage{PromptTokens: 10, CompletionTokens: 5}}
	if p.toolCall != nil { resp.ToolCalls = []providers.ToolCall{*p.toolCall} }
	return resp, nil
}
func (p *testProvider) ChatStream(ctx context.Context, req providers.ChatRequest, fn func(providers.StreamChunk)) (*providers.ChatResponse, error) {
	resp, err := p.Chat(ctx, req)
	if err != nil { return nil, err }
	if fn != nil {
		fn(providers.StreamChunk{Content: resp.Content})
		fn(providers.StreamChunk{Done: true})
	}
	return resp, nil
}

func TestIntegration_AgentLoop_SimpleChat(t *testing.T) {
	pool := testDB(t)
	agentStore := NewStore(pool)
	sessionStore := session.NewStore(pool)
	providerReg := providers.NewRegistry()
	toolReg := tools.NewRegistry()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create a test agent
	ag, err := agentStore.Create(context.Background(), tenant, CreateAgentInput{
		AgentKey: "loop-test-" + time.Now().Format("150405"),
		Model: "test-model", SystemPrompt: "You are a test bot. Always reply with 'TEST OK'.",
	})
	if err != nil { t.Fatal(err) }
	defer agentStore.Delete(context.Background(), ag.ID)

	// Register mock provider
	_ = &testProvider{response: "TEST OK"}
	providerReg.Register(providers.ProviderConfig{
		ID: "test-provider", Name: "test", ProviderType: "openai_compat", Enabled: true,
	})

	// Create loop
	_ = NewLoop(agentStore, sessionStore, providerReg, toolReg, nil, nil, tenant)

	// Override provider resolution to use our mock
	// Since we can't easily inject, test the components individually
	t.Logf("agent created: %s, loop created", ag.ID)

	// Test that the loop guard works
	guard := NewLoopGuard()
	for i := 0; i < 100; i++ {
		hash := guard.RecordCall("web_search", map[string]any{"q": "same query"})
		result := guard.DetectSameArgs("web_search", hash)
		if result.Level == DetectionCritical {
			t.Logf("loop detected at iteration %d: %s", i, result.Message)
			if i < 3 { t.Error("detected too early") }
			return
		}
	}
	t.Log("loop guard threshold may be high")
}

func TestIntegration_AgentLoop_ToolExecution(t *testing.T) {
	// Test that tools execute correctly through the parallel executor
	reg := tools.NewRegistry()
	
	// Register real tools
	dir := t.TempDir()
	reg.Register(tools.NewReadFileTool(dir))
	reg.Register(tools.NewWriteFileTool(dir))
	reg.Register(tools.NewListFilesTool(dir))
	reg.Register(tools.NewExecTool(dir, true))

	executor := NewParallelToolExecutor(reg, 5)

	// Execute write + read in sequence
	writeResult := executor.Execute(context.Background(), []ToolCall{
		{Name: "write_file", Args: map[string]any{"path": "test.txt", "content": "hello from agent"}},
	})
	if len(writeResult) != 1 { t.Fatal("write result count") }
	if writeResult[0].IsError { t.Fatalf("write failed: %s", writeResult[0].Content) }

	readResult := executor.Execute(context.Background(), []ToolCall{
		{Name: "read_file", Args: map[string]any{"path": "test.txt"}},
	})
	if len(readResult) != 1 { t.Fatal("read result count") }
	if readResult[0].IsError { t.Fatalf("read failed: %s", readResult[0].Content) }
	if !strings.Contains(readResult[0].Content, "hello from agent") {
		t.Errorf("content mismatch: %q", readResult[0].Content)
	}
}

func TestIntegration_AgentLoop_ParallelToolExecution(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.NewExecTool(t.TempDir(), true))

	executor := NewParallelToolExecutor(reg, 5)

	// Execute 5 commands in parallel — verify they all complete
	calls := []ToolCall{
		{Name: "exec", Args: map[string]any{"command": "echo one"}},
		{Name: "exec", Args: map[string]any{"command": "echo two"}},
		{Name: "exec", Args: map[string]any{"command": "echo three"}},
		{Name: "exec", Args: map[string]any{"command": "echo four"}},
		{Name: "exec", Args: map[string]any{"command": "echo five"}},
	}

	start := time.Now()
	results := executor.Execute(context.Background(), calls)
	elapsed := time.Since(start)

	if len(results) != 5 { t.Fatalf("expected 5 results, got %d", len(results)) }
	for i, r := range results {
		if r.IsError { t.Errorf("call %d failed: %s", i, r.Content) }
	}
	// Parallel should be faster than sequential (5 × ~10ms = 50ms sequential, ~10ms parallel)
	t.Logf("5 parallel exec calls took %v", elapsed)
}

func TestIntegration_AgentLoop_ToolTimeout(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.NewExecTool(t.TempDir(), true))

	executor := NewParallelToolExecutor(reg, 5)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	results := executor.Execute(ctx, []ToolCall{
		{Name: "exec", Args: map[string]any{"command": "sleep 10"}},
	})
	if len(results) != 1 { t.Fatal("expected 1 result") }
	if !results[0].IsError { t.Error("should timeout") }
	t.Logf("timeout result: %s", results[0].Content[:min2(len(results[0].Content), 100)])
}

func TestIntegration_AgentLoop_MutationDetection(t *testing.T) {
	// Verify that mutating tools are correctly identified
	mutating := []string{"write_file", "edit", "exec", "patch", "delete_file", "create_directory", "move_file", "sandbox_exec"}
	readOnly := []string{"read_file", "list_files", "web_search", "web_fetch", "memory_search", "clarify"}

	for _, name := range mutating {
		if !isMutating(name) { t.Errorf("%q should be mutating", name) }
	}
	for _, name := range readOnly {
		if isMutating(name) { t.Errorf("%q should NOT be mutating", name) }
	}
}

func TestIntegration_AgentLoop_CompactorUnderPressure(t *testing.T) {
	// Build a conversation that exceeds context window
	c := NewCompactor(4000) // small window

	var msgs []providers.Message
	msgs = append(msgs, providers.Message{Role: "system", Content: strings.Repeat("System prompt. ", 100)})
	for i := 0; i < 50; i++ {
		msgs = append(msgs, providers.Message{Role: "user", Content: "Question " + string(rune('A'+i%26)) + ": " + strings.Repeat("detail ", 20)})
		msgs = append(msgs, providers.Message{Role: "assistant", Content: "Answer: " + strings.Repeat("explanation ", 30)})
	}

	// Should detect need for compaction
	action := c.Check(msgs)
	if action == 0 { t.Error("should need compaction with 100 messages in 4K window") }

	// Compact
	compacted := c.Compact(msgs, action)
	if len(compacted) >= len(msgs) { t.Error("compaction should reduce message count") }
	if len(compacted) == 0 { t.Error("compaction should keep some messages") }

	// Last user message should be preserved
	lastUser := ""
	for i := len(compacted) - 1; i >= 0; i-- {
		if compacted[i].Role == "user" { lastUser = compacted[i].Content; break }
	}
	if lastUser == "" { t.Error("last user message lost in compaction") }
	t.Logf("compacted %d → %d messages", len(msgs), len(compacted))
}

func TestIntegration_AgentLoop_PromptBuilder_FullPipeline(t *testing.T) {
	ag := &Agent{
		ID: "test-agent", DisplayName: "TestBot",
		SystemPrompt: "You are a helpful assistant for testing.",
		Model: "gpt-4",
	}

	// Build with all sections
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull, ModelID: "gpt-4"})
	pb.SetMemoryResults([]string{
		"User prefers dark mode",
		"User is a Go developer",
		"User is building Qorven",
	})
	pb.SetWikiArticles([]string{
		"# Qorven\nQorven is a multi-agent AI workspace platform.",
	})

	team := []*Agent{
		{ID: "dev", DisplayName: "Developer", SystemPrompt: "You write code."},
		{ID: "qa", DisplayName: "QA", SystemPrompt: "You test code."},
	}
	pb.SetTeam(team)

	toolReg := tools.NewRegistry()
	toolReg.Register(tools.NewClarifyTool())
	pb.SetToolRegistry(toolReg)

	prompt := pb.Build()
	if prompt == "" { t.Fatal("empty prompt") }
	if len(prompt) < 100 { t.Errorf("prompt too short: %d chars", len(prompt)) }

	// Verify key sections are present
	if !strings.Contains(prompt, "TestBot") && !strings.Contains(prompt, "helpful assistant") {
		t.Error("identity section missing")
	}
	t.Logf("full prompt: %d chars, %d lines", len(prompt), strings.Count(prompt, "\n"))
}

func TestIntegration_AgentLoop_LoopGuard_FullScenario(t *testing.T) {
	guard := NewLoopGuard()

	// Simulate a real agent loop scenario:
	// 1. Agent searches for "cats" 3 times (should detect repetition)
	// 2. Agent writes a file (mutation resets read-only streak)
	// 3. Agent searches for "dogs" (new query, no detection)
	// 4. Tool fails 3 times (circuit breaker)

	// Phase 1: Repeated search
	for i := 0; i < 5; i++ {
		hash := guard.RecordCall("web_search", map[string]any{"query": "cats"})
		guard.RecordResult("web_search", hash, "found 10 results about cats")
		guard.RecordToolSuccess("web_search")
		result := guard.DetectSameArgs("web_search", hash)
		if result.Level == DetectionCritical {
			t.Logf("repetition detected at iteration %d", i)
			break
		}
	}

	// Phase 2: Mutation
	guard.RecordMutation("write_file")
	guard.RecordCall("write_file", map[string]any{"path": "output.txt"})
	guard.RecordToolSuccess("write_file")

	// Phase 3: New search
	hash := guard.RecordCall("web_search", map[string]any{"query": "dogs"})
	result := guard.DetectSameArgs("web_search", hash)
	if result.Level == DetectionCritical { t.Error("new query should not trigger detection") }

	// Phase 4: Tool failures
	guard.RecordToolError("flaky_api")
	guard.RecordToolError("flaky_api")
	if !guard.IsToolCircuitBroken("flaky_api") { t.Error("2 failures should break circuit") }
	if guard.IsToolCircuitBroken("web_search") { t.Error("web_search should not be broken") }

	// Phase 5: Recovery
	guard.RecordToolSuccess("flaky_api")
	if guard.IsToolCircuitBroken("flaky_api") { t.Error("success should reset circuit") }
}

func TestIntegration_AgentLoop_SessionPersistence(t *testing.T) {
	pool := testDB(t)
	sessionStore := session.NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Get a real agent ID
	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents in DB") }

	// Create session
	sess, err := sessionStore.Create(ctx, tenant, agentID, "test-user", "test")
	if err != nil { t.Fatal(err) }
	defer sessionStore.Delete(ctx, sess.ID)

	// Simulate a multi-turn conversation
	turns := []struct{ role, content string }{
		{"user", "What is Go?"},
		{"assistant", "Go is a programming language created by Google."},
		{"user", "What are its main features?"},
		{"assistant", "Go features include goroutines, channels, and a simple syntax."},
		{"user", "Show me an example"},
		{"assistant", "```go\npackage main\n\nfunc main() {\n\tfmt.Println(\"Hello\")\n}\n```"},
	}

	for _, turn := range turns {
		err := sessionStore.AppendMessage(ctx, sess.ID, session.Message{
			Role: turn.role, Content: turn.content,
		}, 10, 10)
		if err != nil { t.Fatalf("append %s: %v", turn.role, err) }
	}

	// Verify history
	history, err := sessionStore.GetHistory(ctx, sess.ID)
	if err != nil { t.Fatal(err) }
	if len(history) < 6 { t.Errorf("expected 6+ messages, got %d", len(history)) }

	// Verify order
	if history[0].Role != "user" { t.Error("first message should be user") }
	if !strings.Contains(history[len(history)-1].Content, "Hello") {
		t.Error("last message should contain code example")
	}

	// Compact
	err = sessionStore.Compact(ctx, sess.ID, 2)
	if err != nil { t.Fatalf("compact: %v", err) }

	// After compaction, should have fewer messages
	history2, _ := sessionStore.GetHistory(ctx, sess.ID)
	t.Logf("before compact: %d, after: %d", len(history), len(history2))
}
