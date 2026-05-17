// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store_test

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestAssertNotSuperuser_RejectsSuperuser confirms the guard's positive
// case: the default dev role (qorven) is a superuser on the test box,
// and AssertNotSuperuser must refuse it. This is a production-safety
// tripwire — a regression that silently permits superuser in
// multi-tenant would disable RLS without any visible failure.
func TestAssertNotSuperuser_RejectsSuperuser(t *testing.T) {
	db, err := store.New(testutil.TestDSN)
	if err != nil {
		// No DB reachable with the configured DSN — this test needs a
		// live Postgres to meaningfully check the guard. Skip cleanly
		// so `go test -short ./...` stays green in environments that
		// haven't spun up the compose Postgres yet.
		t.Skipf("DB not reachable (%v) — set QORVEN_TEST_DSN to run", err)
	}
	defer db.Close()

	err = db.AssertNotSuperuser(context.Background())
	if err == nil {
		t.Fatalf("AssertNotSuperuser returned nil under the superuser dev role — "+
			"the boot guard is broken. Expected error mentioning rolsuper=true.")
	}
	// Message shape check — operators will search for these words
	// when grepping logs.
	if !strings.Contains(err.Error(), "rolsuper") || !strings.Contains(err.Error(), "NOSUPERUSER") {
		t.Fatalf("guard error must mention rolsuper + NOSUPERUSER: got %q", err)
	}
}

// TestAssertNotSuperuser_AcceptsRestrictedRole confirms the negative
// case: a restricted role (qorven_app) passes the guard. Without this,
// a bug that rejects ALL roles would still make
// TestAssertNotSuperuser_RejectsSuperuser look green.
func TestAssertNotSuperuser_AcceptsRestrictedRole(t *testing.T) {
	db, err := store.NewForMultiTenant(rlsTestDSN(t))
	if err != nil {
		t.Fatalf("NewForMultiTenant: %v", err)
	}
	defer db.Close()

	if err := db.AssertNotSuperuser(context.Background()); err != nil {
		t.Fatalf("AssertNotSuperuser must accept qorven_app: %v", err)
	}
}
