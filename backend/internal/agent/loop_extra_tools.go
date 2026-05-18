// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/tools"
)

// executeTool is the Phase 5.3 dispatch seam. Every Loop-side tool
// execution MUST go through this function instead of calling
// l.toolReg.Execute directly.
//
// Why: the Loop now supports per-request ExtraTools (tenant-scoped
// Wasm plugins injected by the orchestrator). If ExtraTools contains
// a tool with the same name as a global registry entry, the extra
// WINS — matching the shadow semantics in BuildToolDefs. A global
// call site that skipped this shim would silently run the wrong
// tool on a name collision.
//
// Pass the request verbatim; the shim reads req.ExtraTools and
// req.TenantID (the latter is used for metric attribution on
// plugin tools).
func (l *Loop) executeTool(ctx context.Context, req RunRequest, name string, args map[string]any) *tools.Result {
	// stamp the tenant on ctx so permissions.WrapLazy can
	// populate permission_requests.tenant_id correctly. Without this,
	// permission_requests rows fall back to the 'default' literal
	// value and multi-tenant RLS rejects them with a cast error.
	if req.TenantID != "" {
		ctx = tools.WithTenantID(ctx, req.TenantID)
	}
	// Stamp a valid UUID user ID so permissions.WrapLazy can find auto-approved policies.
	// Channel messages (Telegram, WhatsApp) carry a non-UUID sender ID; fall back to
	// the tenant admin user so the auto-approved policy lookup succeeds.
	effectiveUserID := req.UserID
	if _, err := uuid.Parse(effectiveUserID); err != nil {
		effectiveUserID = l.resolveTenantAdminUserID(ctx)
	}
	if effectiveUserID != "" {
		ctx = tools.WithUserID(ctx, effectiveUserID)
	}
	// Fast path: no extras, go straight to the global registry.
	if len(req.ExtraTools) == 0 {
		return l.toolReg.Execute(ctx, name, args)
	}
	// Scan extras first — a matching name shadows the global.
	for _, t := range req.ExtraTools {
		if t != nil && t.Name() == name {
			return t.Execute(ctx, args)
		}
	}
	// Name not in extras — fall through.
	return l.toolReg.Execute(ctx, name, args)
}
