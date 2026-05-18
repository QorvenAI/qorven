// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/apps"
)

// handleListApps returns all installed apps plus frontend manifests.
func (gw *Gateway) handleListApps(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"apps": []any{}, "frontend_manifests": []any{}})
		return
	}
	appList, err := gw.appMgr.Store().List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"apps":               appList,
		"frontend_manifests": gw.appMgr.FrontendManifests(),
	})
}

// handleGetApp returns a single app by ID.
func (gw *Gateway) handleGetApp(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	a, err := gw.appMgr.Store().Get(r.Context(), defaultTenant, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleInstallApp installs an app from a directory path on disk (admin only).
func (gw *Gateway) handleInstallApp(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app manager not available"})
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must contain {path}"})
		return
	}
	created, err := gw.appMgr.Install(r.Context(), req.Path)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handlePatchApp toggles enabled or updates config.
func (gw *Gateway) handlePatchApp(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		Enabled *bool          `json:"enabled"`
		Config  map[string]any `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if req.Enabled != nil {
		var err error
		if *req.Enabled {
			err = gw.appMgr.Enable(r.Context(), id)
		} else {
			err = gw.appMgr.Disable(r.Context(), id)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
			return
		}
	}
	if req.Config != nil {
		if err := gw.appMgr.Store().SetConfig(r.Context(), defaultTenant, id, req.Config); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
			return
		}
	}
	a, err := gw.appMgr.Store().Get(r.Context(), defaultTenant, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleUninstallApp removes an app. Pass ?drop_tables=true to also drop app-owned tables.
func (gw *Gateway) handleUninstallApp(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	dropTables := r.URL.Query().Get("drop_tables") == "true"
	if err := gw.appMgr.Uninstall(r.Context(), id, dropTables); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// handleReloadApp re-reads the manifest and re-registers tools/hooks for an app.
func (gw *Gateway) handleReloadApp(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app manager not available"})
		return
	}
	id := chi.URLParam(r, "id")
	a, err := gw.appMgr.Store().Get(r.Context(), defaultTenant, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err := gw.appMgr.Reload(r.Context(), a.Slug); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	updated, _ := gw.appMgr.Store().Get(r.Context(), defaultTenant, id)
	writeJSON(w, http.StatusOK, updated)
}

// handleRunAppTool executes a named tool on an enabled app.
// POST /v1/apps/{slug}/tools/{name}
// Body (optional): {"args": {...}}
// Response: tools.Result as JSON
func (gw *Gateway) handleRunAppTool(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	// Any authenticated user may invoke app tools (unlike install/uninstall which are admin-only).
	// Tools run with QORVEN_DB_DSN but in an isolated subprocess with no token inheritance.
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.appMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "app manager not available"})
		return
	}

	slug := chi.URLParam(r, "slug")
	toolName := chi.URLParam(r, "name")

	var req struct {
		Args map[string]any `json:"args"`
	}
	// Body is optional — ignore decode errors (empty args is valid)
	json.NewDecoder(r.Body).Decode(&req)
	if req.Args == nil {
		req.Args = map[string]any{}
	}

	result, err := gw.appMgr.RunTool(r.Context(), slug, toolName, req.Args)
	if err != nil {
		if errors.Is(err, apps.ErrAppNotLoaded) || errors.Is(err, apps.ErrToolNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAppAsset serves an app's frontend bundle.js.
// This route is public (no auth) — the bundle calls /api/v1 with the user's token.
func (gw *Gateway) handleAppAsset(w http.ResponseWriter, r *http.Request) {
	if gw.appMgr == nil {
		http.NotFound(w, r)
		return
	}
	slug := chi.URLParam(r, "slug")
	path, ok := gw.appMgr.BundlePath(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	var modTime time.Time
	if fi, err := os.Stat(path); err == nil {
		modTime = fi.ModTime()
	}
	http.ServeContent(w, r, "bundle.js", modTime, f)
}
