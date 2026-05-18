// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/qorvenai/qorven/internal/store"
)

// TenantScopeMiddleware opens a per-request Postgres transaction
// scoped to the authenticated user's tenant (SET LOCAL
// app.current_tenant_id = <uuid>) in multi-tenant mode. The tx is
// stashed on the request context via store.WithTx so every
// RLS-sensitive store method transparently picks it up through
// store.FromContext.
//
// Placement: installed AFTER AuthMiddlewareV2 on the /v1 router. That
// ordering is critical — we need a resolved *auth.User in context to
// know the tenant. Without it (e.g. endpoints that handle auth itself,
// or single-tenant mode), we let the request through untouched and
// store.FromContext falls back to the raw pool.
//
// Transaction lifecycle:
//   • commit if the handler chain completes without panic AND the
//     response status is < 400.
//   • rollback if the handler panics, writes a >=500 response, OR
//     returns with ctx cancelled.
//   • The middleware swallows the commit/rollback error into a log
//     line; once the handler has written to the ResponseWriter we
//     can't change the HTTP reply anyway.
//
// Boundary events: every request boundary involves a DB round-trip
// for BEGIN/COMMIT. The cost is real (~0.5ms roundtrip on a local
// socket) and the benefit — unfakeable tenant isolation — is worth
// it for multi-tenant deployments. Single-tenant installs never
// pay this cost because this middleware is a no-op there.
func (gw *Gateway) TenantScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Single-tenant: no-op. The pool is already bypass=on.
		if gw.deploymentConfig == nil || !gw.deploymentConfig.IsMultiTenant(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}

		// Unauthenticated requests fall through — AuthMiddlewareV2 will
		// have rejected them already if authentication was required.
		// For the setup / health paths where auth is skipped entirely,
		// there is no tenant to scope to, so we don't scope.
		user := userFromContext(r.Context())
		if user == nil || user.TenantID == "" {
			next.ServeHTTP(w, r)
			return
		}

		if gw.db == nil || gw.db.Pool == nil {
			// Misconfiguration — gateway in multi-tenant mode without
			// a DB. Surface as 500, log loudly. No data could be served
			// safely in this state regardless.
			slog.Error("TenantScopeMiddleware: no DB pool in multi-tenant mode")
			writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "gateway misconfigured: no database"})
			return
		}

		// Begin the per-request tx. We intentionally do NOT derive a
		// new ctx for the tx — using r.Context() means client
		// disconnection cancels the tx automatically.
		tx, err := gw.db.Pool.BeginTx(r.Context(), pgx.TxOptions{})
		if err != nil {
			slog.Error("TenantScopeMiddleware: BeginTx", "err", err)
			writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "database unavailable"})
			return
		}

		// Scope to tenant via SET LOCAL. We reuse the same UUID shape
		// check as store.WithTenantTx to refuse malformed values
		// before they reach SQL.
		if err := scopeTxToTenant(r.Context(), tx, user.TenantID); err != nil {
			_ = tx.Rollback(r.Context())
			slog.Error("TenantScopeMiddleware: scope", "err", err, "tenant_id", user.TenantID)
			writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "tenant scoping failed"})
			return
		}

		// Wrap the tx in a handle so streaming handlers can release it
		// early (Phase 5, Gap #6) and so the panic-hardened rollback
		// below can be idempotent.
		handle := store.NewTxHandle(tx)
		poison := &poisonFlag{}
		ctx := store.WithTxHandle(r.Context(), handle)
		ctx = context.WithValue(ctx, ctxKeyPoison{}, poison)
		sw := &statusCaptureWriter{ResponseWriter: w, status: http.StatusOK}

		// rollback path handles three failure modes:
		//   1. Handler panic → recovered, tx rolled back, panic re-raised.
		//   2. Handler writes >=500 or poisons the tx → tx rolled back.
		//   3. Normal success → tx committed.
		// The handle prevents double-commit-or-rollback if the handler
		// called ReleaseTxEarly for a streaming response.
		defer func() {
			if rec := recover(); rec != nil {
				if !handle.Released() {
					_ = tx.Rollback(context.Background())
				}
				panic(rec) // re-raise so chi recoverer logs the stack
			}
		}()

		next.ServeHTTP(sw, r.WithContext(ctx))

		// If the handler already released (streaming path), nothing to
		// do — the tx is committed and its connection is back in the pool.
		if handle.Released() {
			return
		}

		if sw.status >= 500 || r.Context().Err() != nil || txIsPoisoned(ctx) {
			if err := tx.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				slog.Warn("TenantScopeMiddleware: rollback", "err", err)
			}
			return
		}
		if err := tx.Commit(context.Background()); err != nil {
			// Can't change the HTTP status now (response already
			// written); surface via structured log so ops see it.
			slog.Error("TenantScopeMiddleware: commit failed",
				"err", err, "tenant_id", user.TenantID, "path", r.URL.Path)
		}
	})
}

// scopeTxToTenant mirrors store.WithTenantTx's inner scope step. We
// don't call WithTenantTx itself because it expects to own the tx
// lifecycle; here the middleware owns it.
func scopeTxToTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	if !looksLikeTenantIDUUID(tenantID) {
		return errors.New("tenant id must be UUID-shaped")
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.current_tenant_id = '"+tenantID+"'"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SET LOCAL app.tenant_id = '"+tenantID+"'"); err != nil {
		return err
	}
	return nil
}

// looksLikeTenantIDUUID duplicates the same check as store.looksLikeTenantID.
// We don't re-export the store helper to avoid a circular dependency —
// this is cheap enough to keep local.
func looksLikeTenantIDUUID(s string) bool {
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

// ctxKeyPoison marks a request-tx as "must rollback regardless of
// HTTP status." Handlers that detect a partial/inconsistent write
// after they've already flushed the header (status 200 written, but
// something went wrong downstream) call PoisonTx to force rollback.
//
// Use sparingly — the common case (400+ status, panic, ctx cancel) is
// already handled by the middleware. Poison is the escape hatch for
// "I can't change my status code anymore but this tx absolutely must
// not commit."
type ctxKeyPoison struct{}

type poisonFlag struct {
	poisoned bool
}

// PoisonTx marks the current request's tenant tx as must-rollback.
// No-op when no tenant tx is in scope (single-tenant, tests).
// Safe to call multiple times; idempotent.
func PoisonTx(ctx context.Context) {
	if p, ok := ctx.Value(ctxKeyPoison{}).(*poisonFlag); ok && p != nil {
		p.poisoned = true
	}
}

// txIsPoisoned reports whether PoisonTx was called on ctx.
func txIsPoisoned(ctx context.Context) bool {
	p, ok := ctx.Value(ctxKeyPoison{}).(*poisonFlag)
	return ok && p != nil && p.poisoned
}

// statusCaptureWriter records the first WriteHeader-supplied status so
// the middleware can decide commit vs rollback after the handler chain
// finishes. If no status is set, it defaults to 200 (mirrors
// net/http behavior).
type statusCaptureWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusCaptureWriter) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusCaptureWriter) Write(b []byte) (int, error) {
	if !s.written {
		s.status = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}
