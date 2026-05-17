// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package testsupport provides shared helpers for tests that need a
// live Postgres connection. Centralising the DSN lookup behind a
// single function keeps the test suite runnable on any developer
// machine without patching 17 files when the DB password changes.
package testsupport

import (
	"os"
)

// DSN returns the Postgres DSN to use for integration tests. Lookup
// order (first hit wins):
//
//  1. QORVEN_TEST_DSN     — the canonical override; point at any
//                           database the test can nuke + recreate.
//  2. QORVEN_POSTGRES_DSN — reused from the gateway's prod env var so
//                           Docker Compose setups work without a
//                           separate test env.
//  3. A localhost default that assumes the developer ran
//                          `docker compose up postgres` with the
//                           password env var set. Won't match a
//                           default install because there is no
//                           default password in the compose file —
//                           tests will fail loudly, which is correct.
//
// Tests that need to SKIP when no DB is available should use
// DSNOrSkip(t) instead — they'll call t.Skip() cleanly when nothing
// is wired up.
func DSN() string {
	if v := os.Getenv("QORVEN_TEST_DSN"); v != "" {
		return v
	}
	if v := os.Getenv("QORVEN_POSTGRES_DSN"); v != "" {
		return v
	}
	return "postgres://qorven@localhost:5432/qorven_dev?sslmode=disable"
}

// DSNOrSkip returns the test DSN or calls t.Skip when the expected
// env var isn't set. Use this in integration tests that can't run
// without a live DB — keeping them under -short results in a green
// CI while letting `QORVEN_TEST_DSN=... go test ./...` exercise them.
func DSNOrSkip(t interface {
	Skip(args ...any)
	Helper()
}) string {
	t.Helper()
	dsn := os.Getenv("QORVEN_TEST_DSN")
	if dsn == "" {
		dsn = os.Getenv("QORVEN_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("QORVEN_TEST_DSN or QORVEN_POSTGRES_DSN not set — skipping DB-backed test")
	}
	return dsn
}
