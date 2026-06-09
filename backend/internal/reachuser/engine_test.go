package reachuser

import (
	"context"
	"testing"
	"time"
)

// fakeDeliverer records what was delivered and reports presence.
type fakeDeliverer struct {
	online    bool
	delivered []string // channel of each delivery
	failIM    bool
}

func (f *fakeDeliverer) IsOnline(ctx context.Context, userID string) bool { return f.online }
func (f *fakeDeliverer) Deliver(ctx context.Context, e Escalation, rung int, channel string) error {
	f.delivered = append(f.delivered, channel)
	if channel == ChannelIM && f.failIM {
		return context.DeadlineExceeded
	}
	return nil
}

func TestEngine_Open_DeliversInAppImmediately(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	st := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000cc"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	fd := &fakeDeliverer{online: false}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	eng := NewEngine(st, fd, func() time.Time { return now })

	id, err := eng.Open(ctx, Escalation{TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "n1", Title: "t", Urgency: "normal"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelInApp {
		t.Fatalf("expected in-app delivered on open, got %v", fd.delivered)
	}
	got, _ := st.get(ctx, id)
	if got.CurrentRung != 1 || got.Status != "pending" {
		t.Errorf("after open want rung1/pending, got rung %d status %s", got.CurrentRung, got.Status)
	}
}

func TestEngine_Tick_AdvancesDueToIMThenEmailThenExhausts(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	st := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000cd"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	fd := &fakeDeliverer{online: false}
	now := time.Now()
	clock := now
	eng := NewEngine(st, fd, func() time.Time { return clock })

	id, _ := eng.Open(ctx, Escalation{TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "n2", Urgency: "normal"})
	fd.delivered = nil

	clock = now.Add(6 * time.Minute)
	if err := eng.Tick(ctx); err != nil {
		t.Fatalf("tick1: %v", err)
	}
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelIM {
		t.Fatalf("tick1 expected IM, got %v", fd.delivered)
	}

	clock = clock.Add(31 * time.Minute)
	fd.delivered = nil
	eng.Tick(ctx)
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelEmail {
		t.Fatalf("tick2 expected email, got %v", fd.delivered)
	}
	got, _ := st.get(ctx, id)
	if got.Status != "exhausted" {
		t.Errorf("after email want exhausted, got %s", got.Status)
	}
}

func TestEngine_Tick_FailedDeliveryRetriesSameRung(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	st := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000cf"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	fd := &fakeDeliverer{online: false, failIM: true}
	now := time.Now()
	clock := now
	eng := NewEngine(st, fd, func() time.Time { return clock })
	id, _ := eng.Open(ctx, Escalation{TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "nf", Urgency: "normal"})

	// climb to IM (which fails)
	clock = now.Add(6 * time.Minute)
	fd.delivered = nil
	eng.Tick(ctx)
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelIM {
		t.Fatalf("expected IM attempt, got %v", fd.delivered)
	}
	got, _ := st.get(ctx, id)
	if got.Status != "pending" || got.CurrentRung != 1 {
		t.Fatalf("after failed IM want pending rung1 (retry same step), got status %s rung %d", got.Status, got.CurrentRung)
	}
	// not due yet (backed off 60s); a tick now does nothing
	fd.delivered = nil
	eng.Tick(ctx)
	if len(fd.delivered) != 0 {
		t.Errorf("should not retry before back-off elapses, got %v", fd.delivered)
	}
	// after back-off, IM retried
	clock = clock.Add(61 * time.Second)
	fd.failIM = false
	eng.Tick(ctx)
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelIM {
		t.Errorf("expected IM retry after back-off, got %v", fd.delivered)
	}
}

func TestEngine_Ack_StopsClimb(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	st := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000ce"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	fd := &fakeDeliverer{online: false}
	now := time.Now()
	clock := now
	eng := NewEngine(st, fd, func() time.Time { return clock })
	eng.Open(ctx, Escalation{TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "n3", Urgency: "normal"})

	if err := eng.Ack(ctx, "notification", "n3"); err != nil {
		t.Fatalf("ack: %v", err)
	}
	clock = now.Add(10 * time.Minute)
	fd.delivered = nil
	eng.Tick(ctx)
	if len(fd.delivered) != 0 {
		t.Errorf("acked escalation must not deliver further, got %v", fd.delivered)
	}
}

func TestEngine_Urgent_ClimbsImmediatelyOnTick(t *testing.T) {
	pool := storeTestPool(t)
	ctx := context.Background()
	st := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000d0"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM escalations WHERE tenant_id=$1", tenant) })

	fd := &fakeDeliverer{online: false}
	now := time.Now()
	eng := NewEngine(st, fd, func() time.Time { return now }) // clock does NOT advance

	// Urgent open delivers in-app and stores next_advance_at=now (immediately due).
	eng.Open(ctx, Escalation{TenantID: tenant, UserID: "u1", Kind: "notification", RefID: "ug", Urgency: "urgent"})
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelInApp {
		t.Fatalf("urgent open: want in-app, got %v", fd.delivered)
	}
	// First tick (same clock) → IM fires immediately (no wait).
	fd.delivered = nil
	eng.Tick(ctx)
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelIM {
		t.Fatalf("urgent tick1: want IM immediately, got %v", fd.delivered)
	}
	// Next tick (same clock) → email fires immediately, then exhausted.
	fd.delivered = nil
	eng.Tick(ctx)
	if len(fd.delivered) != 1 || fd.delivered[0] != ChannelEmail {
		t.Fatalf("urgent tick2: want email immediately, got %v", fd.delivered)
	}
}
