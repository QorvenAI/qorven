// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/plugins/registry"
)

// ─────────────────── POST /v1/plugins ───────────────────
//
// Uploads a .wasm binary for the caller's tenant. Admin-only.
//
// Request shape (multipart/form-data):
//   wasm          — binary file field, max 8 MB
//   name          — string, matches ^[a-z][a-z0-9_]{0,62}$
//   description   — optional string
//   parameters    — optional JSON string (tool parameter schema)
//
// Security gates:
//   1. AuthMiddlewareV2 has resolved the user.
//   2. Handler checks user.Role == "admin". Non-admins get 403.
//   3. TenantScopeMiddleware has already scoped the DB transaction
//      to the caller's tenant — the INSERT lands under RLS.
//   4. Max size is enforced by http.MaxBytesReader BEFORE we allocate
//      the full body. A plugin larger than the cap hits a 413 early.
//
// Returns 201 + the sanitized plugin metadata (no wasm bytes in the
// response body — those are KB-MB and we do not echo them back).
const maxWasmUploadBytes = 8 << 20 // 8 MiB cap — matches the AGENTS.md doc

func (gw *Gateway) handleUploadWasmPlugin(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "plugin upload requires admin role",
			"code":  "admin_only",
		})
		return
	}
	if gw.wasmPluginStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "plugin registry not configured",
		})
		return
	}

	// Hard-cap the request body at MaxWasmUploadBytes. MaxBytesReader
	// returns an error on the FIRST Read past the limit, so we never
	// buffer more than ~8 MiB. Next line's ParseMultipartForm uses
	// an in-memory budget too (maxMemory param).
	r.Body = http.MaxBytesReader(w, r.Body, maxWasmUploadBytes)
	if err := r.ParseMultipartForm(maxWasmUploadBytes); err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error": "upload exceeds 8 MiB limit",
			"code":  "payload_too_large",
		})
		return
	}
	file, _, err := r.FormFile("wasm")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "wasm file field missing",
			"code":  "missing_wasm",
		})
		return
	}
	defer file.Close()

	bin, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "could not read wasm body: " + err.Error(),
		})
		return
	}
	// ParseMultipartForm's limit is a soft budget; re-enforce the
	// hard cap on the buffered bytes. A multipart form with several
	// small fields could otherwise add up.
	if len(bin) > maxWasmUploadBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error": "wasm binary exceeds 8 MiB",
			"code":  "payload_too_large",
		})
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	paramsRaw := r.FormValue("parameters")

	var params json.RawMessage
	if paramsRaw != "" {
		if !json.Valid([]byte(paramsRaw)) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "parameters field is not valid JSON",
				"code":  "bad_parameters_json",
			})
			return
		}
		params = json.RawMessage(paramsRaw)
	}

	p, err := gw.wasmPluginStore.Upload(r.Context(), registry.UploadInput{
		TenantID:    user.TenantID,
		Name:        name,
		Description: description,
		WasmBinary:  bin,
		Parameters:  params,
		CreatedBy:   user.Username,
	})
	if err != nil {
		if errors.Is(err, registry.ErrInvalidName) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": err.Error(),
				"code":  "invalid_name",
			})
			return
		}
		if errors.Is(err, registry.ErrReservedName) {
			// Phase 6 security gate: surface the reservation as its
			// own error code so admin UIs and AI agents can render
			// a specific "pick a different name" prompt rather than
			// a generic 400. Explicitly list the conflicting name in
			// the body.
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "plugin name collides with a platform-reserved tool; pick a different name",
				"code":  "reserved_name",
				"name":  name,
			})
			return
		}
		slog.Error("plugin upload failed",
			"tenant_id", user.TenantID, "name", name, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "upload failed",
		})
		return
	}

	// Invalidate the loader cache so a subsequent plan run picks up
	// the new version. Safe to call with nil.
	if gw.wasmPluginLoader != nil {
		gw.wasmPluginLoader.Invalidate(user.TenantID, p.Name)
	}

	slog.Info("plugin uploaded",
		"tenant_id", user.TenantID, "name", p.Name,
		"sha256", p.SHA256, "size_bytes", len(bin), "actor", user.Username)

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          p.ID,
		"name":        p.Name,
		"description": p.Description,
		"sha256":      p.SHA256,
		"parameters":  p.Parameters,
		"size_bytes":  len(bin),
		"created_at":  p.CreatedAt,
	})
}

// ─────────────────── GET /v1/plugins ───────────────────
//
// Lists active plugins for the caller's tenant. Any authenticated
// user in the tenant may read — unlike upload which is admin-only.
// The response never includes the wasm binary.
func (gw *Gateway) handleListWasmPlugins(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.wasmPluginStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"plugins": []any{}})
		return
	}

	plugins, err := gw.wasmPluginStore.ListActive(r.Context(), user.TenantID)
	if err != nil {
		slog.Error("plugin list failed", "tenant_id", user.TenantID, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "list failed",
		})
		return
	}

	// Project to a response shape that omits wasm_binary.
	out := make([]map[string]any, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, map[string]any{
			"id":          p.ID,
			"name":        p.Name,
			"description": p.Description,
			"sha256":      p.SHA256,
			"parameters":  p.Parameters,
			"size_bytes":  len(p.WasmBinary),
			"created_by":  p.CreatedBy,
			"created_at":  p.CreatedAt,
			"updated_at":  p.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": out, "count": len(out)})
}

// ─────────────────── DELETE /v1/plugins/{name} ───────────────────
//
// Revokes (soft-deletes) the tenant's plugin with the given name.
// Admin-only. Idempotent: revoking an absent or already-revoked
// plugin returns 404.
func (gw *Gateway) handleRevokeWasmPlugin(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "plugin revoke requires admin role",
			"code":  "admin_only",
		})
		return
	}
	if gw.wasmPluginStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "plugin registry not configured",
		})
		return
	}

	name := chi.URLParam(r, "name")
	if err := gw.wasmPluginStore.Revoke(r.Context(), user.TenantID, name); err != nil {
		if errors.Is(err, registry.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": "plugin not found",
				"code":  "not_found",
			})
			return
		}
		if errors.Is(err, registry.ErrInvalidName) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": err.Error(),
				"code":  "invalid_name",
			})
			return
		}
		slog.Error("plugin revoke failed",
			"tenant_id", user.TenantID, "name", name, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "revoke failed",
		})
		return
	}

	if gw.wasmPluginLoader != nil {
		gw.wasmPluginLoader.Invalidate(user.TenantID, name)
	}
	slog.Info("plugin revoked",
		"tenant_id", user.TenantID, "name", name, "actor", user.Username)
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "name": name})
}
