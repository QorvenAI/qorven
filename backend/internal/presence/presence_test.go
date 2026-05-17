package presence_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/presence"
	"github.com/qorvenai/qorven/internal/testutil"
)

func TestPresenceStore_OnlineOffline(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := context.Background()

	// Insert a test user — username is UNIQUE NOT NULL, use a timestamp suffix for isolation
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, email, password_hash)
         VALUES ($1, $2, $3, $4) RETURNING id::text`,
		tenantID, "presence-test-user-"+suffix, "presence-test-"+suffix+"@example.com", "x",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	store := presence.NewStore(pool)

	if err := store.SetOnline(ctx, userID, tenantID, "web"); err != nil {
		t.Fatalf("SetOnline: %v", err)
	}

	p, err := store.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !p.IsOnline {
		t.Error("expected online=true")
	}
	if p.Channel != "web" {
		t.Errorf("expected channel=web, got %s", p.Channel)
	}

	if err := store.SetOffline(ctx, userID); err != nil {
		t.Fatalf("SetOffline: %v", err)
	}
	p, err = store.Get(ctx, userID)
	if err != nil {
		t.Fatalf("Get after offline: %v", err)
	}
	if p.IsOnline {
		t.Error("expected online=false after SetOffline")
	}
}

func TestPresenceStore_IsOnline(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := context.Background()

	store := presence.NewStore(pool)

	online, err := store.IsOnline(ctx, tenantID)
	if err != nil {
		t.Fatalf("IsOnline: %v", err)
	}
	if online {
		t.Error("expected no one online initially")
	}

	suffix2 := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, email, password_hash)
         VALUES ($1, $2, $3, $4) RETURNING id::text`,
		tenantID, "presence-online-user-"+suffix2, "presence-online-"+suffix2+"@example.com", "x",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := store.SetOnline(ctx, userID, tenantID, "web"); err != nil {
		t.Fatalf("SetOnline: %v", err)
	}
	online, err = store.IsOnline(ctx, tenantID)
	if err != nil {
		t.Fatalf("IsOnline after set: %v", err)
	}
	if !online {
		t.Error("expected online after SetOnline")
	}
}
