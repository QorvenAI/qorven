// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
	// RLSMode records whether this pool was opened with RLS bypass on
	// (single-tenant / migrator) or off (multi-tenant). Purely
	// informational for /v1/debug/pool-stats and for tests that need
	// to assert the bootstrap chose the right mode.
	RLSMode string // "bypass" | "enforce"
}

// New opens a pool with RLS bypass ON. This is the single-tenant
// default and the right choice for every CLI that runs migrations or
// admin tooling — those paths cannot be tenant-scoped because the
// scope doesn't exist yet. Multi-tenant gateways MUST use
// NewForMultiTenant so each request transaction sets app.current_tenant_id
// via SET LOCAL and the RLS policies actually fire.
func New(dsn string) (*DB, error) {
	return openPool(dsn, "bypass")
}

// NewForMultiTenant opens a pool with RLS bypass OFF. Queries on
// connections from this pool MUST run inside a transaction that first
// runs `SET LOCAL app.current_tenant_id = $1` — otherwise the RLS
// policies deny every row and queries return empty / fail.
//
// Phase 4 Day 2: the gateway's bootstrap calls this when
// deployment.IsMultiTenant() is true. Single-tenant installs keep
// calling New and see no behavior change.
func NewForMultiTenant(dsn string) (*DB, error) {
	return openPool(dsn, "enforce")
}

