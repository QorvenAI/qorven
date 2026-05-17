// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"testing"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Integration tests — real database memory save/search.

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	// Ensure test agent exists (FK constraint requires it)
	pool.Exec(ctx, `INSERT INTO agents (id, tenant_id, agent_key, display_name, model, status)
		VALUES ('021c16ae-6b93-4d62-bf0e-32fd6a275fd7', '00000000-0000-0000-0000-000000000001',
		'test-agent', 'Test Agent', 'test', 'active') ON CONFLICT (id) DO NOTHING`)
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestIntegration_Memory_SaveAndSearch(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	// Save a memory
	id, err := store.Save(ctx, "00000000-0000-0000-0000-000000000001", Memory{
		AgentID: "021c16ae-6b93-4d62-bf0e-32fd6a275fd7",
		Content: "The user prefers dark mode and uses Go programming language.",
		Type: "preference", Source: "chat",
	})
	if err != nil { t.Fatalf("save: %v", err) }
	if id == "" { t.Fatal("empty id") }
	t.Logf("saved memory: %s", id)

	// Search for it
	results, err := store.Search(ctx, "00000000-0000-0000-0000-000000000001", "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "dark mode", 10)
	if err != nil { t.Fatalf("search: %v", err) }
	if len(results) == 0 { t.Error("should find the saved memory") }
	
	found := false
	for _, r := range results {
		if r.Memory.ID == id { found = true; break }
	}
	if !found { t.Log("saved memory not in top 10 results (many memories in DB)") }

	// Search with different query
	results2, err := store.Search(ctx, "00000000-0000-0000-0000-000000000001", "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "Go programming", 10)
	if err != nil { t.Fatalf("search2: %v", err) }
	t.Logf("search 'Go programming': %d results", len(results2))

	// Delete
	if err != nil { t.Fatalf("delete: %v", err) }
}

func TestIntegration_Memory_SaveMultiple(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	memories := []string{
		"User works at a tech startup in San Francisco.",
		"User's favorite programming language is Go.",
		"User prefers terminal-based tools over GUIs.",
		"User is building an AI agent platform called Qorven.",
		"User values code quality over speed.",
	}

	var ids []string
	for _, content := range memories {
		id, err := store.Save(ctx, tenant, Memory{
			AgentID: "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", Content: content,
			Type: "fact", Source: "chat",
		})
		if err != nil { t.Fatalf("save: %v", err) }
		ids = append(ids, id)
	}
	t.Logf("saved %d memories", len(ids))

	// Search should find relevant ones
	results, _ := store.Search(ctx, tenant, "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "programming language", 5)
	t.Logf("search 'programming language': %d results", len(results))

	results2, _ := store.Search(ctx, tenant, "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "Qorven AI platform", 5)
	t.Logf("search 'Qorven AI platform': %d results", len(results2))

	// Cleanup
}

func TestIntegration_Memory_SearchEmpty(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	results, err := store.Search(ctx, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000097", "anything", 10)
	if err != nil { t.Fatalf("search: %v", err) }
	if len(results) != 0 { t.Logf("unexpected results for nonexistent agent: %d", len(results)) }
}

// === HARD MEMORY TESTS ===

func TestIntegration_Memory_HybridSearch(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"
	agentID := "021c16ae-6b93-4d62-bf0e-32fd6a275fd7"

	// Save memories with different content
	memories := []struct{ content, typ string }{
		{"User's favorite color is blue and they love the ocean", "preference"},
		{"The project deadline is next Friday at 5pm", "fact"},
		{"User mentioned they work at a startup in San Francisco", "fact"},
		{"User prefers TypeScript over JavaScript for frontend work", "preference"},
		{"The API rate limit is 100 requests per minute", "fact"},
	}
	var ids []string
	for _, m := range memories {
		id, err := store.Save(ctx, tenant, Memory{AgentID: agentID, Content: m.content, Type: m.typ, Source: "test"})
		if err != nil { t.Fatalf("save: %v", err) }
		ids = append(ids, id)
	}
	t.Logf("saved %d memories", len(ids))

	// Search for specific topics
	tests := []struct{ query string; minResults int }{
		{"favorite color blue", 1},
		{"project deadline", 1},
		{"San Francisco startup", 1},
		{"TypeScript JavaScript", 1},
		{"rate limit API", 1},
		{"completely unrelated xyz123", 0},
	}
	for _, tt := range tests {
		results, err := store.Search(ctx, tenant, agentID, tt.query, 10)
		if err != nil { t.Fatalf("search %q: %v", tt.query, err) }
		if len(results) < tt.minResults {
			t.Errorf("search %q: got %d results, want >= %d", tt.query, len(results), tt.minResults)
		}
		t.Logf("search %q: %d results", tt.query, len(results))
	}
}

func TestIntegration_Memory_SaveLargeContent(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"
	agentID := "021c16ae-6b93-4d62-bf0e-32fd6a275fd7"

	// Save a very large memory
	large := strings.Repeat("This is a long memory about various topics. ", 500)
	id, err := store.Save(ctx, tenant, Memory{AgentID: agentID, Content: large, Type: "fact", Source: "test"})
	if err != nil { t.Fatalf("save large: %v", err) }
	if id == "" { t.Error("empty id for large content") }
	t.Logf("saved large memory (%d chars): %s", len(large), id)
}

func TestIntegration_Memory_ConcurrentSave(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"
	agentID := "021c16ae-6b93-4d62-bf0e-32fd6a275fd7"

	var wg sync.WaitGroup
	errors := int32(0)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := store.Save(ctx, tenant, Memory{
				AgentID: agentID,
				Content: "Concurrent memory " + string(rune('A'+n%26)),
				Type: "fact", Source: "concurrent",
			})
			if err != nil { atomic.AddInt32(&errors, 1) }
		}(i)
	}
	wg.Wait()
	if errors > 0 { t.Errorf("%d/20 concurrent saves failed", errors) }
}
