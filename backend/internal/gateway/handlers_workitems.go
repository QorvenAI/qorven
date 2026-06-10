// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/qorvenai/qorven/internal/agent"
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
	room := r.URL.Query().Get("room")
	var items []workitems.WorkItem
	var err error
	if room != "" {
		items, err = gw.workItems.ListForRoom(r.Context(), defaultTenant, room, status)
	} else {
		items, err = gw.workItems.ListForOwner(r.Context(), defaultTenant, owner, status)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"work_items": gw.enrichWorkItems(r.Context(), items)})
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
	dto := gw.enrichWorkItems(r.Context(), []workitems.WorkItem{*item})
	writeJSON(w, http.StatusOK, map[string]any{"work_item": dto[0], "events": events})
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

// workItemDTO is the JSON shape returned to the frontend. WorkItem itself has
// no json tags, so we project explicit lowercase keys and add the owner's key
// and display name (resolved from owner_agent_id).
type workItemDTO struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Origin    string `json:"origin"`
	OwnerID   string `json:"owner_agent_id"`
	OwnerKey  string `json:"owner_key"`
	OwnerName string `json:"owner_name"`
	Status    string `json:"status"`
	BlockedOn string `json:"blocked_on_kind"`
	ParentID  string `json:"parent_id"`
}

// enrichWorkItems resolves owner_agent_id → key/name in one batch query and
// returns DTOs. On any resolution error it falls back to the raw owner id.
func (gw *Gateway) enrichWorkItems(ctx context.Context, items []workitems.WorkItem) []workItemDTO {
	idSet := map[string]struct{}{}
	for _, it := range items {
		if it.OwnerAgentID != "" {
			idSet[it.OwnerAgentID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	byID := map[string]*agent.Agent{}
	if gw.agents != nil && len(ids) > 0 {
		if ags, err := gw.agents.GetByIDs(ctx, ids); err == nil {
			for _, a := range ags {
				byID[a.ID] = a
			}
		}
	}
	out := make([]workItemDTO, 0, len(items))
	for _, it := range items {
		d := workItemDTO{
			ID: it.ID, Title: it.Title, Origin: it.Origin, OwnerID: it.OwnerAgentID,
			OwnerKey: it.OwnerAgentID, Status: it.Status, BlockedOn: it.BlockedOnKind, ParentID: it.ParentID,
		}
		if a, ok := byID[it.OwnerAgentID]; ok {
			d.OwnerKey = a.AgentKey
			d.OwnerName = a.DisplayName
		}
		out = append(out, d)
	}
	return out
}
