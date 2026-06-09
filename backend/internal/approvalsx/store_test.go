package approvalsx

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func axTestPool(t *testing.T) *pgxpool.Pool {
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

func TestStore_OpenListDecide(t *testing.T) {
	pool := axTestPool(t)
	ctx := context.Background()
	s := NewStore(pool)
	tenant := "00000000-0000-0000-0000-0000000000f1"
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM approvals_unified WHERE tenant_id=$1", tenant) })

	amt := int64(40_000_000) // $40 in µUSD
	id, err := s.Open(ctx, Approval{
		TenantID: tenant, Kind: "work_item", RequesterAgentID: "cto",
		Summary: "Deploy onboarding v2", AmountUUSD: &amt, Risk: "normal",
		Context: map[string]any{"work_item_id": "wi-1"},
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	pend, err := s.ListPending(ctx, tenant)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, a := range pend {
		if a.ID == id {
			found = true
			if a.Risk != "normal" || a.AmountUUSD == nil || *a.AmountUUSD != amt {
				t.Errorf("unexpected approval: %+v", a)
			}
		}
	}
	if !found {
		t.Fatalf("opened approval %s not in pending list", id)
	}

	if err := s.Decide(ctx, id, true, "user", "ok ship it"); err != nil {
		t.Fatalf("decide: %v", err)
	}
	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "approved" || got.DecidedBy != "user" {
		t.Errorf("after decide want approved/user, got %+v", got)
	}
	if err := s.Decide(ctx, id, false, "user", "changed mind"); err != nil {
		t.Fatalf("re-decide should not error: %v", err)
	}
	got2, _ := s.Get(ctx, id)
	if got2.Status != "approved" {
		t.Errorf("re-decide must not flip an already-decided approval, got %s", got2.Status)
	}
}
