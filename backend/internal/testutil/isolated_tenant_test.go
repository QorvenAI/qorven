// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package testutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/testutil"
)

func TestNewIsolatedTenant_DistinctIDs(t *testing.T) {
	_, a := testutil.NewIsolatedTenant(t)
	_, b := testutil.NewIsolatedTenant(t)
	if a == b {
		t.Fatalf("two calls returned the same tenant id: %s", a)
	}
	if a == testutil.TestTenantID || b == testutil.TestTenantID {
		t.Fatalf("isolated tenant must not alias TestTenantID")
	}
}

func TestNewIsolatedTenant_CleanupDeletes(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Row exists while test holds the handle.
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE id = $1`, tenantID).Scan(&count); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}

	// The cleanup runs at test exit — we can't observe it from inside
	// this test. Instead drive a sub-test and confirm its cleanup
	// ran by querying from the outer test.
	var leakedID string
	t.Run("subtest-with-cleanup", func(t *testing.T) {
		_, sub := testutil.NewIsolatedTenant(t)
		leakedID = sub
	})
	// After the sub-test returns, its t.Cleanup must have fired.
	var subCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE id = $1`, leakedID).Scan(&subCount); err != nil {
		t.Fatalf("post-subtest lookup: %v", err)
	}
	if subCount != 0 {
		t.Fatalf("sub-test tenant %s leaked (count=%d)", leakedID, subCount)
	}
}
