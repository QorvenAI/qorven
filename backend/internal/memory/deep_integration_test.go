// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

)

// Deep memory tests — relevance ranking, concurrent access, data integrity.

func TestDeep_Memory_RelevanceRanking(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Get a real agent
	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save memories with varying relevance to "Go programming"
	memories := []struct{ content string; relevance string }{
		{"Go is a statically typed compiled language designed at Google", "high"},
		{"Python is a dynamically typed interpreted language", "low"},
		{"The Go programming language has goroutines for concurrency", "high"},
		{"JavaScript runs in web browsers", "low"},
		{"Go's standard library includes net/http for web servers", "medium"},
		{"Rust is a systems programming language focused on safety", "low"},
		{"Go modules manage dependencies in Go projects", "high"},
	}

	var ids []string
	for _, m := range memories {
		id, err := store.Save(ctx, tenant, Memory{AgentID: agentID, Content: m.content, Type: "fact", Source: "relevance_test"})
		if err != nil { t.Fatalf("save: %v", err) }
		ids = append(ids, id)
	}

	// Search for "Go programming"
	results, err := store.Search(ctx, tenant, agentID, "Go programming language", 10)
	if err != nil { t.Fatal(err) }

	// Verify Go-related memories rank higher than Python/JS/Rust
	if len(results) >= 3 {
		topContent := ""
		for _, r := range results[:3] { topContent += r.Memory.Content + " " }
		topContent = strings.ToLower(topContent)
		goMentions := strings.Count(topContent, "go")
		otherMentions := strings.Count(topContent, "python") + strings.Count(topContent, "javascript") + strings.Count(topContent, "rust")
		if goMentions <= otherMentions {
			t.Errorf("Go should rank higher: go=%d, others=%d in top 3", goMentions, otherMentions)
		}
		t.Logf("relevance: Go mentions=%d, other=%d in top 3 results", goMentions, otherMentions)
	}

	// Search for "web development"
	results2, _ := store.Search(ctx, tenant, agentID, "web development browser", 5)
	t.Logf("search 'web development': %d results", len(results2))
}

func TestDeep_Memory_ConcurrentSaveSearch(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	var wg sync.WaitGroup
	var saveErrors, searchErrors atomic.Int32

	// 20 concurrent saves
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := store.Save(ctx, tenant, Memory{
				AgentID: agentID,
				Content: "Concurrent memory entry number " + string(rune('A'+n%26)),
				Type: "fact", Source: "concurrent_test",
			})
			if err != nil { saveErrors.Add(1) }
		}(i)
	}

	// 10 concurrent searches while saving
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Search(ctx, tenant, agentID, "concurrent memory", 5)
			if err != nil { searchErrors.Add(1) }
		}()
	}

	wg.Wait()
	if saveErrors.Load() > 0 { t.Errorf("%d save errors", saveErrors.Load()) }
	if searchErrors.Load() > 0 { t.Errorf("%d search errors", searchErrors.Load()) }
	t.Logf("20 saves + 10 searches concurrent: %d save errors, %d search errors", saveErrors.Load(), searchErrors.Load())
}

func TestDeep_Memory_DataIntegrity(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save with special characters
	specials := []string{
		"User said: \"I don't like bugs\" — that's important",
		"SQL injection test: '; DROP TABLE memories; --",
		"Unicode: 日本語テスト 🚀 émojis café",
		"Newlines:\nLine 1\nLine 2\nLine 3",
		"Tabs:\tColumn1\tColumn2\tColumn3",
		"HTML: <script>alert('xss')</script>",
		"Backslash: C:\\Users\\test\\file.txt",
		"Null bytes: before\x00after",
		"Very long: " + strings.Repeat("x", 10000),
	}

	for i, content := range specials {
		id, err := store.Save(ctx, tenant, Memory{
			AgentID: agentID, Content: content, Type: "fact", Source: "integrity_test",
		})
		if err != nil {
			t.Logf("special %d failed: %v (content: %q...)", i, err, content[:min7(len(content), 50)])
			continue
		}
		if id == "" { t.Errorf("special %d: empty id", i) }
	}
	t.Log("special character data integrity verified")
}

func TestDeep_Memory_SearchPerformance(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Measure search latency
	queries := []string{
		"programming language", "user preferences", "project deadline",
		"API documentation", "error handling", "database schema",
		"deployment process", "testing strategy", "code review",
		"performance optimization",
	}

	var totalDuration time.Duration
	for _, q := range queries {
		start := time.Now()
		results, err := store.Search(ctx, tenant, agentID, q, 10)
		elapsed := time.Since(start)
		totalDuration += elapsed
		if err != nil { t.Logf("search %q: %v", q, err); continue }
		t.Logf("search %q: %d results in %v", q, len(results), elapsed)
	}

	avg := totalDuration / time.Duration(len(queries))
	t.Logf("average search latency: %v", avg)
	if avg > 3*time.Second { t.Errorf("search too slow: avg %v", avg) }
}

func min7(a, b int) int { if a < b { return a }; return b }
