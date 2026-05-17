// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handleListSandboxApps returns all running sandbox apps for the authenticated
// user's tenant.
//
// GET /v1/sandbox/apps
func (gw *Gateway) handleListSandboxApps(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.appRunner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	apps, err := gw.appRunner.List(r.Context(), user.TenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

// handleStopSandboxApp stops a running sandbox app by ID.
//
// DELETE /v1/sandbox/apps/{id}
func (gw *Gateway) handleStopSandboxApp(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.appRunner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sandbox not available"})
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
	if err := gw.appRunner.Stop(r.Context(), id, user.TenantID); err != nil {
		if err.Error() == "app not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}
