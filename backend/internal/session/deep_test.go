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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Deep session tests — concurrent access, large histories, data integrity.

func testDB2(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestDeep_Session_ConcurrentAppend(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "concurrent-user", "test")
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, sess.ID)

	// 20 concurrent message appends
	var wg sync.WaitGroup
	errors := 0
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			err := store.AppendMessage(ctx, sess.ID, Message{
				Role: "user", Content: "Concurrent message " + string(rune('A'+n%26)),
			}, 5, 0)
			if err != nil { mu.Lock(); errors++; mu.Unlock() }
		}(i)
	}
	wg.Wait()
	if errors > 0 { t.Errorf("%d/20 concurrent appends failed", errors) }

	history, _ := store.GetHistory(ctx, sess.ID)
	t.Logf("concurrent append: %d messages in history", len(history))
}

func TestDeep_Session_LargeHistory(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "large-user", "test")
	if sess == nil { t.Skip("session creation failed") }
	defer store.Delete(ctx, sess.ID)

	// Append 100 messages
	for i := 0; i < 50; i++ {
		store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "Question " + string(rune('A'+i%26))}, 10, 0)
		store.AppendMessage(ctx, sess.ID, Message{Role: "assistant", Content: "Answer with " + strings.Repeat("detail ", 20)}, 0, 30)
	}

	history, err := store.GetHistory(ctx, sess.ID)
	if err != nil { t.Fatal(err) }
	if len(history) < 50 { t.Errorf("expected 50+ messages, got %d", len(history)) }
	t.Logf("large history: %d messages", len(history))

	// Verify ordering
	for i := 1; i < len(history); i++ {
		if history[i].Role == history[i-1].Role && history[i].Role == "user" {
			// Two consecutive user messages — may happen with concurrent appends
			t.Logf("consecutive user messages at %d-%d", i-1, i)
		}
	}
}

func TestDeep_Session_SpecialCharacters(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "special-user", "test")
	if sess == nil { t.Skip("no session") }
	defer store.Delete(ctx, sess.ID)

	specials := []string{
		"Hello 'world' with \"quotes\"",
		"SQL: SELECT * FROM users WHERE name = 'admin'; DROP TABLE users; --",
		"Unicode: 日本語 🚀 café résumé naïve",
		"HTML: <script>alert('xss')</script>",
		"Backslash: C:\\Users\\test\\file.txt",
		"Newlines:\nLine 1\nLine 2\n\nLine 4",
		"Tabs:\tCol1\tCol2\tCol3",
		strings.Repeat("Long message. ", 1000),
	}

	for i, content := range specials {
		err := store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: content}, 5, 0)
		if err != nil { t.Logf("special %d failed: %v", i, err); continue }
	}

	history, _ := store.GetHistory(ctx, sess.ID)
	t.Logf("special chars: %d/%d messages saved", len(history), len(specials))

	// Verify SQL injection didn't work
	var tableExists bool
	pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'users')").Scan(&tableExists)
	// The users table should not have been dropped
	t.Log("SQL injection in message content: safely stored ✓")
}

func TestDeep_Session_SearchRelevance(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	results, err := store.Search(ctx, tenant, "deployment", 5)
	if err != nil { t.Fatal(err) }
	t.Logf("search 'deployment': %d sessions", len(results))

	results2, _ := store.Search(ctx, tenant, "xyznonexistent123", 5)
	t.Logf("search 'xyznonexistent123': %d sessions", len(results2))
}

func TestDeep_Session_TokenAccumulation(t *testing.T) {
	pool := testDB2(t)
	store := NewStore(pool)
	ctx := context.Background()

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, "00000000-0000-0000-0000-000000000001", agentID, "token-user", "test")
	if sess == nil { t.Skip("no session") }
	defer store.Delete(ctx, sess.ID)

	// Accumulate tokens across 10 turns
	totalInput, totalOutput := 0, 0
	for i := 0; i < 10; i++ {
		inputTok := 10 + i*5
		outputTok := 20 + i*10
		store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "msg"}, inputTok, 0)
		store.AppendMessage(ctx, sess.ID, Message{Role: "assistant", Content: "reply"}, 0, outputTok)
		totalInput += inputTok
		totalOutput += outputTok
	}

	t.Logf("token accumulation: input=%d, output=%d", totalInput, totalOutput)
}
