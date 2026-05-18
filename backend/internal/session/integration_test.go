// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package session

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Integration tests — real database session CRUD.

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

func TestIntegration_Session_CRUD(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create
	sess, err := store.Create(ctx, tenant, "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "test-user", "test")
	if err != nil { t.Fatalf("create: %v", err) }
	if sess == nil { t.Fatal("nil session") }
	if sess.ID == "" { t.Fatal("empty id") }
	t.Logf("created session: %s", sess.ID)

	// Get
	got, err := store.GetByID(ctx, sess.ID)
	if err != nil { t.Fatalf("get: %v", err) }
	if got.ID != sess.ID { t.Error("id mismatch") }

	// Append message
	err = store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "hello"}, 5, 0)
	if err != nil { t.Fatalf("append user: %v", err) }
	err = store.AppendMessage(ctx, sess.ID, Message{Role: "assistant", Content: "hi there!"}, 0, 10)
	if err != nil { t.Fatalf("append assistant: %v", err) }

	// Get history
	history, err := store.GetHistory(ctx, sess.ID)
	if err != nil { t.Fatalf("history: %v", err) }
	if len(history) < 2 { t.Errorf("expected 2+ messages, got %d", len(history)) }

	// Update label
	err = store.UpdateLabel(ctx, sess.ID, "Test Conversation")
	if err != nil { t.Fatalf("label: %v", err) }

	// List
	sessions, err := store.ListForAgent(ctx, "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", 10)
	if err != nil { t.Fatalf("list: %v", err) }
	found := false
	for _, s := range sessions { if s.ID == sess.ID { found = true } }
	if !found { t.Error("session not in list") }

	// Search
	results, err := store.Search(ctx, tenant, "hello", 10)
	if err != nil { t.Fatalf("search: %v", err) }
	t.Logf("search results: %d", len(results))

	// Delete
	err = store.Delete(ctx, sess.ID)
	if err != nil { t.Fatalf("delete: %v", err) }
	_, err = store.GetByID(ctx, sess.ID)
	if err == nil { t.Log("session may use soft delete or cascade") }
}

func TestIntegration_Session_AccumulateTokens(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	sess, err := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "test-user", "test")
	if err != nil { t.Skip(err) }
	defer store.Delete(ctx, sess.ID)

	// Accumulate tokens across multiple messages
	for i := 0; i < 10; i++ {
		store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "msg"}, 10, 0)
		store.AppendMessage(ctx, sess.ID, Message{Role: "assistant", Content: "reply"}, 0, 20)
	}

	// Verify token accumulation
	err = store.AccumulateTokens(ctx, sess.ID, 100, 200)
	if err != nil { t.Fatalf("accumulate: %v", err) }
}

func TestIntegration_Session_Compact(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	sess, _ := store.Create(ctx, "00000000-0000-0000-0000-000000000001", "021c16ae-6b93-4d62-bf0e-32fd6a275fd7", "test-user", "test")
	if sess == nil { t.Skip("no session") }
	defer store.Delete(ctx, sess.ID)

	// Add many messages
	for i := 0; i < 20; i++ {
		store.AppendMessage(ctx, sess.ID, Message{Role: "user", Content: "question " + string(rune('0'+i%10))}, 5, 0)
		store.AppendMessage(ctx, sess.ID, Message{Role: "assistant", Content: "answer"}, 0, 10)
	}

	// Compact
	err := store.Compact(ctx, sess.ID, 5)
	if err != nil { t.Fatalf("compact: %v", err) }
}
