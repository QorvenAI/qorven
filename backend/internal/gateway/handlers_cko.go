// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import "net/http"

// handleCKORefresh triggers an on-demand CKO brief refresh for a scope.
// Body: {"scope":"company|department|role","scope_key":""}. Admin-gated.
func (gw *Gateway) handleCKORefresh(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.ckoCurator == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cko curator not available"})
		return
	}
	var body struct {
		Scope    string `json:"scope"`
		ScopeKey string `json:"scope_key"`
	}
	_ = decodeJSONBody(r, &body, 1<<16)
	if body.Scope == "" {
		body.Scope = "company"
	}
	switch body.Scope {
	case "company", "department", "role":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid scope: must be company, department, or role"})
		return
	}
	if err := gw.ckoCurator.Refresh(r.Context(), body.Scope, body.ScopeKey); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "scope": body.Scope, "scope_key": body.ScopeKey})
}
