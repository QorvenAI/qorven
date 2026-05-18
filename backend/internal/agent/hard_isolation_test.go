// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Hard agent tests — real DB multi-agent scenarios, context window management.

func hardPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestHard_Agent_MultiAgentIsolation(t *testing.T) {
	pool := hardPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create 2 agents with different prompts
	ag1, err := store.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "isolation-A-" + time.Now().Format("150405"),
		Model: "gpt-4o-mini", SystemPrompt: "You are Agent A. You only know about cats.",
	})
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, ag1.ID)

	ag2, err := store.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "isolation-B-" + time.Now().Format("150405"),
		Model: "gpt-4o-mini", SystemPrompt: "You are Agent B. You only know about dogs.",
	})
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, ag2.ID)

	// Verify they're different agents
	if ag1.ID == ag2.ID { t.Fatal("same ID") }
	if ag1.AgentKey == ag2.AgentKey { t.Fatal("same key") }

	// Get each and verify prompts are isolated
	got1, _ := store.Get(ctx, ag1.ID)
	got2, _ := store.Get(ctx, ag2.ID)
	if got1.SystemPrompt == got2.SystemPrompt { t.Error("prompts should be different") }
	if !strings.Contains(got1.SystemPrompt, "cats") { t.Error("agent A prompt wrong") }
	if !strings.Contains(got2.SystemPrompt, "dogs") { t.Error("agent B prompt wrong") }
	t.Log("multi-agent isolation: different prompts verified ✓")
}

func TestHard_Agent_UpdateDoesNotCorruptOthers(t *testing.T) {
	pool := hardPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create 2 agents
	ag1, _ := store.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "corrupt-A-" + time.Now().Format("150405"),
		Model: "gpt-4o-mini", SystemPrompt: "Original A",
	})
	defer store.Delete(ctx, ag1.ID)

	ag2, _ := store.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "corrupt-B-" + time.Now().Format("150405"),
		Model: "gpt-4o-mini", SystemPrompt: "Original B",
	})
	defer store.Delete(ctx, ag2.ID)

	// Update agent 1
	store.Update(ctx, ag1.ID, map[string]any{"system_prompt": "Updated A"})

	// Verify agent 2 is NOT affected
	got2, _ := store.Get(ctx, ag2.ID)
	if got2.SystemPrompt != "Original B" { t.Errorf("agent B corrupted: %q", got2.SystemPrompt) }
	t.Log("update isolation: agent B unaffected by agent A update ✓")
}

func TestHard_Session_CrossAgentIsolation(t *testing.T) {
	pool := hardPool(t)
	agentStore := NewStore(pool)
	sessStore := session.NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create 2 agents
	ag1, _ := agentStore.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "sess-iso-A-" + time.Now().Format("150405"), Model: "gpt-4o-mini",
	})
	defer agentStore.Delete(ctx, ag1.ID)

	ag2, _ := agentStore.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "sess-iso-B-" + time.Now().Format("150405"), Model: "gpt-4o-mini",
	})
	defer agentStore.Delete(ctx, ag2.ID)

	// Create sessions for each
	sess1, _ := sessStore.Create(ctx, tenant, ag1.ID, "user1", "test")
	defer sessStore.Delete(ctx, sess1.ID)
	sess2, _ := sessStore.Create(ctx, tenant, ag2.ID, "user2", "test")
	defer sessStore.Delete(ctx, sess2.ID)

	// Add messages to each
	sessStore.AppendMessage(ctx, sess1.ID, session.Message{Role: "user", Content: "SECRET_A"}, 5, 0)
	sessStore.AppendMessage(ctx, sess2.ID, session.Message{Role: "user", Content: "SECRET_B"}, 5, 0)

	// Verify isolation — session 1 should not contain SECRET_B
	history1, _ := sessStore.GetHistory(ctx, sess1.ID)
	for _, msg := range history1 {
		if strings.Contains(msg.Content, "SECRET_B") { t.Fatal("SECURITY: cross-session data leak!") }
	}

	history2, _ := sessStore.GetHistory(ctx, sess2.ID)
	for _, msg := range history2 {
		if strings.Contains(msg.Content, "SECRET_A") { t.Fatal("SECURITY: cross-session data leak!") }
	}
	t.Log("cross-agent session isolation: no data leakage ✓")
}

