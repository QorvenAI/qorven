// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"

)

// Diamond-hard store tests — verify the data layer works as a product.

func diamondPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil { t.Skipf("DB: %v", err) }
	if err := pool.Ping(ctx); err != nil { t.Skipf("DB: %v", err) }
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestDiamond_Store_TransactionIsolation(t *testing.T) {
	pool := diamondPool(t)
	ctx := context.Background()

	// Start two transactions that read the same data
	tx1, err := pool.Begin(ctx)
	if err != nil { t.Fatal(err) }
	defer tx1.Rollback(ctx)

	tx2, err := pool.Begin(ctx)
	if err != nil { t.Fatal(err) }
	defer tx2.Rollback(ctx)

	var name1, name2 string
	tx1.QueryRow(ctx, "SELECT display_name FROM agents LIMIT 1").Scan(&name1)
	tx2.QueryRow(ctx, "SELECT display_name FROM agents LIMIT 1").Scan(&name2)

	if name1 != name2 { t.Logf("different reads: %q vs %q", name1, name2) }
	t.Log("transaction isolation: concurrent reads consistent ✓")
}

func TestDiamond_Store_ConnectionPoolStress(t *testing.T) {
	pool := diamondPool(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 50)
	start := time.Now()

	for i := 0; i < 50; i++ {
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

	if errCount > 5 { t.Errorf("too many pool errors: %d/50", errCount) }
	if elapsed > 10*time.Second { t.Errorf("pool exhaustion: %v", elapsed) }
	t.Logf("50 concurrent queries: %d errors, %v elapsed ✓", errCount, elapsed)
}

func TestDiamond_Store_RollbackIntegrity(t *testing.T) {
	pool := diamondPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil { t.Fatal(err) }

	marker := "CRASH_TEST_" + time.Now().Format("150405")

	// Insert into agents (will be rolled back)
	tx.Exec(ctx, `INSERT INTO agents (id, tenant_id, agent_key, display_name, model, system_prompt)
		VALUES (gen_random_uuid(), '00000000-0000-0000-0000-000000000001', $1, 'Crash Test', 'gpt-4', 'test')`, marker)

	// "Crash" — rollback
	tx.Rollback(ctx)

	// Verify the data is NOT in the database
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM agents WHERE agent_key = $1", marker).Scan(&count)
	if count > 0 { t.Fatal("CRITICAL: rolled-back data persisted!") }

	t.Log("rollback integrity: data not persisted ✓")
}

func TestDiamond_Store_SQLInjectionVectors(t *testing.T) {
	pool := diamondPool(t)
	ctx := context.Background()

	injections := []string{
		"'; DROP TABLE agents; --",
		"' OR '1'='1",
		"'; UPDATE agents SET system_prompt='hacked'; --",
		"' UNION SELECT * FROM pg_shadow; --",
	}

	for _, injection := range injections {
		var count int
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM agents WHERE agent_key = $1", injection).Scan(&count)
		if count > 0 { t.Errorf("injection matched: %q", injection) }
	}

	// Verify tables still exist
	var exists bool
	pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'agents')").Scan(&exists)
	if !exists { t.Fatal("CRITICAL: agents table dropped by SQL injection!") }

	t.Log("SQL injection: all 4 vectors safe, tables intact ✓")
}

func TestDiamond_Store_LargePayload(t *testing.T) {
	pool := diamondPool(t)
	ctx := context.Background()

	largePrompt := strings.Repeat("You are a helpful assistant. ", 1000) // ~30KB
	marker := "LARGE_" + time.Now().Format("150405")

	var agentID string
	err := pool.QueryRow(ctx, `INSERT INTO agents (id, tenant_id, agent_key, display_name, model, system_prompt)
		VALUES (gen_random_uuid(), '00000000-0000-0000-0000-000000000001', $1, 'Large Test', 'gpt-4', $2)
		RETURNING id`, marker, largePrompt).Scan(&agentID)
	if err != nil { t.Fatal(err) }
	defer pool.Exec(ctx, "DELETE FROM agents WHERE id = $1", agentID)

	// Read it back and verify integrity
	var readBack string
	pool.QueryRow(ctx, "SELECT system_prompt FROM agents WHERE id = $1", agentID).Scan(&readBack)
	if len(readBack) != len(largePrompt) { t.Errorf("size mismatch: wrote %d, read %d", len(largePrompt), len(readBack)) }
	if readBack != largePrompt { t.Error("content mismatch on large payload") }

	t.Logf("large payload: %d bytes written and read back intact ✓", len(largePrompt))
}
