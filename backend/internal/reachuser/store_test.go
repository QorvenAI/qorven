package reachuser

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func storeTestPool(t *testing.T) *pgxpool.Pool {
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

func TestStore_OpenDueAdvanceAck(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000bb"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	id, err := s.Open(ctx, Escalation{
		TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "n1",
		Title: "Approve deploy", Body: "ship v2", Urgency: "normal", CurrentRung: 1,
	}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	due, err := s.DuePending(ctx, time.Now())
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	found := false
	for _, e := range due {
		if e.ID == id {
			found = true
			if e.CurrentRung != 1 || e.Urgency != "normal" {
				t.Errorf("unexpected escalation: %+v", e)
			}
		}
	}
	if !found {
		t.Fatalf("expected due escalation %s in %d rows", id, len(due))
	}

	if err := s.Advance(ctx, id, 2, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("advance: %v", err)
	}
	due2, _ := s.DuePending(ctx, time.Now())
	for _, e := range due2 {
		if e.ID == id {
			t.Fatalf("escalation should not be due after advancing to future time")
		}
	}

	if err := s.Ack(ctx, "notification", "n1"); err != nil {
		t.Fatalf("ack: %v", err)
	}
	got, err := s.get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "acked" {
		t.Errorf("expected status acked, got %q", got.Status)
	}
}

func TestStore_OpenWithZeroTime_NotDue(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000bf"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	// Zero next_advance_at => stored as NULL => never appears in DuePending.
	id, err := s.Open(ctx, Escalation{
		TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "z1",
		Title: "low", Urgency: "low", CurrentRung: 1,
	}, time.Time{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	due, err := s.DuePending(ctx, time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	for _, e := range due {
		if e.ID == id {
			t.Fatalf("zero-time escalation must NOT be due (should be NULL next_advance_at)")
		}
	}
}
