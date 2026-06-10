// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

func TestSyncCompanyHub(t *testing.T) {
	pool, tenant := testutil.NewIsolatedTenant(t)
	ctx := context.Background()
	gw := &Gateway{db: &store.DB{Pool: pool}}

	// Name the tenant.
	if _, err := pool.Exec(ctx, `UPDATE tenants SET name='Acme Inc' WHERE id=$1`, tenant); err != nil {
		t.Fatalf("name tenant: %v", err)
	}
	// Two C-level agents (l1, l2) + one worker (l3).
	mkAgent := func(key, level string) string {
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO agents (tenant_id, agent_key, display_name, org_level, status)
			 VALUES ($1,$2,$2,$3,'active') RETURNING id`, tenant, key, level).Scan(&id); err != nil {
			t.Fatalf("create agent %s: %v", key, err)
		}
		return id
	}
	l1 := mkAgent("ceo", "l1")
	l2 := mkAgent("cto", "l2")
	l3 := mkAgent("dev", "l3")

	// Create a bare company-hub room for the tenant (no members, generic name).
	var roomID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO rooms (tenant_id, name, display_name, description, created_by)
		 VALUES ($1,'company-hub','Company Hub','x','system') RETURNING id`, tenant).Scan(&roomID); err != nil {
		t.Fatalf("create room: %v", err)
	}

	// Sync twice — must be idempotent.
	gw.syncCompanyHub(ctx, tenant, roomID)
	gw.syncCompanyHub(ctx, tenant, roomID)

	// display_name should now be the tenant name.
	var dn string
	pool.QueryRow(ctx, `SELECT display_name FROM rooms WHERE id=$1`, roomID).Scan(&dn)
	if dn != "Acme Inc" {
		t.Errorf("display_name: want 'Acme Inc', got %q", dn)
	}

	// l1 + l2 are members; l3 is not; no duplicates.
	members := map[string]int{}
	rows, err := pool.Query(ctx, `SELECT agent_id::text, count(*) FROM room_members WHERE room_id=$1 GROUP BY agent_id`, roomID)
	if err != nil {
		t.Fatalf("query members: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var aid string
		var n int
		if err := rows.Scan(&aid, &n); err != nil {
			t.Fatalf("scan member: %v", err)
		}
		members[aid] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	if members[l1] != 1 {
		t.Errorf("l1 should be a single member, got %d", members[l1])
	}
	if members[l2] != 1 {
		t.Errorf("l2 should be a single member, got %d", members[l2])
	}
	if _, ok := members[l3]; ok {
		t.Errorf("l3 worker should NOT be a member")
	}
}
