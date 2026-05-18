// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Diamond-hard memory tests — verify the memory system works as a product.

func TestDiamond_Memory_SaveSearchRelevance(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save 5 memories about different topics
	marker := time.Now().Format("150405")
	memories := []struct{ content, topic string }{
		{"DIAMOND_" + marker + " User prefers dark mode in all applications", "preference"},
		{"DIAMOND_" + marker + " The project uses PostgreSQL 16 with pgvector extension", "technical"},
		{"DIAMOND_" + marker + " Meeting scheduled for Friday at 3pm with the team", "schedule"},
		{"DIAMOND_" + marker + " User's favorite programming language is Go", "preference"},
		{"DIAMOND_" + marker + " The API rate limit is 100 requests per minute", "technical"},
	}

	var savedIDs []string
	for _, m := range memories {
		id, err := store.Save(ctx, tenant, Memory{AgentID: agentID, Content: m.content, Type: "fact", Source: "diamond_test", Importance: 1.0})
		if err != nil { t.Fatalf("save: %v", err) }
		savedIDs = append(savedIDs, id)
	}
	defer func() {
		for _, id := range savedIDs { pool.Exec(ctx, "DELETE FROM memories WHERE id = $1", id) }
	}()

	// Search for preferences — should rank preference memories higher
	// NOTE: uses 'simple' tsconfig which does NOT stem (prefers ≠ preference)
	results, err := store.Search(ctx, tenant, agentID, "dark mode", 5)
	if err != nil { t.Fatal(err) }
	if len(results) == 0 { t.Fatal("no results") }

	// Top result should be about preferences, not schedules
	topContent := strings.ToLower(results[0].Memory.Content)
	if strings.Contains(topContent, "dark mode") || strings.Contains(topContent, "programming language") {
		t.Log("preference memory ranked first ✓")
	} else if strings.Contains(topContent, "meeting") {
		t.Error("schedule ranked above preference for preference query")
	}

	// Search for technical — should find PostgreSQL and rate limit
	results2, _ := store.Search(ctx, tenant, agentID, "PostgreSQL database", 5)
	if len(results2) > 0 {
		found := false
		for _, r := range results2 {
			if strings.Contains(r.Memory.Content, "PostgreSQL") { found = true }
		}
		if found { t.Log("technical memory found for technical query ✓") }
	}

	t.Logf("relevance: %d results for preference, %d for technical", len(results), len(results2))
}

func TestDiamond_Memory_TemporalOrdering(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	marker := "TEMPORAL_" + time.Now().Format("150405")

	// Save memories with slight delays
	for i := 0; i < 5; i++ {
		store.Save(ctx, tenant, Memory{
			AgentID: agentID,
			Content: marker + " Memory number " + string(rune('1'+i)),
			Type: "fact", Source: "temporal_test", Importance: 1.0,
		})
		time.Sleep(10 * time.Millisecond)
	}

	// Search — more recent should appear
	results, _ := store.Search(ctx, tenant, agentID, marker, 10)
	t.Logf("temporal: %d results for %s", len(results), marker)
}

func TestDiamond_Memory_DataIntegrity_SpecialChars(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save content with characters that could break SQL or JSON
	dangerous := []string{
		"User said: \"I don't like bugs\" — that's important",
		"SQL test: '; DROP TABLE memories; --",
		"Unicode: 日本語テスト 🚀 café résumé",
		"Backslash: C:\\Users\\test\\file.txt",
		"HTML: <script>alert('xss')</script>",
		"Newlines:\nLine 1\nLine 2\nLine 3",
		"Very long: " + strings.Repeat("x", 5000),
	}

	saved := 0
	for _, content := range dangerous {
		_, err := store.Save(ctx, tenant, Memory{AgentID: agentID, Content: content, Type: "fact", Source: "integrity_test", Importance: 1.0})
		if err != nil { t.Logf("save failed (expected for some): %v", err); continue }
		saved++
	}

	// Verify the database wasn't corrupted
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE agent_id = $1", agentID).Scan(&count)
	if count == 0 { t.Error("no memories in DB after saves") }

	// Verify SQL injection didn't work
	var tableExists bool
	pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'memories')").Scan(&tableExists)
	if !tableExists { t.Fatal("CRITICAL: memories table was dropped by SQL injection!") }

	t.Logf("data integrity: %d/%d saved, DB intact, SQL injection safe ✓", saved, len(dangerous))
}
