// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Reserved for NewIsolatedTenant — ensures the symbol is referenced.
var _ = context.Background

// testutil provides shared test helpers, DB setup, and mock providers.

var TestDSN = envOr("QORVEN_TEST_DSN", "postgres://qorven:qorven@localhost:5432/qorven_test?sslmode=disable")

const (
	TestTenantID = "00000000-0000-0000-0000-000000000001"
	TestAgentID  = "test-agent-001"
	TestToken    = ""
)

// Pool returns a shared pgxpool for tests. Skips if DB not available.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" { dsn = TestDSN }

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("DB not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("DB not reachable: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TempID generates a unique test ID.
func TempID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1000000)
}

// NewUUID generates a v4 UUID string for use in tests.
// It uses the same hand-rolled approach as NewIsolatedTenant to avoid
// pulling in an external dependency.
func NewUUID(t *testing.T) string {
	t.Helper()
	var buf [16]byte
	nano := time.Now().UnixNano()
	for i := range buf {
		buf[i] = byte(nano >> uint(i%8) & 0xFF)
	}
	atomicNonce++
	buf[12] ^= byte(atomicNonce)
	buf[13] ^= byte(atomicNonce >> 8)
	// Use time-based entropy in remaining bytes for more uniqueness.
	buf[14] ^= byte(nano >> 32)
	buf[15] ^= byte(nano >> 40)
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// NewIsolatedTenant creates a fresh tenant row for the duration of t
// and registers a t.Cleanup that deletes it (cascading every child
// row via FK). Use this instead of TestTenantID for any test that
// writes to tenant-scoped tables — it lets tests run in isolation and
// paves the way for -parallel.
//
// The returned id is a v4 UUID string, never the seeded TestTenantID.
// Callers that need to select "test-tenant rows only" can filter by
// the id. Cleanup is best-effort — a failing DELETE logs but does not
// fail the test (the tenant_id is unique enough that leaked rows are
// ignorable until the next migration).
//
// tests migrate to this pattern incrementally. Tests
// that still use TestTenantID continue to work; they just share state.
// New tests MUST use NewIsolatedTenant.
func NewIsolatedTenant(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	p := Pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build a v4 UUID by hand so we don't drag github.com/google/uuid
	// into testutil.
	var buf [16]byte
	nano := time.Now().UnixNano()
	for i := range buf {
		buf[i] = byte(nano >> uint(i%8) & 0xFF)
	}
	// Mix in a per-process counter so callers invoked within the same
	// nanosecond still get unique ids.
	atomicNonce++
	buf[12] ^= byte(atomicNonce)
	buf[13] ^= byte(atomicNonce >> 8)
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	tenantID := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
	slug := "it-" + TempID("t")

	if _, err := p.Exec(ctx,
		`INSERT INTO tenants (id, name, slug) VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		tenantID, slug, slug,
	); err != nil {
		t.Fatalf("NewIsolatedTenant: insert tenant: %v", err)
	}

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()

		// Phase 7 fix — belt-and-braces cleanup for tables that do NOT
		// cascade on tenants delete:
		//
		//   permission_requests.tenant_id is TEXT (historical), not a
		//   UUID FK, so DELETE FROM tenants cannot cascade through it.
		//   Rows accumulate across test runs, eventually making the
		//   permission-gate auto-approver miss its 10s window for the
		//   current test's request (the approver pool starts hitting
		//   MaxConns just scanning stale rows).
		//
		// Explicitly purge permission_requests first so the subsequent
		// DELETE FROM tenants is an uncluttered, fast operation.
		// Errors are logged-not-fatal — cleanup failures should never
		// fail a test that already passed.
		if _, err := p.Exec(cctx,
			`DELETE FROM permission_requests WHERE tenant_id = $1`, tenantID); err != nil {
			fmt.Fprintf(os.Stderr, "NewIsolatedTenant: purge permission_requests: %v\n", err)
		}
		// Same story for wakeup_requests (also TEXT tenant_id, no FK
		// cascade through to tenants).
		if _, err := p.Exec(cctx,
			`DELETE FROM wakeup_requests WHERE tenant_id = $1`, tenantID); err != nil {
			fmt.Fprintf(os.Stderr, "NewIsolatedTenant: purge wakeup_requests: %v\n", err)
		}

		if _, err := p.Exec(cctx, `DELETE FROM tenants WHERE id = $1`, tenantID); err != nil {
			fmt.Fprintf(os.Stderr, "NewIsolatedTenant cleanup: %v\n", err)
		}
	})
	return p, tenantID
}

// atomicNonce is a process-local counter that lets TempID +
// NewIsolatedTenant produce distinct outputs within the same
// nanosecond. Not thread-safe on its own; protected by the Go test
// runner's serial default. When -parallel tests land, this flips to
// a sync/atomic counter.
var atomicNonce uint32

// NewIsolatedUser inserts a test user into the given tenant and registers
// cleanup to delete it. Returns the user ID UUID string.
func NewIsolatedUser(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	uid := NewUUID(t)
	username := "test-user-" + TempID("u")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, tenant_id, username, password_hash, role, is_active)
		VALUES ($1::uuid, $2::uuid, $3, '', 'admin', true)
	`, uid, tenantID, username); err != nil {
		t.Fatalf("NewIsolatedUser: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, err := pool.Exec(cctx, `DELETE FROM users WHERE id = $1`, uid); err != nil {
			fmt.Fprintf(os.Stderr, "NewIsolatedUser cleanup: %v\n", err)
		}
	})
	return uid
}

// NewIsolatedAgent inserts a test agent into the given tenant and registers
// cleanup to delete it. Returns the agent ID UUID string.
func NewIsolatedAgent(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	aid := NewUUID(t)
	agentKey := "test-agent-" + TempID("a")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO agents (id, tenant_id, agent_key, model, system_prompt)
		VALUES ($1::uuid, $2::uuid, $3, 'default', '')
	`, aid, tenantID, agentKey); err != nil {
		t.Fatalf("NewIsolatedAgent: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, err := pool.Exec(cctx, `DELETE FROM agents WHERE id = $1`, aid); err != nil {
			fmt.Fprintf(os.Stderr, "NewIsolatedAgent cleanup: %v\n", err)
		}
	})
	return aid
}

// Ctx returns a context with 30s timeout for tests.
func Ctx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// OpenAIKey returns the OpenAI API key from config or env. Skips if not available.
func OpenAIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		// Try reading from config.toml
		data, err := os.ReadFile("../../config.toml")
		if err == nil {
			// Simple extraction — find api_key line
			for _, line := range splitLines(string(data)) {
				if len(line) > 10 && line[:7] == "api_key" {
					start := indexOf(line, '"') + 1
					end := lastIndexOf(line, '"')
					if start > 0 && end > start {
						key = line[start:end]
						break
					}
				}
			}
		}
	}
	if key == "" {
		t.Skip("OPENAI_API_KEY not available")
	}
	return key
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) { lines = append(lines, s[start:]) }
	return lines
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ { if s[i] == c { return i } }
	return -1
}

func lastIndexOf(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- { if s[i] == c { return i } }
	return -1
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}
