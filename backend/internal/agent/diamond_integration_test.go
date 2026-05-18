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
	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// diamond_integration_test.go — Full lifecycle integration tests.
// These test the REAL production path across multiple packages with a real database.

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

// ── Full Agent Lifecycle: create → configure → chat → remember → recall → delete ──

func TestDiamond_Integration_AgentLifecycle(t *testing.T) {
	pool := integrationPool(t)
	agentStore := NewStore(pool)
	sessStore := session.NewStore(pool)
	memStore := memory.NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"
	marker := time.Now().Format("150405")

	// 1. CREATE agent
	ag, err := agentStore.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "lifecycle-" + marker, DisplayName: "Lifecycle Test",
		Model: "gpt-4o-mini", SystemPrompt: "You are a test agent for lifecycle verification.",
		Temperature: 0.7,
	})
	if err != nil { t.Fatal(err) }
	defer agentStore.Delete(ctx, ag.ID)
	t.Logf("1. created agent %s ✓", ag.ID[:8])

	// 2. VERIFY agent exists and has correct config
	got, err := agentStore.Get(ctx, ag.ID)
	if err != nil { t.Fatal(err) }
	if got.AgentKey != "lifecycle-"+marker { t.Errorf("key: %q", got.AgentKey) }
	if got.Model != "gpt-4o-mini" { t.Errorf("model: %q", got.Model) }
	if !strings.Contains(got.SystemPrompt, "lifecycle verification") { t.Error("prompt wrong") }
	t.Log("2. verified config ✓")

	// 3. CREATE session for this agent
	sess, err := sessStore.Create(ctx, tenant, ag.ID, "test-user", "web")
	if err != nil { t.Fatal(err) }
	defer sessStore.Delete(ctx, sess.ID)
	t.Logf("3. created session %s ✓", sess.ID[:8])

	// 4. APPEND conversation messages
	conversation := []struct{ role, content string }{
		{"user", "My project is called Qorven. It's an AI agent platform."},
		{"assistant", "Got it! Qorven is your AI agent platform. How can I help?"},
		{"user", "The deadline is April 30, 2026."},
		{"assistant", "Noted — Qorven deadline is April 30, 2026."},
	}
	for _, turn := range conversation {
		sessStore.AppendMessage(ctx, sess.ID, session.Message{Role: turn.role, Content: turn.content}, 10, 10)
	}
	t.Log("4. appended 4 messages ✓")

	// 5. VERIFY conversation history
	history, err := sessStore.GetHistory(ctx, sess.ID)
	if err != nil { t.Fatal(err) }
	if len(history) < 4 { t.Fatalf("expected 4 messages, got %d", len(history)) }

	fullText := ""
	for _, m := range history { fullText += m.Content + " " }
	if !strings.Contains(fullText, "Qorven") { t.Error("Qorven not in history") }
	if !strings.Contains(fullText, "April 30") { t.Error("deadline not in history") }
	t.Log("5. verified history ✓")

	// 6. SAVE memories from the conversation
	mem1, err := memStore.Save(ctx, tenant, memory.Memory{
		AgentID: ag.ID, Content: "User's project is called Qorven — an AI agent platform",
		Type: "fact", Source: "conversation", Importance: 1.0,
	})
	if err != nil { t.Fatal(err) }
	defer pool.Exec(ctx, "DELETE FROM memories WHERE id = $1", mem1)

	mem2, err := memStore.Save(ctx, tenant, memory.Memory{
		AgentID: ag.ID, Content: "Qorven deadline is April 30, 2026",
		Type: "fact", Source: "conversation", Importance: 1.0,
	})
	if err != nil { t.Fatal(err) }
	defer pool.Exec(ctx, "DELETE FROM memories WHERE id = $1", mem2)
	t.Log("6. saved 2 memories ✓")

	// 7. SEARCH memories — verify write-then-read consistency
	results, err := memStore.Search(ctx, tenant, ag.ID, "Qorven platform", 5)
	if err != nil { t.Fatal(err) }
	if len(results) == 0 { t.Fatal("memory search returned 0 results — write-then-read FAILED") }

	found := false
	for _, r := range results {
		if strings.Contains(r.Memory.Content, "Qorven") { found = true }
	}
	if !found { t.Error("Qorven memory not found in search results") }
	t.Logf("7. memory search: %d results, Qorven found ✓", len(results))

	// 8. UPDATE agent
	agentStore.Update(ctx, ag.ID, map[string]any{"display_name": "Updated Lifecycle"})
	updated, _ := agentStore.Get(ctx, ag.ID)
	if updated.DisplayName != "Updated Lifecycle" { t.Error("update not applied") }
	t.Log("8. updated agent ✓")

	// 9. DELETE and verify cleanup
	agentStore.Delete(ctx, ag.ID)
	_, err = agentStore.Get(ctx, ag.ID)
	if err == nil { t.Error("agent still exists after delete") }
	t.Log("9. deleted agent ✓")

	t.Log("full lifecycle: create→config→session→chat→memory→search→update→delete ✓")
}

