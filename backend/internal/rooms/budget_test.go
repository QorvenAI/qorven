package rooms

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func TestBudgetDecision(t *testing.T) {
	if !BudgetAllows(0, 20) {
		t.Errorf("0 turns of 20 should be allowed")
	}
	if !BudgetAllows(19, 20) {
		t.Errorf("19 turns of 20 should be allowed")
	}
	if BudgetAllows(20, 20) {
		t.Errorf("20 turns of 20 should be DENIED (cap reached)")
	}
	if BudgetAllows(25, 20) {
		t.Errorf("25 turns of 20 should be DENIED")
	}
	if !BudgetAllows(1000, 0) {
		t.Errorf("cap 0 means unlimited → allowed")
	}
}

func budgetTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, testsupport.DSN())
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("DB not reachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestBudgetStore_RecordAndCountWindow(t *testing.T) {
	pool := budgetTestPool(t)
	ctx := context.Background()
	s := NewBudgetStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000c1"
	room := "00000000-0000-0000-0000-0000000000c2"
	agentID := "00000000-0000-0000-0000-0000000000a1"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM room_run_budgets WHERE tenant_id=$1", tenant) })

	n, err := s.TurnsInWindow(ctx, room, 10*time.Minute)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 turns, got %d", n)
	}

	for i := 0; i < 3; i++ {
		if err := s.RecordTurn(ctx, tenant, room, agentID); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	n2, err := s.TurnsInWindow(ctx, room, 10*time.Minute)
	if err != nil {
		t.Fatalf("count after 3: %v", err)
	}
	if n2 != 3 {
		t.Fatalf("expected 3 turns in window, got %d", n2)
	}

	// A row backdated 1 hour must fall OUTSIDE a 30-minute window.
	if _, err := pool.Exec(ctx,
		`INSERT INTO room_run_budgets (tenant_id, room_id, agent_id, created_at) VALUES ($1,$2,$3, now() - interval '1 hour')`,
		tenant, room, agentID); err != nil {
		t.Fatalf("insert backdated: %v", err)
	}
	n3, err := s.TurnsInWindow(ctx, room, 30*time.Minute)
	if err != nil {
		t.Fatalf("count window: %v", err)
	}
	if n3 != 3 {
		t.Errorf("30-min window should still be 3 (backdated row excluded), got %d", n3)
	}
}