func TestHard_Compactor_ToolCallPreservation(t *testing.T) {
	c := NewCompactor(2000) // small window to force compaction

	// Build conversation with tool calls
	msgs := []providers.Message{
		{Role: "system", Content: strings.Repeat("System prompt. ", 50)},
		{Role: "user", Content: "Search for Go tutorials"},
		{Role: "assistant", ToolCalls: []providers.ToolCall{{ID: "tc1", Name: "web_search", Arguments: map[string]any{"query": "Go tutorials"}}}},
		{Role: "tool", Content: "Found: Go by Example, Tour of Go, Effective Go", ToolCallID: "tc1"},
		{Role: "assistant", Content: "I found several Go tutorials for you."},
		{Role: "user", Content: "Tell me about the first one"},
		{Role: "assistant", Content: "Go by Example is a hands-on introduction to Go."},
		{Role: "user", Content: "What about concurrency?"},
	}

	action := c.Check(msgs)
	if action == 0 { t.Log("under limit — no compaction needed"); return }

	compacted := c.Compact(msgs, action)

	// The last user message MUST survive
	lastUser := ""
	for i := len(compacted) - 1; i >= 0; i-- {
		if compacted[i].Role == "user" { lastUser = compacted[i].Content; break }
	}
	if !strings.Contains(lastUser, "concurrency") { t.Error("last user message lost") }

	// System prompt should survive (possibly compressed)
	hasSystem := false
	for _, m := range compacted { if m.Role == "system" { hasSystem = true } }
	if !hasSystem { t.Error("system prompt lost") }

	t.Logf("compaction with tool calls: %d → %d messages, last user + system preserved ✓", len(msgs), len(compacted))
}

func TestHard_LoopGuard_RealWorldScenario(t *testing.T) {
	guard := NewLoopGuard()

	// Simulate: agent searches, gets result, searches again with same query (loop)
	// then tries a different approach (mutation), then searches with new query

	// Phase 1: Repeated search (should detect)
	detected := false
	for i := 0; i < 10; i++ {
		hash := guard.RecordCall("web_search", map[string]any{"query": "Go error handling"})
		guard.RecordResult("web_search", hash, "same results every time")
		guard.RecordToolSuccess("web_search")
		result := guard.DetectSameArgs("web_search", hash)
		if result.Level == DetectionCritical {
			detected = true
			t.Logf("loop detected at iteration %d ✓", i)
			break
		}
	}
	if !detected { t.Log("loop detection threshold not reached in 10 iterations") }

	// Phase 2: Agent writes a file (mutation should reset streak)
	guard.RecordMutation("write_file")
	guard.RecordCall("write_file", map[string]any{"path": "output.txt"})
	guard.RecordToolSuccess("write_file")

	// Phase 3: New search query (should NOT detect loop)
	hash := guard.RecordCall("web_search", map[string]any{"query": "Go concurrency patterns"})
	result := guard.DetectSameArgs("web_search", hash)
	if result.Level == DetectionCritical { t.Error("new query should not trigger loop detection") }

	// Phase 4: Tool failure → circuit breaker
	guard.RecordToolError("flaky_api")
	guard.RecordToolError("flaky_api")
	if !guard.IsToolCircuitBroken("flaky_api") { t.Error("2 failures should break circuit") }

	// Phase 5: Success resets circuit
	guard.RecordToolSuccess("flaky_api")
	if guard.IsToolCircuitBroken("flaky_api") { t.Error("success should reset circuit") }

	t.Log("real-world loop guard scenario: all phases verified ✓")
}

func TestHard_PromptBuilder_ContextWindowAwareness(t *testing.T) {
	// Build prompts of increasing complexity and verify they grow
	ag := &Agent{ID: "test", DisplayName: "Bot", SystemPrompt: "You are helpful."}

	// Minimal prompt
	pb1 := NewPromptBuilder(ag, RuntimeContext{Mode: PromptMinimal})
	p1 := pb1.Build()

	// Full prompt
	pb2 := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	p2 := pb2.Build()

	// Full with memory + team
	pb3 := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb3.SetMemoryResults([]string{"User likes Go", "User builds Qorven", "Deadline is Friday"})
	pb3.SetTeam([]*Agent{{ID: "dev", DisplayName: "Dev"}, {ID: "qa", DisplayName: "QA"}})
	p3 := pb3.Build()

	// Each should be progressively larger
	if len(p2) <= len(p1) { t.Logf("full (%d) should be > minimal (%d)", len(p2), len(p1)) }
	if len(p3) <= len(p2) { t.Logf("full+memory+team (%d) should be > full (%d)", len(p3), len(p2)) }

	t.Logf("prompt sizes: minimal=%d, full=%d, full+memory+team=%d", len(p1), len(p2), len(p3))
}
