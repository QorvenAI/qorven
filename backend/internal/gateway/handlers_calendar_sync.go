// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/calendar"
)

func (gw *Gateway) handleListCalendarSyncs(w http.ResponseWriter, r *http.Request) {
	if gw.calSyncStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "calendar sync not configured"})
		return
	}
	list, err := gw.calSyncStore.ListSyncs(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"syncs": list})
}

func (gw *Gateway) handleCreateCalendarSync(w http.ResponseWriter, r *http.Request) {
	if gw.calSyncStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "calendar sync not configured"})
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	var body struct {
		Scope            string  `json:"scope"`
		ScopeID          *string `json:"scope_id"`
		OwnerAgentID     *string `json:"owner_agent_id"`
		Provider         string  `json:"provider"`
		AccountID        string  `json:"account_id"`
		RemoteCalendarID string  `json:"remote_calendar_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Provider == "" || body.Scope == "" {
		writeJSON(w, 400, map[string]string{"error": "provider and scope required"})
		return
	}
	switch body.Scope {
	case "private", "company", "department":
	default:
		writeJSON(w, 400, map[string]string{"error": "invalid scope"})
		return
	}
	id, err := gw.calSyncStore.CreateSync(r.Context(), calendar.Sync{
		TenantID: defaultTenant, Scope: body.Scope, ScopeID: body.ScopeID, OwnerAgentID: body.OwnerAgentID,
		Provider: body.Provider, AccountID: body.AccountID, RemoteCalendarID: body.RemoteCalendarID,
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleDeleteCalendarSync(w http.ResponseWriter, r *http.Request) {
	if gw.calSyncStore == nil {
		w.WriteHeader(503)
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if err := gw.calSyncStore.DeleteSync(r.Context(), defaultTenant, chi.URLParam(r, "id")); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

func (gw *Gateway) handleSyncCalendarNow(w http.ResponseWriter, r *http.Request) {
	if gw.calSyncStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "calendar sync not configured"})
		return
	}
	if u := userFromContext(r.Context()); u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	safeGo("calendar.sync.manual", func() {
		cctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		gw.runCalendarSync(cctx)
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "sync started"})
}
