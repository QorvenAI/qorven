// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/plans"
)

// authorize is the single-source-of-truth authorization check for every
// session-bound or plan-bound operation in the gateway. All Phase 2+
// handlers (commands API, plan HTTP endpoints, permission reply, list
// pending) consume it via the thin adapters below.
//
// Two rule sets, selected by deployment mode:
//
//   ────────────── Single-tenant (default) ──────────────
//   Preserves Phase 2 behavior byte-for-byte.
//   - Local dev mode (no auth token AND no user on context): permit.
//   - Auth configured but no user on context: deny "no_actor".
//   - Actor role == "admin": permit (all sessions, any tenant).
//   - Actor is an active service account: permit.
//   - Session-bound resource:
//       - Session not found: permit; downstream raises the real error.
//       - owner_actor_id matches actor id: permit.
//       - owner_actor_id empty (legacy row): deny "legacy_session_no_owner".
//       - Otherwise: deny "not_owner".
//   - Session-less resource: deny "no_session_admin_only".
//
//   ────────────── Multi-tenant (opt-in via deployment_mode) ──────────────
//   Strict rules; a cross-tenant admin/user no longer sees another
//   tenant's session.
//   - No actor on context: deny "no_actor" REGARDLESS of Auth.Token
//     (dev fallback is retired in multi-tenant).
//   - Service account: permit (service accounts are global by design,
//     same as Phase 2).
//   - Admin of DIFFERENT tenant than the session: deny
//     "cross_tenant_admin_denied". Admins retain intra-tenant bypass.
//   - Session not found: deny "session_not_found" (we no longer pass
//     through; downstream error is insufficient in multi-tenant).
//   - Legacy-no-owner session: deny "legacy_session_no_owner" (even
//     admins — these rows must be backfilled before multi-tenant flips).
//   - Owner match + same tenant: permit.
//   - Owner match + different tenant: deny "cross_tenant_owner_mismatch"
//     (impossible in correctly-seeded data, but defended against).
//   - Session-less resource + non-admin: deny "no_session_admin_only".
//   - Session-less resource + cross-tenant admin: permitted today
//     because plan-less resources don't carry tenant; tracked for
//     Phase 4 when plans gain strict tenant checks.
//
// A nil return means "permitted"; a non-nil *apicommands.AuthzError
// means "forbidden" with a structured code. Non-AuthzError errors are
// internal failures — callers translate to 500.
func (gw *Gateway) authorize(ctx context.Context, scope AuthScope) error {
	multi := gw.deploymentConfig != nil && gw.deploymentConfig.IsMultiTenant(ctx)

	u := userFromContext(ctx)
	if u == nil {
		// Multi-tenant retires the dev-mode fallback: no anonymous access.
		if multi {
			return &apicommands.AuthzError{
				Reason: "unauthenticated actor cannot access resource",
				Code:   "no_actor",
			}
		}
		// Single-tenant: dev-mode fallback stays.
		if gw.cfg != nil && gw.cfg.Auth.Token == "" {
			return nil
		}
		return &apicommands.AuthzError{
			Reason: "unauthenticated actor cannot access resource",
			Code:   "no_actor",
		}
	}

	// Service account bypass.
	//
	// SAs are now tenant-aware.
	//   • Global SA (tenant_id IS NULL, the legacy infra actors
	//     system/orchestrator/qoros): blanket pass in both modes.
	//   • Per-tenant SA (tenant_id set): only bypasses for resources
	//     in the same tenant. Cross-tenant access is denied even if the
	//     SA role is "admin" — the whole point of binding the SA to a
	//     tenant is that it MUST NOT cross the boundary.
	//
	// We resolve the SA details (tenant binding) here, then compare
	// against resource tenancy on the session-bound path below. For the
	// session-less path we rely on the same u.TenantID guard that admin
	// users hit.
	var saTenant string
	var isSA bool
	if gw.serviceAccounts != nil {
		if _, t, ok := gw.serviceAccounts.LookupDetailed(ctx, u.ID); ok {
			isSA = true
			saTenant = t
		}
	}
	if isSA && saTenant == "" {
		// Global (legacy) SA — blanket pass, single- and multi-tenant.
		return nil
	}
	if isSA && !multi {
		// Tenant-bound SAs only enforce in multi-tenant mode; in a
		// single-tenant deployment the binding is informational.
		return nil
	}
	// A tenant-bound SA in multi-tenant mode falls through to the
	// session-lookup path, where we compare saTenant against the
	// resource's tenant_id before granting bypass.

	// Session-bound path.
	if scope.SessionID != "" {
		if gw.sessions == nil {
			// No session store configured (embedded test) — permit.
			// This branch never fires in production because
			// ensureProtocolSurfaces installs the store.
			return nil
		}
		sess, err := gw.sessions.GetByID(ctx, scope.SessionID)
		if err != nil {
			if multi {
				// Multi-tenant: do NOT pass through. The downstream
				// handler could leak existence via a different error
				// shape, and we care about discoverability here.
				return &apicommands.AuthzError{
					Reason: fmt.Sprintf("session %s not found", scope.SessionID),
					Code:   "session_not_found",
				}
			}
			// Single-tenant: defer to downstream (no information leak
			// in a single-tenant install).
			slog.Debug("authorize: session not found; permitting (single_tenant)",
				"session_id", scope.SessionID, "err", err)
			return nil
		}

		// Legacy-no-owner rows are equally dangerous in either mode;
		// multi-tenant is even stricter because there's no admin bypass.
		if sess.OwnerActorID == "" {
			if multi {
				return &apicommands.AuthzError{
					Reason: fmt.Sprintf(
						"session %s has no recorded owner (legacy row); "+
							"backfill required before multi-tenant use",
						scope.SessionID),
					Code: "legacy_session_no_owner",
				}
			}
			// Single-tenant admin bypass for legacy rows.
			if u.Role == "admin" {
				return nil
			}
			return &apicommands.AuthzError{
				Reason: fmt.Sprintf(
					"session %s has no recorded owner (legacy row); only admins and service accounts may access",
					scope.SessionID),
				Code: "legacy_session_no_owner",
			}
		}

		// Tenant-bound service account bypass. Honors the SA's declared
		// tenant_id: same tenant as the session → permit; different
		// tenant → deny with cross_tenant_sa_denied. The SA's `role`
		// field does not widen this — an admin-role SA bound to tenant A
		// still cannot reach tenant B.
		if isSA && multi {
			if saTenant != "" && sess.TenantID != "" && saTenant == sess.TenantID {
				return nil
			}
			return &apicommands.AuthzError{
				Reason: fmt.Sprintf(
					"service account %s (tenant %s) may not access session %s (tenant %s)",
					u.ID, saTenant, scope.SessionID, sess.TenantID),
				Code: "cross_tenant_sa_denied",
			}
		}

		// Owner match — gated by tenant in multi-tenant mode.
		if sess.OwnerActorID == u.ID {
			if multi && u.TenantID != "" && sess.TenantID != "" && u.TenantID != sess.TenantID {
				return &apicommands.AuthzError{
					Reason: fmt.Sprintf(
						"owner actor %s belongs to tenant %s but session %s is in tenant %s",
						u.ID, u.TenantID, scope.SessionID, sess.TenantID),
					Code: "cross_tenant_owner_mismatch",
				}
			}
			return nil
		}

		// Admin path — full access in single-tenant, tenant-bounded in
		// multi-tenant.
		if u.Role == "admin" {
			if !multi {
				return nil
			}
			if u.TenantID != "" && sess.TenantID != "" && u.TenantID == sess.TenantID {
				return nil
			}
			return &apicommands.AuthzError{
				Reason: fmt.Sprintf(
					"admin %s (tenant %s) may not access session %s (tenant %s) in multi-tenant mode",
					u.ID, u.TenantID, scope.SessionID, sess.TenantID),
				Code: "cross_tenant_admin_denied",
			}
		}

		// Non-owner non-admin — always denied.
		return &apicommands.AuthzError{
			Reason: fmt.Sprintf("actor %s is not the owner of session %s", u.ID, scope.SessionID),
			Code:   "not_owner",
		}
	}

	// Session-less resource: admin / service-account only.
	//
	// A tenant-bound SA reaching here (no SessionID) cannot be matched
	// against a concrete resource tenant, so in multi-tenant mode we
	// deny rather than blanket-pass. The caller must supply scope so
	// tenant comparison is possible. Global (NULL-tenant) SAs were
	// already returned above.
	if isSA && multi {
		return &apicommands.AuthzError{
			Reason: fmt.Sprintf(
				"service account %s (tenant %s) cannot authorize session-less resource without scope",
				u.ID, saTenant),
			Code: "sa_tenant_scope_required",
		}
	}
	if u.Role == "admin" {
		// Admins of any tenant pass through today. Phase 4 gains
		// plan-level tenant_id checks that will tighten this.
		return nil
	}
	return &apicommands.AuthzError{
		Reason: "resource has no session id; only admins and service accounts may access",
		Code:   "no_session_admin_only",
	}
}

