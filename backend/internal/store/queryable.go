// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Queryable is the minimal surface every tenant-bound store needs.
// Both *pgxpool.Pool and pgx.Tx implement it, so stores can transparently
// switch between "use the raw pool" (single-tenant / background job)
// and "use the per-request tenant-scoped transaction" (multi-tenant
// HTTP handler) without touching query call sites.
type Queryable interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Compile-time assertions so a pgx upgrade that silently renames a
// method is caught at build time, not at query time.
var (
	_ Queryable = (*pgxpool.Pool)(nil)
	_ Queryable = (pgx.Tx)(nil)
)

// ctxKeyTx is the context key for a tenant-scoped transaction.
// Unexported + typed to prevent unrelated code from stuffing random
// values into it.
type ctxKeyTx struct{}

// ctxKeyTxHandle points at the mutable handle that wraps the per-
// request tx. Middleware installs both the tx value (for direct
// FromContext use) AND the handle (so handlers can release early).
type ctxKeyTxHandle struct{}

// WithTx returns a context that carries tx. Installed by the gateway's
// TenantScopeMiddleware once per request in multi-tenant mode; read by
// FromContext in every RLS-sensitive store.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, ctxKeyTx{}, tx)
}

// TxHandle is the middleware's mutable grip on the per-request tx.
// Handlers that need to release the DB connection before the HTTP
// response is fully written (streaming endpoints — SSE, chunked
// JSON, long file downloads) call ReleaseTxEarly to return the
// connection to the pool; subsequent RLS-bound DB calls within the
// same request will fall back to the raw pool (bypass=off → deny
// under RLS), so releases must happen AFTER all tenant-scoped reads
// are done and BEFORE the long stream body.
//
// Rationale (Phase 5, Gap #6): holding a Postgres connection for the
// entire lifetime of an SSE stream pins a pool slot. Under 20-slot
// MaxConns that's 20 concurrent streams total — fine for a single
// user, a cliff for a SaaS tenant with many clients.
type TxHandle struct {
	tx       pgx.Tx
	released bool
}

// WithTxHandle installs both the tx value (for FromContext) and the
// handle (for ReleaseTxEarly). Called by TenantScopeMiddleware.
func WithTxHandle(ctx context.Context, h *TxHandle) context.Context {
	if h == nil {
		return ctx
	}
	ctx = context.WithValue(ctx, ctxKeyTx{}, h.tx)
	ctx = context.WithValue(ctx, ctxKeyTxHandle{}, h)
	return ctx
}

// NewTxHandle wraps a tx so the middleware and handler can
// coordinate early release.
func NewTxHandle(tx pgx.Tx) *TxHandle { return &TxHandle{tx: tx} }

// Tx returns the wrapped tx. May be nil if already released.
func (h *TxHandle) Tx() pgx.Tx {
	if h == nil || h.released {
		return nil
	}
	return h.tx
}

// Released reports whether the handle has been released. Middleware
// checks this to decide commit vs no-op at request end.
func (h *TxHandle) Released() bool {
	if h == nil {
		return true
	}
	return h.released
}

// Release commits the tx and marks it released. Subsequent FromContext
// lookups still return the original tx (by ctx value), but those
// queries will fail because the tx is closed — the contract is that
// callers release ONLY when they're done with tenant-scoped DB work.
func (h *TxHandle) Release(ctx context.Context) error {
	if h == nil || h.released {
		return nil
	}
	h.released = true
	return h.tx.Commit(ctx)
}

// ReleaseTxEarly is the handler-facing helper. Retrieves the handle
// from ctx and commits the tx so the connection returns to the pool.
// Safe to call on any ctx — a no-op when no handle is present
// (single-tenant, tests, unauthenticated paths).
//
// Call sites: SSE stream heads, after the initial metadata read. See
// the tenant-scope-middleware godoc for the full contract.
func ReleaseTxEarly(ctx context.Context) error {
	h, ok := ctx.Value(ctxKeyTxHandle{}).(*TxHandle)
	if !ok || h == nil {
		return nil
	}
	return h.Release(ctx)
}

// TxFromContext retrieves the tenant-scoped transaction stashed on
// ctx by WithTx. Returns nil + false when no tx is present — the
// caller should fall back to its pool in that case (single-tenant
// mode, background jobs, tests without the middleware).
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(ctxKeyTx{}).(pgx.Tx)
	return tx, ok
}

// FromContext returns the ctx-scoped Queryable if one is installed,
// else the provided fallback (usually a *pgxpool.Pool field on the
// store). Use exactly one line at the top of every store method:
//
//	q := store.FromContext(ctx, s.pool)
//	err := q.QueryRow(ctx, "SELECT ...", id).Scan(...)
//
// This way a single-tenant install runs the same code path as before
// (fallback == pool, bypass=on) while a multi-tenant install
// transparently routes through the request-scoped tx.
func FromContext(ctx context.Context, fallback Queryable) Queryable {
	if tx, ok := TxFromContext(ctx); ok {
		return tx
	}
	return fallback
}
