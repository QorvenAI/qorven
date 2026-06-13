// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"testing"
)

// The cron path must be able to resolve a non-empty UserID for a non-UUID agent id,
// and defaultTenant must be set — otherwise scheduled runs hit the permission gate
// with empty identity (the bug this fixes).

// TestCron_DefaultTenantIsSet confirms the package-level defaultTenant constant is
// non-empty. If it were empty, every cron RunRequest.TenantID would be empty and the
// permission gate would reject every scheduled run.
func TestCron_DefaultTenantIsSet(t *testing.T) {
	if defaultTenant == "" {
		t.Fatal("defaultTenant is empty; cron RunRequest.TenantID would be empty")
	}
}

// TestCron_ResolverPassesThroughUUID confirms that resolveTenantUserIDForChannel
// returns a valid UUID sender unchanged (no DB required). This is the cron fast-path
// when runAs is already a UUID-shaped agent id.
func TestCron_ResolverPassesThroughUUID(t *testing.T) {
	gw := &Gateway{} // no db — UUID passthrough requires none
	in := "550e8400-e29b-41d4-a716-446655440000"
	if got := gw.resolveTenantUserIDForChannel(context.Background(), defaultTenant, in); got != in {
		t.Fatalf("UUID passthrough failed: got %q, want %q", got, in)
	}
}