// AuthScope describes the resource being authorized. Callers populate
// SessionID when the resource is session-bound; leave empty when it's
// a global / project-level resource that only admins/service accounts
// should reach.
//
// Fields may expand in Phase 4 (tenant scoping, project ownership).
// Keep nil-friendly so callers don't break when we extend.
type AuthScope struct {
	SessionID string
	// PlanID is optional; Authorize resolves the plan to a session and
	// scopes by that. Callers that already have a *plans.Plan should
	// pass its SessionID directly — this is just a convenience for the
	// HTTP handlers that look up by plan id.
	PlanID string
}

// authorizeForPlan resolves a *plans.Plan into a SessionID and calls
// authorize. Used by the plan HTTP handlers. Returns a typed error so
// callers translate uniformly to 403 / 500.
func (gw *Gateway) authorizeForPlan(ctx context.Context, p *plans.Plan) error {
	if p == nil {
		return errors.New("authorize: nil plan")
	}
	return gw.authorize(ctx, AuthScope{SessionID: p.SessionID, PlanID: p.ID})
}

// authorizeSessionID is the command-API adapter: *exactly* the shape
// apicommands.Server.OwnerCheck expects.
func (gw *Gateway) authorizeSessionID(ctx context.Context, sessionID string) error {
	return gw.authorize(ctx, AuthScope{SessionID: sessionID})
}

// isAuthzError extracts an *apicommands.AuthzError from err, if any.
// Used by HTTP handlers to distinguish 403 from 500.
func isAuthzError(err error) (*apicommands.AuthzError, bool) {
	if err == nil {
		return nil, false
	}
	var e *apicommands.AuthzError
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Silence the unused-package lint when auth types change shape.
var _ = auth.User{}
