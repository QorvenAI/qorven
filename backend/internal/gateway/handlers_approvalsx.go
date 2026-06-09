// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handleListFabricApprovals lists pending unified approvals.
func (gw *Gateway) handleListFabricApprovals(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.fabricApprovals == nil {
		writeJSON(w, http.StatusOK, map[string]any{"approvals": []any{}})
		return
	}
	items, err := gw.fabricApprovals.ListPending(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": items})
}

// handleDecideFabricApproval decides a unified approval. Body: {approve bool, note?}.
// Records the decision, stops the escalation climb, and unblocks any work item.
func (gw *Gateway) handleDecideFabricApproval(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.fabricApprovals == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "approvals not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Approve bool   `json:"approve"`
		Note    string `json:"note"`
	}
	if msg := decodeJSONBody(r, &body, 1<<20); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if err := gw.DecideApproval(r.Context(), id, body.Approve, user.ID, body.Note); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "approved": body.Approve})
}
