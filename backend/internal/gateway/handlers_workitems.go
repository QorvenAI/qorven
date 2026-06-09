// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/qorvenai/qorven/internal/workitems"
)

// handleCreateWorkItem creates a work item. Body: {title, owner_agent_id?, origin?, requested_by?}.
func (gw *Gateway) handleCreateWorkItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.workItems == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "work items not available"})
		return
	}
	var body struct {
		Title        string `json:"title"`
		OwnerAgentID string `json:"owner_agent_id"`
		Origin       string `json:"origin"`
		RequestedBy  string `json:"requested_by"`
	}
	if msg := decodeJSONBody(r, &body, 1<<20); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if body.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}
	if body.RequestedBy == "" {
		body.RequestedBy = user.ID
	}
	id, err := gw.workItems.Create(r.Context(), workitems.WorkItem{
		TenantID: defaultTenant, Title: body.Title, OwnerAgentID: body.OwnerAgentID,
		Origin: body.Origin, RequestedBy: body.RequestedBy,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// handleListWorkItems lists work items for an owner (?owner=agentID&status=).
func (gw *Gateway) handleListWorkItems(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.workItems == nil {
		writeJSON(w, http.StatusOK, map[string]any{"work_items": []any{}})
		return
	}
	owner := r.URL.Query().Get("owner")
	status := r.URL.Query().Get("status")
	items, err := gw.workItems.ListForOwner(r.Context(), defaultTenant, owner, status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"work_items": items})
}

// handleGetWorkItem returns a work item + its events.
func (gw *Gateway) handleGetWorkItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.workItems == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "work items not available"})
		return
	}
	id := chi.URLParam(r, "id")
	item, err := gw.workItems.Get(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	events, _ := gw.workItems.Events(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{"work_item": item, "events": events})
}

// handleTransitionWorkItem moves a work item. Body: {to, detail?}.
func (gw *Gateway) handleTransitionWorkItem(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.workItems == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "work items not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		To     string `json:"to"`
		Detail string `json:"detail"`
	}
	if msg := decodeJSONBody(r, &body, 1<<20); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if err := gw.workItems.Transition(r.Context(), id, body.To, user.ID, body.Detail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
