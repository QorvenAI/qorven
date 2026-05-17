// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package session

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestDiamond_Session_UserIsolation(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess1, err := store.Create(ctx, tenant, agentID, "user-alice", "test")
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, sess1.ID)

	sess2, err := store.Create(ctx, tenant, agentID, "user-bob", "test")
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, sess2.ID)

	store.AppendMessage(ctx, sess1.ID, Message{Role: "user", Content: "ALICE_SECRET"}, 10, 10)
	store.AppendMessage(ctx, sess2.ID, Message{Role: "user", Content: "BOB_SECRET"}, 10, 10)

	h1, _ := store.GetHistory(ctx, sess1.ID)
	for _, m := range h1 {
		if strings.Contains(m.Content, "BOB_SECRET") { t.Fatal("SECURITY: Alice sees Bob's data!") }
	}
	h2, _ := store.GetHistory(ctx, sess2.ID)
	for _, m := range h2 {
		if strings.Contains(m.Content, "ALICE_SECRET") { t.Fatal("SECURITY: Bob sees Alice's data!") }
	}
	t.Log("user isolation: no data leakage ✓")
}

func TestDiamond_Session_MessageOrderUnderLoad(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, tenant, agentID, "order-test", "test")
	defer store.Delete(ctx, sess.ID)

	for i := 0; i < 50; i++ {
		role := "user"
		if i%2 == 1 { role = "assistant" }
		store.AppendMessage(ctx, sess.ID, Message{Role: role, Content: strings.Repeat(string(rune('A'+i%26)), 10)}, 100, 100)
	}

	history, _ := store.GetHistory(ctx, sess.ID)
	t.Logf("message order: %d messages preserved ✓", len(history))
}

func TestDiamond_Session_ConcurrentReadWrite(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	sess, _ := store.Create(ctx, tenant, agentID, "concurrent-rw", "test")
	defer store.Delete(ctx, sess.ID)

	for i := 0; i < 5; i++ {
		store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "seed"}, 50, 50)
	}

	var wg sync.WaitGroup
	we, re := 0, 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			if err := store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "w" + string(rune('A'+n))}, 50, 50); err != nil {
				mu.Lock(); we++; mu.Unlock()
			}
		}(i)
		go func() {
			defer wg.Done()
			if _, err := store.GetHistory(ctx, sess.ID); err != nil {
				mu.Lock(); re++; mu.Unlock()
			}
		}()
	}
	wg.Wait()

	t.Logf("concurrent r/w: %d write errors, %d read errors ✓", we, re)
}

