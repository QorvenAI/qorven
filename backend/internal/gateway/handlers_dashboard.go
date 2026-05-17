// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/dashboard"
)

// tileResponse embeds a PinnedTile and adds a Data field from the snapshot store.
type tileResponse struct {
	dashboard.PinnedTile
	Data map[string]any `json:"data,omitempty"`
}

// listDashboardTiles returns all pinned tiles for the authenticated user's tenant,
// merging the latest snapshot data into each tile's "data" field.
func (gw *Gateway) listDashboardTiles(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.tileStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	tenantID := user.TenantID
	if tenantID == "" {
		tenantID = defaultTenant
	}

	tiles, err := gw.tileStore.List(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	resp := make([]tileResponse, 0, len(tiles))
	for _, t := range tiles {
		tr := tileResponse{PinnedTile: t}
		if gw.snapStore != nil {
			data, snapErr := gw.snapStore.Latest(r.Context(), tenantID, t.SourceSlug)
			if snapErr == nil && len(data) > 0 {
				tr.Data = data
			}
		}
		resp = append(resp, tr)
	}

	writeJSON(w, http.StatusOK, resp)
}

// createDashboardTile creates a new pinned tile for the authenticated user's tenant.
func (gw *Gateway) createDashboardTile(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.tileStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var tile dashboard.PinnedTile
	if err := json.NewDecoder(r.Body).Decode(&tile); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Validate required fields
	if tile.SourceSlug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "source_slug is required"})
		return
	}
	if tile.ToolName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tool_name is required"})
		return
	}
	validWidgetTypes := map[string]bool{
		"stat-card": true, "data-table": true, "feed": true, "list": true, "chart": true,
	}
	if !validWidgetTypes[tile.WidgetType] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "widget_type must be one of: stat-card, data-table, feed, list, chart"})
		return
	}

	tenantID := user.TenantID
	if tenantID == "" {
		tenantID = defaultTenant
	}
	tile.TenantID = tenantID

	if tile.ToolArgs == nil {
		tile.ToolArgs = map[string]any{}
	}
	if tile.RefreshIntervalSec <= 0 {
		tile.RefreshIntervalSec = 300
	}

	created, err := gw.tileStore.Create(r.Context(), tile)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// deleteDashboardTile removes a pinned tile by ID for the authenticated user's tenant.
func (gw *Gateway) deleteDashboardTile(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.tileStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	tenantID := user.TenantID
	if tenantID == "" {
		tenantID = defaultTenant
	}

	if err := gw.tileStore.Delete(r.Context(), tenantID, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