func openPool(dsn, rlsMode string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	// AfterConnect installs the pool-wide RLS default on every new
	// connection. For "bypass", every query from this connection
	// succeeds regardless of tenant scope (single-tenant). For
	// "enforce", queries deny unless wrapped in a transaction that
	// sets app.current_tenant_id via SET LOCAL.
	//
	// We also initialise the legacy app.tenant_id and app.current_tenant_id
	// GUCs to empty string. Without this, on a completely fresh install
	// the old RLS policies from migrations 001/003 call
	// current_setting('app.tenant_id') (without missing_ok) and PostgreSQL
	// raises "unrecognized configuration parameter" because the GUC has
	// never been touched on this server. Setting it to '' once is enough —
	// after that, current_setting returns '' rather than erroring.
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		var bypassStmt string
		switch rlsMode {
		case "bypass":
			bypassStmt = "SET app.bypass_rls = 'on'"
		case "enforce":
			bypassStmt = "SET app.bypass_rls = 'off'"
		default:
			return fmt.Errorf("openPool: unknown rlsMode %q", rlsMode)
		}
		// Initialise all app.* GUCs used by RLS policies in one batch.
		// The tenant ID fields are set to empty string so that
		// current_setting('app.tenant_id') never raises "unrecognized
		// configuration parameter" on a fresh install. They will be
		// overridden per-connection (bypass_rls) or per-transaction
		// (tenant IDs) by the application code.
		initSQL := bypassStmt + "; " +
			"SET app.tenant_id = ''; " +
			"SET app.current_tenant_id = ''"
		if _, err := c.Exec(ctx, initSQL); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Best-effort: RLS GUCs may not be registered yet if
			// migrations 001/040 haven't run. Fall back to just the
			// bypass flag so the migration runner itself can proceed.
			if _, err2 := c.Exec(ctx, bypassStmt); err2 != nil && ctx.Err() == nil {
				slog.Warn("AfterConnect: SET app.bypass_rls failed; RLS may not be installed yet",
					"mode", rlsMode, "err", err2)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	slog.Info("database connected", "pool_max", cfg.MaxConns, "rls_mode", rlsMode)
	return &DB{Pool: pool, RLSMode: rlsMode}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

// SetTenant sets the RLS tenant context for a connection.
// Deprecated: pool-wide tenant context is unsafe — the setting persists
// on a connection and leaks to the next acquirer. Use WithTenantTx for
// per-request scoping.
func (db *DB) SetTenant(ctx context.Context, tenantID string) error {
	_, err := db.Pool.Exec(ctx, "SELECT set_config('app.tenant_id', $1, false)", tenantID)
	return err
}

// WithTenantTx runs fn inside a transaction with app.current_tenant_id
// scoped to tenantID. The SET LOCAL statement is discarded on commit/
// rollback, so the connection returned to the pool carries no leaked
// tenant state. This is the ONLY supported way to execute tenant-bound
// queries against a pool opened with NewForMultiTenant.
//
// A call with tenantID == "" is a bug — we refuse to silently set an
// empty tenant (which would fail to match any row under RLS).
func (db *DB) WithTenantTx(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if tenantID == "" {
		return fmt.Errorf("WithTenantTx: tenantID required")
	}
	tx, err := db.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// SET LOCAL binds the GUC to this transaction only. On commit or
	// rollback the setting is discarded. Postgres does NOT accept
	// bind parameters on SET LOCAL, so we have to format the value
	// into the statement. Guard against injection by enforcing UUID
	// shape — any non-hex/hyphen byte is rejected before we format.
	if !looksLikeTenantID(tenantID) {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("WithTenantTx: tenantID must be a UUID-shaped string; got %q", tenantID)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_tenant_id = '%s'", tenantID)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("scope tenant: %w", err)
	}
	// Legacy policies from migrations 001/003/004 reference the
	// old-style `app.tenant_id` GUC. Keep them satisfied too so code
	// touching agents / memories / tasks inside a multi-tenant tx
	// doesn't trip policies we didn't rewrite in migration 040.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.tenant_id = '%s'", tenantID)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("scope legacy tenant: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// AssertNotSuperuser is the Phase 4 multi-tenant boot guard. Queries
// pg_roles for the connecting role's rolsuper / rolbypassrls. Returns
// a non-nil error if either flag is set — such a role defeats RLS
// entirely (Postgres bypasses every policy for superusers regardless
// of FORCE).
//
// The gateway calls this at boot in multi-tenant mode and refuses to
// start if it returns an error. Single-tenant mode skips the check
// (bypass=on by design). Do not "soften" this — a decorative RLS
// boundary is worse than no boundary because it creates false
// confidence.
func (db *DB) AssertNotSuperuser(ctx context.Context) error {
	var rolsuper, rolbypassrls bool
	var username string
	err := db.Pool.QueryRow(ctx, `
        SELECT CURRENT_USER::text, rolsuper, rolbypassrls
          FROM pg_roles WHERE rolname = CURRENT_USER
    `).Scan(&username, &rolsuper, &rolbypassrls)
	if err != nil {
		return fmt.Errorf("AssertNotSuperuser: probe role: %w", err)
	}
	if rolsuper || rolbypassrls {
		return fmt.Errorf(
			"AssertNotSuperuser: role %q has rolsuper=%v rolbypassrls=%v — "+
				"RLS policies are inert under this role. Create a restricted role "+
				"(CREATE ROLE ... NOSUPERUSER NOBYPASSRLS; GRANT ALL ON ALL TABLES ...) "+
				"and point QORVEN_POSTGRES_DSN at it before flipping to multi-tenant",
			username, rolsuper, rolbypassrls)
	}
	return nil
}

// looksLikeTenantID accepts only the shape of a UUID: 32 hex digits +
// 4 hyphens at the canonical positions. Used to sanitize the value
// before we format it into a SET LOCAL statement (which disallows
// bind parameters).
func looksLikeTenantID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// PoolStats returns database connection pool statistics.
func (db *DB) PoolStats() map[string]any {
	s := db.Pool.Stat()
	return map[string]any{
		"total_conns":     s.TotalConns(),
		"idle_conns":      s.IdleConns(),
		"acquired_conns":  s.AcquiredConns(),
		"max_conns":       s.MaxConns(),
		"constructing":    s.ConstructingConns(),
		"empty_acquire":   s.EmptyAcquireCount(),
		"canceled_acquire": s.CanceledAcquireCount(),
	}
}
