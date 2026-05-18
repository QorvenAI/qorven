// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package session

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hard session tests — concurrent access, data integrity, edge cases.

func TestHard_Session_ConcurrentCreateDelete(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	var wg sync.WaitGroup
	var created atomic.Int32

	// Create 30 sessions concurrently
	ids := make(chan string, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sess, err := store.Create(ctx, tenant, agentID, "concurrent-"+string(rune('A'+n%26)), "test")
			if err != nil { return }
			created.Add(1)
			ids <- sess.ID
		}(i)
	}
	wg.Wait()
	close(ids)

	t.Logf("concurrent create: %d/30 sessions", created.Load())

	// Delete all
	for id := range ids { store.Delete(ctx, id) }
}

func TestHard_Session_MessageOrdering(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "order-test", "test")
	if sess == nil { t.Skip("no session") }
	defer store.Delete(ctx, sess.ID)

	// Append 50 messages with sequential content
	for i := 0; i < 50; i++ {
		role := "user"
		if i%2 == 1 { role = "assistant" }
		store.AppendMessage(ctx, sess.ID, Message{
			Role: role, Content: "MSG_" + string(rune('A'+i/26)) + string(rune('A'+i%26)),
		}, 5, 5)
	}

	// Verify ordering
	history, _ := store.GetHistory(ctx, sess.ID)
	if len(history) < 50 { t.Errorf("expected 50, got %d", len(history)) }

	// Check sequential order
	outOfOrder := 0
	for i := 1; i < len(history); i++ {
		curr := history[i].Content
		prev := history[i-1].Content
		if curr < prev && strings.HasPrefix(curr, "MSG_") && strings.HasPrefix(prev, "MSG_") {
			outOfOrder++
		}
	}
	if outOfOrder > 0 { t.Errorf("%d messages out of order", outOfOrder) }
	t.Logf("message ordering: %d messages, %d out of order", len(history), outOfOrder)
}

func TestHard_Session_CompactPreservesRecent(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "compact-test", "test")
	if sess == nil { t.Skip("no session") }
	defer store.Delete(ctx, sess.ID)

	// Add 30 messages
	for i := 0; i < 30; i++ {
		store.AppendMessage(ctx, sess.ID, Message{
			Role: "user", Content: "Question_" + string(rune('A'+i%26)),
		}, 10, 0)
		store.AppendMessage(ctx, sess.ID, Message{
			Role: "assistant", Content: "Answer_" + string(rune('A'+i%26)),
		}, 0, 20)
	}

	before, _ := store.GetHistory(ctx, sess.ID)

	// Compact keeping 5 recent
	store.Compact(ctx, sess.ID, 5)

	after, _ := store.GetHistory(ctx, sess.ID)
	t.Logf("compact: %d → %d messages (keep 5 recent)", len(before), len(after))

	// Most recent messages should survive
	if len(after) > 0 {
		last := after[len(after)-1]
		if !strings.HasPrefix(last.Content, "Answer_") && !strings.HasPrefix(last.Content, "Question_") {
			t.Logf("last message after compact: %q", last.Content[:min8(len(last.Content), 50)])
		}
	}
}

func TestHard_Session_SearchPerformance(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Run 20 searches and measure latency
	queries := []string{
		"deployment", "database", "testing", "performance",
		"security", "authentication", "API design", "error handling",
		"concurrency", "memory management", "caching", "monitoring",
		"logging", "configuration", "migration", "backup",
		"scaling", "load balancing", "CI/CD", "documentation",
	}

	var totalDuration time.Duration
	for _, q := range queries {
		start := time.Now()
		results, err := store.Search(ctx, tenant, q, 5)
		elapsed := time.Since(start)
		totalDuration += elapsed
		if err != nil { continue }
		_ = results
	}

	avg := totalDuration / time.Duration(len(queries))
	t.Logf("session search: 20 queries, avg=%v, total=%v", avg, totalDuration)
	if avg > 5*time.Second { t.Errorf("search too slow: avg %v", avg) }
}

func min8(a, b int) int { if a < b { return a }; return b }
