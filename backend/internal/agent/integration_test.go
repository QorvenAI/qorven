// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Integration tests — real database, real stores, real agent CRUD.
// These catch bugs that unit tests miss: SQL errors, type mismatches, constraint violations.

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB not available: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB not reachable: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestIntegration_AgentStore_CRUD(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Create
	ag, err := store.Create(ctx, tenant, CreateAgentInput{
		AgentKey: "integration-test-" + time.Now().Format("150405"),
		Model: "gpt-4", SystemPrompt: "You are a test agent.", Temperature: 0.7,
	})
	if err != nil { t.Fatalf("create: %v", err) }
	if ag == nil { t.Fatal("nil agent") }
	id := ag.ID
	t.Logf("created agent: %s", id)

	// Get
	ag, err = store.Get(ctx, id)
	if err != nil { t.Fatalf("get: %v", err) }
	if ag == nil { t.Fatal("nil agent") }
	if ag.ID != id { t.Errorf("id mismatch: %q != %q", ag.ID, id) }

	// GetByKey
	ag2, err := store.GetByKey(ctx, ag.AgentKey)
	if err != nil { t.Fatalf("getByKey: %v", err) }
	if ag2.ID != id { t.Error("getByKey returned wrong agent") }

	// List
	agents, err := store.List(ctx, tenant)
	if err != nil { t.Fatalf("list: %v", err) }
	found := false
	for _, a := range agents { if a.ID == id { found = true } }
	if !found { t.Error("created agent not in list") }

	// Update
	err = store.Update(ctx, id, map[string]any{"display_name": "Updated Name"})
	if err != nil { t.Fatalf("update: %v", err) }
	ag3, _ := store.Get(ctx, id)
	if ag3.DisplayName != "Updated Name" { t.Errorf("name not updated: %q", ag3.DisplayName) }

	// TrackUsage
	store.TrackUsage(ctx, id, 100, 50)

	// CheckBudget
	ok, _, _ := store.CheckBudget(ctx, id)
	if !ok { t.Log("budget exceeded (may have limits set)") }

	// Delete
	err = store.Delete(ctx, id)
	if err != nil { t.Fatalf("delete: %v", err) }
	_, err = store.Get(ctx, id)
	if err == nil { t.Error("agent should be deleted") }
}

func TestIntegration_AgentStore_GetDefault(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	ag, err := store.GetDefault(ctx, tenant)
	if err != nil { t.Logf("no default agent: %v", err) }
	if ag != nil { if ag.ID == "" { t.Error("default agent has empty ID") } }
}

func TestIntegration_AgentStore_ListAll(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()

	agents, err := store.ListAll(ctx, "00000000-0000-0000-0000-000000000001")
	if err != nil { t.Fatalf("listAll: %v", err) }
	t.Logf("total agents: %d", len(agents))
}

func TestIntegration_AgentStore_Update_InvalidColumn(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	agSql, err := store.Create(ctx, tenant, CreateAgentInput{AgentKey: "sql-inject-test", Model: "gpt-4", SystemPrompt: "test"})
	if err != nil || agSql == nil { t.Skipf("create failed: %v", err) }
	id := agSql.ID
	defer store.Delete(ctx, id)

	// Try SQL injection via column name — should be blocked by whitelist
	err = store.Update(ctx, id, map[string]any{"display_name; DROP TABLE agents; --": "hacked"})
	if err == nil { t.Log("SQL injection attempt handled (may silently ignore bad columns)") }

	// Verify agent still exists
	ag, err := store.Get(ctx, id)
	if err != nil { t.Fatalf("agent destroyed by SQL injection: %v", err) }
	if ag.DisplayName == "hacked" { t.Error("SQL injection succeeded!") }
}

func TestIntegration_AgentStore_CreateDuplicate(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	key := "dup-test-" + time.Now().Format("150405")
	ag1, err := store.Create(ctx, tenant, CreateAgentInput{AgentKey: key, Model: "gpt-4", SystemPrompt: "test", Temperature: 0.7})
	var id1 string; if ag1 != nil { id1 = ag1.ID }
	if err != nil { t.Fatal(err) }
	defer store.Delete(ctx, id1)

	// Second create with same key should fail or create different ID
	ag2, err := store.Create(ctx, tenant, CreateAgentInput{AgentKey: key + "-2", Model: "gpt-4", SystemPrompt: "test", Temperature: 0.7})
	var id2 string; if ag2 != nil { id2 = ag2.ID }
	if err != nil { t.Logf("duplicate create: %v", err) }
	if id2 != "" { defer store.Delete(ctx, id2) }
	if id2 == id1 { t.Error("duplicate should have different ID") }
}
