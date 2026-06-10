package workitems

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func wiTestPool(t *testing.T) *pgxpool.Pool {
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

func TestStore_CreateTransitionBlockEvents(t *testing.T) {
	pool := wiTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000e1"
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM work_item_events WHERE work_item_id IN (SELECT id FROM work_items WHERE tenant_id=$1)", tenant)
		pool.Exec(ctx, "DELETE FROM work_items WHERE tenant_id=$1", tenant)
	})

	id, err := s.Create(ctx, WorkItem{TenantID: tenant, Title: "Rebuild onboarding", RequestedBy: "u1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil || got.Status != StatusOpen {
		t.Fatalf("get after create: want open, got %+v err=%v", got, err)
	}

	if err := s.Transition(ctx, id, StatusAssigned, "cto", "assigned to L3"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := s.Transition(ctx, id, StatusDone, "cto", "skip"); err == nil {
		t.Fatalf("expected illegal transition assigned→done to error")
	}

	if err := s.SetBlockedOn(ctx, id, "approval", "appr-123", "cto"); err != nil {
		t.Fatalf("block: %v", err)
	}
	got2, _ := s.Get(ctx, id)
	if got2.Status != StatusBlocked || got2.BlockedOnKind != "approval" || got2.BlockedOnID != "appr-123" {
		t.Fatalf("after block want blocked/approval/appr-123, got %+v", got2)
	}

	evs, err := s.Events(ctx, id)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(evs) < 3 {
		t.Errorf("expected ≥3 events (create, assign, block), got %d", len(evs))
	}
}

func TestStore_Unblock_RequiresBlocked(t *testing.T) {
	pool := wiTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000e2"
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM work_item_events WHERE work_item_id IN (SELECT id FROM work_items WHERE tenant_id=$1)", tenant)
		pool.Exec(ctx, "DELETE FROM work_items WHERE tenant_id=$1", tenant)
	})
	id, _ := s.Create(ctx, WorkItem{TenantID: tenant, Title: "x", RequestedBy: "u1"})
	// Not blocked yet → Unblock errors.
	if err := s.Unblock(ctx, id, "u1"); err == nil {
		t.Fatalf("expected Unblock on a non-blocked item to error")
	}
	// Block then unblock → in_progress, accurate FromStatus.
	if err := s.SetBlockedOn(ctx, id, "approval", "a1", "u1"); err != nil {
		t.Fatalf("block: %v", err)
	}
	if err := s.Unblock(ctx, id, "u1"); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	got, _ := s.Get(ctx, id)
	if got.Status != StatusInProgress || got.BlockedOnID != "" {
		t.Errorf("after unblock want in_progress + cleared block, got %+v", got)
	}
}

func TestStore_ListForRoom(t *testing.T) {
	pool := wiTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000e2"
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM work_item_events WHERE work_item_id IN (SELECT id FROM work_items WHERE tenant_id=$1)", tenant)
		pool.Exec(ctx, "DELETE FROM work_items WHERE tenant_id=$1", tenant)
	})

	// Two items in room R1, one in room R2.
	id1, err := s.Create(ctx, WorkItem{TenantID: tenant, Title: "first", Origin: "room:R1", RequestedBy: "u1"})
	if err != nil {
		t.Fatalf("create id1: %v", err)
	}
	if _, err := s.Create(ctx, WorkItem{TenantID: tenant, Title: "second", Origin: "room:R1", RequestedBy: "u1"}); err != nil {
		t.Fatalf("create id2: %v", err)
	}
	if _, err := s.Create(ctx, WorkItem{TenantID: tenant, Title: "other", Origin: "room:R2", RequestedBy: "u1"}); err != nil {
		t.Fatalf("create id3: %v", err)
	}

	// R1 returns exactly the two R1 items.
	got, err := s.ListForRoom(ctx, tenant, "R1", "")
	if err != nil {
		t.Fatalf("list R1: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("R1: want 2 items, got %d", len(got))
	}

	// Status filter narrows: move id1 to assigned, filter by 'assigned' → 1.
	if err := s.Transition(ctx, id1, StatusAssigned, "u1", "assigned"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	assigned, err := s.ListForRoom(ctx, tenant, "R1", StatusAssigned)
	if err != nil {
		t.Fatalf("list assigned: %v", err)
	}
	if len(assigned) != 1 || assigned[0].Title != "first" {
		t.Fatalf("assigned filter: want [first], got %+v", assigned)
	}
}

func TestStore_SetBlockedOn_RejectsTerminal(t *testing.T) {
	pool := wiTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000e3"
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM work_item_events WHERE work_item_id IN (SELECT id FROM work_items WHERE tenant_id=$1)", tenant)
		pool.Exec(ctx, "DELETE FROM work_items WHERE tenant_id=$1", tenant)
	})
	id, _ := s.Create(ctx, WorkItem{TenantID: tenant, Title: "x", RequestedBy: "u1"})
	// open → cancelled (terminal).
	if err := s.Transition(ctx, id, StatusCancelled, "u1", ""); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// Blocking a cancelled item must error.
	if err := s.SetBlockedOn(ctx, id, "approval", "a1", "u1"); err == nil {
		t.Fatalf("expected SetBlockedOn on a cancelled item to error")
	}
}
