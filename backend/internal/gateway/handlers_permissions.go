// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/permissions"
)

// handleListAgentPermissions returns all permission policies for a specific agent.
// GET /v1/agents/{id}/permissions
func (gw *Gateway) handleListAgentPermissions(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	policies, err := gw.permissionGate.ListPolicies(r.Context(), defaultTenant, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if policies == nil {
		policies = []permissions.PolicyEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"policies": policies})
}

// handleUpsertAgentPermission creates or updates a permission policy for a tool on a specific agent.
// PUT /v1/agents/{id}/permissions
func (gw *Gateway) handleUpsertAgentPermission(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	var body struct {
		Tool  string `json:"tool"`
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Tool == "" || body.Scope == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool and scope required"})
		return
	}
	scope := permissions.PermScope(body.Scope)
	switch scope {
	case permissions.ScopeAutoApproved, permissions.ScopeAskFirst, permissions.ScopeBlocked:
		// valid
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be auto_approved, ask_first, or blocked"})
		return
	}
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if err := gw.permissionGate.SetPolicyScoped(r.Context(), defaultTenant, userID, agentID, body.Tool, scope); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "tool": body.Tool, "scope": body.Scope})
}

// handleDeleteAgentPermission removes a permission policy for a specific tool on a specific agent.
// DELETE /v1/agents/{id}/permissions/{tool}
func (gw *Gateway) handleDeleteAgentPermission(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	tool := chi.URLParam(r, "tool")
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if err := gw.permissionGate.RevokePolicy(r.Context(), defaultTenant, userID, agentID, tool); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
