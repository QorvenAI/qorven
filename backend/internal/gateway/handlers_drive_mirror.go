// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/drive"
)

func (gw *Gateway) handleListMirrors(w http.ResponseWriter, r *http.Request) {
	if gw.mirrorStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mirrors not configured"})
		return
	}
	list, err := gw.mirrorStore.ListMirrors(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"mirrors": list})
}

func (gw *Gateway) handleCreateMirror(w http.ResponseWriter, r *http.Request) {
	if gw.mirrorStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "mirrors not configured"})
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	var body struct {
		Scope          string  `json:"scope"`
		ScopeID        *string `json:"scope_id"`
		OwnerAgentID   *string `json:"owner_agent_id"`
		Provider       string  `json:"provider"`
		AccountID      string  `json:"account_id"`
		RemoteFolderID string  `json:"remote_folder_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Provider == "" || body.Scope == "" {
		writeJSON(w, 400, map[string]string{"error": "provider and scope required"})
		return
	}
	switch body.Scope {
	case drive.ScopePrivate, drive.ScopeCompany, drive.ScopeDepartment, drive.ScopeCustom:
	default:
		writeJSON(w, 400, map[string]string{"error": "invalid scope"})
		return
	}
	id, err := gw.mirrorStore.CreateMirror(r.Context(), drive.Mirror{
		TenantID: defaultTenant, Scope: body.Scope, ScopeID: body.ScopeID, OwnerAgentID: body.OwnerAgentID,
		Provider: body.Provider, AccountID: body.AccountID, RemoteFolderID: body.RemoteFolderID,
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleDeleteMirror(w http.ResponseWriter, r *http.Request) {
	if gw.mirrorStore == nil {
		w.WriteHeader(503)
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err := gw.mirrorStore.DeleteMirror(r.Context(), defaultTenant, chi.URLParam(r, "id")); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}