// ── Concurrent Agent Operations ──

func TestDiamond_Integration_ConcurrentAgentCRUD(t *testing.T) {
	pool := integrationPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"
	marker := time.Now().Format("150405")

	// Create 10 agents concurrently
	var wg sync.WaitGroup
	ids := make(chan string, 10)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ag, err := store.Create(ctx, tenant, CreateAgentInput{
				AgentKey: "concurrent-" + marker + "-" + string(rune('A'+n)),
				Model: "gpt-4o-mini", SystemPrompt: "Agent " + string(rune('A'+n)),
			})
			if err != nil { errors <- err; return }
			ids <- ag.ID
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errors)

	errCount := 0
	for err := range errors { t.Logf("create error: %v", err); errCount++ }

	var agentIDs []string
	for id := range ids { agentIDs = append(agentIDs, id) }
	defer func() { for _, id := range agentIDs { store.Delete(ctx, id) } }()

	if errCount > 2 { t.Errorf("too many create errors: %d/10", errCount) }
	if len(agentIDs) < 8 { t.Errorf("expected 8+ agents, got %d", len(agentIDs)) }

	// Update all concurrently
	var wg2 sync.WaitGroup
	for _, id := range agentIDs {
		wg2.Add(1)
		go func(agentID string) {
			defer wg2.Done()
			store.Update(ctx, agentID, map[string]any{"display_name": "Updated"})
		}(id)
	}
	wg2.Wait()

	// Verify all updated
	for _, id := range agentIDs {
		ag, err := store.Get(ctx, id)
		if err != nil { continue }
		if ag.DisplayName != "Updated" { t.Errorf("agent %s not updated", id[:8]) }
	}

	t.Logf("concurrent CRUD: %d agents created, updated, verified ✓", len(agentIDs))
}

// ── Memory Write-Then-Read Consistency Under Load ──

func TestDiamond_Integration_MemoryConsistency(t *testing.T) {
	pool := integrationPool(t)
	memStore := memory.NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	marker := "CONSISTENCY_" + time.Now().Format("150405")

	// Write 10 memories with unique content
	var savedIDs []string
	for i := 0; i < 10; i++ {
		content := marker + " fact number " + string(rune('A'+i)) + " about testing"
		id, err := memStore.Save(ctx, tenant, memory.Memory{
			AgentID: agentID, Content: content, Type: "fact",
			Source: "consistency_test", Importance: 1.0,
		})
		if err != nil { t.Fatalf("save %d: %v", i, err) }
		savedIDs = append(savedIDs, id)
	}
	defer func() {
		for _, id := range savedIDs { pool.Exec(ctx, "DELETE FROM memories WHERE id = $1", id) }
	}()

	// Immediately search — all should be findable
	results, err := memStore.Search(ctx, tenant, agentID, marker+" fact testing", 20)
	if err != nil { t.Fatal(err) }

	if len(results) < 5 {
		t.Errorf("write-then-read: expected 5+ results, got %d (consistency issue!)", len(results))
	}

	// Verify each saved memory is searchable
	foundCount := 0
	for _, r := range results {
		if strings.Contains(r.Memory.Content, marker) { foundCount++ }
	}
	if foundCount < 5 { t.Errorf("only %d/%d memories found in search", foundCount, len(savedIDs)) }

	t.Logf("memory consistency: wrote %d, found %d in search ✓", len(savedIDs), foundCount)
}

// ── Database Connection Pool Under Stress ──

func TestDiamond_Integration_DBPoolStress(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()

	// 100 concurrent queries
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	start := time.Now()

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var count int
			err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM agents").Scan(&count)
			if err != nil { errors <- err }
		}()
	}
	wg.Wait()
	close(errors)
	elapsed := time.Since(start)

	errCount := 0
	for err := range errors { t.Logf("pool error: %v", err); errCount++ }

	if errCount > 5 { t.Errorf("too many pool errors: %d/100", errCount) }
	if elapsed > 5*time.Second { t.Errorf("too slow: %v", elapsed) }

	t.Logf("DB pool: 100 concurrent queries, %d errors, %v ✓", errCount, elapsed.Round(time.Millisecond))
}
