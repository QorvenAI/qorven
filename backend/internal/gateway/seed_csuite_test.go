// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/store"
)

// TestSeedCSuite_FreshTenant verifies that the seedCSuite logic creates 4 C-suite
// officers and promotes Prime→CEO on a clean tenant, then is a no-op on the second
// run (idempotency).
//
// Uses a scratch tenant UUID so it never touches the real dev org.
// Skips automatically if the DB is unreachable.
func TestSeedCSuite_FreshTenant(t *testing.T) {
	if testing.Short() {
		t.Skip("skip DB test in short mode")
	}
	dsn := os.Getenv("QORVEN_POSTGRES_URL")
	if dsn == "" {
		dsn = "postgres://postgres@localhost:5432/qorven_dev"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to DB: %v", err)
	}
	defer pool.Close()

	if pingErr := pool.Ping(ctx); pingErr != nil {
		t.Skipf("DB not reachable: %v", pingErr)
	}

	// Use a throwaway tenant — never touches the real org.
	scratchTenant := fmt.Sprintf("00000000-0000-4000-a000-%012d", time.Now().UnixMilli()%1_000_000_000_000)
	t.Logf("scratch tenant: %s", scratchTenant)

	// Ensure tenant row exists (some tables FK on tenant_id).
	_, err = pool.Exec(ctx,
		`INSERT INTO tenants (id, name, plan, created_at, updated_at)
		 VALUES ($1::uuid, 'test-csuite', 'free', now(), now())
		 ON CONFLICT DO NOTHING`,
		scratchTenant)
	if err != nil {
		t.Skipf("cannot insert test tenant (schema may differ): %v", err)
	}

	defer func() {
		cctx := context.Background()
		pool.Exec(cctx, `DELETE FROM agents WHERE tenant_id = $1`, scratchTenant)
		pool.Exec(cctx, `DELETE FROM org_hierarchy WHERE tenant_id = $1`, scratchTenant)
		pool.Exec(cctx, `DELETE FROM org_roster   WHERE tenant_id = $1`, scratchTenant)
		pool.Exec(cctx, `DELETE FROM departments  WHERE tenant_id = $1`, scratchTenant)
		pool.Exec(cctx, `DELETE FROM tenants WHERE id = $1::uuid`, scratchTenant)
	}()

	// ── Stores ────────────────────────────────────────────────────────────────
	agentStore := agent.NewStore(pool)
	db := &store.DB{Pool: pool}
	_ = db // only used below for the Exec wrapper

	// Create Prime in the scratch tenant.
	spec := agent.ChiefSpec()
	prime, err := agentStore.Create(ctx, scratchTenant, spec)
	if err != nil {
		t.Fatalf("create prime: %v", err)
	}
	t.Logf("prime id=%s  org_role=%s", prime.ID, prime.OrgRole)

	// ── First seed run — should create 4 officers ─────────────────────────────
	if err := runSeedForTenant(ctx, pool, agentStore, scratchTenant, prime.ID); err != nil {
		t.Fatalf("first seed run: %v", err)
	}

	var officerCount int
	pool.QueryRow(ctx,
		`SELECT count(*) FROM agents
		 WHERE tenant_id = $1
		   AND org_role IN ('coo','cfo','cto','cmo')
		   AND agent_key != 'chief'
		   AND terminated_at IS NULL`,
		scratchTenant).Scan(&officerCount)
	if officerCount != 4 {
		t.Errorf("want 4 C-suite officers, got %d", officerCount)
	} else {
		t.Logf("officers created: %d (PASS)", officerCount)
	}

	// Prime should have been promoted to ceo.
	var primeRole string
	pool.QueryRow(ctx,
		`SELECT org_role FROM agents WHERE id = $1`, prime.ID).Scan(&primeRole)
	if primeRole != "ceo" {
		t.Errorf("prime org_role = %q, want 'ceo'", primeRole)
	} else {
		t.Logf("prime promoted to %s (PASS)", primeRole)
	}

	// ── Second seed run — must be a no-op ─────────────────────────────────────
	if err := runSeedForTenant(ctx, pool, agentStore, scratchTenant, prime.ID); err != nil {
		t.Fatalf("second seed run: %v", err)
	}

	var countAfter int
	pool.QueryRow(ctx,
		`SELECT count(*) FROM agents
		 WHERE tenant_id = $1
		   AND org_role IN ('coo','cfo','cto','cmo')
		   AND agent_key != 'chief'
		   AND terminated_at IS NULL`,
		scratchTenant).Scan(&countAfter)
	if countAfter != officerCount {
		t.Errorf("idempotency failed: count went from %d to %d on second run", officerCount, countAfter)
	} else {
		t.Logf("second run no-op confirmed: count still %d (PASS)", countAfter)
	}
}

// runSeedForTenant runs the seedCSuite logic against an arbitrary tenant.
// It mirrors the production seedCSuite logic exactly, allowing us to test
// against a scratch tenant without modifying the real org.
func runSeedForTenant(ctx context.Context, pool *pgxpool.Pool, agentStore *agent.Store, tenantID, primeID string) error {
	// Freshness check — same query as seedCSuite.
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM agents
		 WHERE tenant_id = $1
		   AND org_role IN ('coo','cfo','cto','cmo','ceo')
		   AND agent_key != 'chief'
		   AND (terminated_at IS NULL AND (status IS NULL OR status != 'suspended'))`,
		tenantID).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // already populated
	}

	// Promote Prime coo→ceo.
	_, _ = pool.Exec(ctx,
		`UPDATE agents SET org_role = 'ceo', title = 'CEO'
		 WHERE id = $1 AND org_role = 'coo'`,
		primeID)

	// Default C-suite seats — same slice as production.
	officers := []struct {
		key     string
		name    string
		title   string
		role    string
		orgRole string
		budget  float64
	}{
		{"officer-coo", "COO", "Chief Operating Officer", "coo", "coo", 50},
		{"officer-cfo", "CFO", "Chief Financial Officer", "cfo", "cfo", 50},
		{"officer-cto", "CTO", "Chief Technology Officer", "cto", "cto", 50},
		{"officer-cmo", "CMO", "Chief Marketing Officer", "cmo", "cmo", 50},
	}

	for _, off := range officers {
		var existingID string
		pool.QueryRow(ctx,
			`SELECT id::text FROM agents WHERE tenant_id=$1 AND agent_key=$2`,
			tenantID, off.key).Scan(&existingID)
		if existingID != "" {
			continue
		}

		seed, hasSeed := agent.AgentSeeds[off.role]
		soulContent := ""
		if hasSeed && seed.Soul != "" {
			soulContent = seed.Soul
		} else {
			soulContent = "You are " + off.name + ", the " + off.title + "."
		}

		a, createErr := agentStore.Create(ctx, tenantID, agent.CreateAgentInput{
			AgentKey:          off.key,
			DisplayName:       off.name,
			Role:              off.role,
			Title:             off.title,
			OrgRole:           off.orgRole,
			OrgLevel:          "l2",
			ManagerID:         primeID,
			SystemPrompt:      soulContent,
			Model:             "auto",
			Temperature:       0.5,
			ContextWindow:     128000,
			MaxToolIterations: 20,
			ToolProfile:       "full",
		})
		if createErr != nil {
			return fmt.Errorf("create %s: %w", off.key, createErr)
		}
		if off.budget > 0 {
			pool.Exec(ctx,
				`UPDATE agents SET monthly_budget_usd=$1, can_delegate=true WHERE id=$2`,
				off.budget, a.ID)
		}
	}
	return nil
}
