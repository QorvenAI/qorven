package memory

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/testsupport"
)

func briefTestPool(t *testing.T) *pgxpool.Pool {
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

func TestBriefStore_UpsertAndClearanceFilter(t *testing.T) {
	pool := briefTestPool(t)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-0000000000aa"
	s := NewBriefStore(pool, tenant)
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM knowledge_briefs WHERE tenant_id=$1", tenant)
	})

	// A company brief at internal clearance and a role brief at restricted.
	if err := s.Upsert(ctx, Brief{Scope: "company", ScopeKey: "", Clearance: ClassInternal, Content: "company facts"}); err != nil {
		t.Fatalf("upsert company: %v", err)
	}
	if err := s.Upsert(ctx, Brief{Scope: "role", ScopeKey: "cko", Clearance: ClassRestricted, Content: "secret"}); err != nil {
		t.Fatalf("upsert role: %v", err)
	}

	// An internal-clearance agent (role "code") sees the company brief, NOT the restricted one.
	got := s.GetForAgent(ctx, "code", "", ClassInternal)
	if len(got) != 1 || got[0].Content != "company facts" {
		t.Fatalf("internal agent: expected only company brief, got %+v", got)
	}

	// A restricted-clearance CKO sees both.
	got2 := s.GetForAgent(ctx, "cko", "", ClassRestricted)
	if len(got2) != 2 {
		t.Fatalf("restricted agent: expected 2 briefs, got %d", len(got2))
	}
}
