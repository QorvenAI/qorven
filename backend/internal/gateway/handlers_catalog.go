// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// handleCatalogSearch searches the Pipedream integration catalog.
// GET /v1/connectors/catalog?q=slack&limit=50
func (gw *Gateway) handleCatalogSearch(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	if gw.catalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "catalog not available — add Pipedream API key in Settings"})
		return
	}

	// Refresh cache if stale
	_ = gw.catalog.Refresh(r.Context())

	query := r.URL.Query().Get("q")
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}

	results := gw.catalog.Search(query, limit)
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   gw.catalog.Count(),
	})
}

// handleCatalogActivate installs a catalog app as a local connector platform.
// POST /v1/connectors/catalog/activate
func (gw *Gateway) handleCatalogActivate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.catalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "catalog not available"})
		return
	}

	var body struct {
		Slug       string   `json:"slug"`
		Name       string   `json:"name"`
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Slug == "" || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug and name required"})
		return
	}

	if err := gw.catalog.Activate(r.Context(), body.Slug, body.Name, body.Categories); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "activated", "platform_id": body.Slug})
}

// handleCatalogDiscover fetches and stores actions for an activated platform.
// POST /v1/connectors/catalog/discover
func (gw *Gateway) handleCatalogDiscover(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.catalog == nil || gw.relayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "catalog not available"})
		return
	}

	var body struct {
		PlatformID string `json:"platform_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlatformID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform_id required"})
		return
	}

	apiKey, err := gw.relayStore.GetRelayKey(r.Context(), defaultTenant, "pipedream")
	if err != nil || apiKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pipedream not configured"})
		return
	}

	count, err := gw.catalog.DiscoverAndStoreActions(r.Context(), body.PlatformID, apiKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"platform_id":    body.PlatformID,
		"actions_stored": count,
	})
}
