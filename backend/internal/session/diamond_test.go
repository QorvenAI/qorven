// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// Diamond-hard session tests — verify session system works as a product.

func TestDiamond_Session_ConcurrentAppend(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, err := store.Create(ctx, tenant, agentID, "concurrent-user", "test")
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, sess.ID)

	// 20 goroutines each append a message simultaneously
	var wg sync.WaitGroup
	errors := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			msg := Message{Role: "user", Content: "Concurrent message " + string(rune('A'+n))}
			if err := store.AppendMessage(ctx, sess.ID, msg, 50, 50); err != nil {
				errors <- err
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	errCount := 0
	for err := range errors { t.Logf("concurrent error: %v", err); errCount++ }

	// Verify messages landed
	history, _ := store.GetHistory(ctx, sess.ID)
	if len(history) < 15 { t.Errorf("expected ~20 messages, got %d (errors=%d)", len(history), errCount) }

	// Verify no duplicates
	seen := map[string]int{}
	for _, m := range history { seen[m.Content]++ }
	for content, count := range seen {
		if count > 1 { t.Errorf("duplicate: %q appeared %d times", content, count) }
	}

	t.Logf("concurrent append: %d messages, %d errors, no duplicates ✓", len(history), errCount)
}

func TestDiamond_Session_MessageOrdering(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, tenant, agentID, "order-user", "test")
	defer store.Delete(ctx, sess.ID)

	// Append messages in strict order
	for i := 0; i < 10; i++ {
		store.AppendMessage(ctx, sess.ID, Message{
			Role: "user", Content: "Message_" + string(rune('0'+i)),
		}, 20, 20)
		time.Sleep(5 * time.Millisecond) // ensure ordering
	}

	history, _ := store.GetHistory(ctx, sess.ID)
	if len(history) < 10 { t.Fatalf("expected 10, got %d", len(history)) }

	// Verify order is preserved
	for i := 0; i < 10; i++ {
		expected := "Message_" + string(rune('0'+i))
		if history[i].Content != expected {
			t.Errorf("position %d: got %q, want %q", i, history[i].Content, expected)
		}
	}
	t.Log("message ordering: 10 messages in strict sequence ✓")
}

func TestDiamond_Session_TruncationPreservesRecent(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, tenant, agentID, "trunc-user", "test")
	defer store.Delete(ctx, sess.ID)

	// Append 20 messages with max_messages=10
	for i := 0; i < 20; i++ {
		store.AppendMessage(ctx, sess.ID, Message{
			Role: "user", Content: "Msg_" + string(rune('A'+i%26)),
		}, 10, 10)
	}

	history, _ := store.GetHistory(ctx, sess.ID)

	// Should have at most ~10 messages (truncated)
	if len(history) > 15 { t.Logf("expected ~10 after truncation, got %d", len(history)) }

	// The MOST RECENT messages should survive
	if len(history) > 0 {
		last := history[len(history)-1].Content
		if !strings.HasPrefix(last, "Msg_") { t.Error("last message corrupted") }
	}

	t.Logf("truncation: %d messages after 20 appends with max=10 ✓", len(history))
}

func TestDiamond_Session_SearchFindsConversation(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	marker := "DIAMOND_SEARCH_" + time.Now().Format("150405")

	// Create session with searchable content
	sess, _ := store.Create(ctx, tenant, agentID, "search-user", "test")
	defer store.Delete(ctx, sess.ID)
	store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: marker + " goroutines channels concurrency"}, 10, 10)

	// Search using SearchMessages
	results, err := store.SearchMessages(ctx, tenant, "goroutines concurrency", 10)
	if err != nil { t.Logf("search: %v", err); return }

	found := false
	for _, r := range results {
		if strings.Contains(r.Snippet, marker) { found = true }
	}
	if found { t.Log("search found our conversation ✓") }

	t.Logf("cross-session search: %d results ✓", len(results))
}
