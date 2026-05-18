// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/mediagen"
)

// handleMediaCatalog returns the list of available image/video generation drivers.
// GET /v1/media/catalog
func (gw *Gateway) handleMediaCatalog(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind") // optional filter: "image", "video"
	entries := mediagen.Catalog()
	if kind != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.Kind == kind {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	writeJSON(w, 200, map[string]any{"drivers": entries, "count": len(entries)})
}

// handleMediaProvidersList lists configured media providers for this tenant.
// GET /v1/media/providers
func (gw *Gateway) handleMediaProvidersList(w http.ResponseWriter, r *http.Request) {
	if gw.mediaStore == nil {
		writeJSON(w, 200, map[string]any{"providers": []any{}, "manager": map[string]any{}})
		return
	}
	rows, err := gw.mediaStore.List(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Redact API keys
	for i := range rows {
		rows[i].APIKey = ""
	}
	mgr := map[string]any{}
	if gw.mediaMgr != nil {
		mgr = gw.mediaMgr.ListProviders()
	}
	writeJSON(w, 200, map[string]any{"providers": rows, "manager": mgr})
}

// handleMediaProvidersCreate adds a new media provider.
// POST /v1/media/providers
func (gw *Gateway) handleMediaProvidersCreate(w http.ResponseWriter, r *http.Request) {
	if gw.mediaStore == nil {
		http.Error(w, "media store not configured", http.StatusServiceUnavailable)
		return
	}
	var body mediagen.ProviderRow
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if body.Kind == "" {
		body.Kind = "image"
	}

	created, err := gw.mediaStore.Create(r.Context(), defaultTenant, body)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Hot-register with manager
	if gw.mediaMgr != nil && created.Kind == "image" {
		full, _ := gw.mediaStore.GetByID(r.Context(), defaultTenant, created.ID)
		if full != nil {
			if p, err := mediagen.BuildProvider(*full); err == nil {
				gw.mediaMgr.RegisterImage(p)
				if full.IsDefault {
					gw.mediaMgr.SetPrimaryImage(p.Name())
				}
			}
		}
	}

	created.APIKey = ""
	writeJSON(w, 200, created)
}

// handleMediaProvidersUpdate updates a media provider.
// PUT /v1/media/providers/{id}
func (gw *Gateway) handleMediaProvidersUpdate(w http.ResponseWriter, r *http.Request) {
	if gw.mediaStore == nil {
		http.Error(w, "media store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var body mediagen.ProviderRow
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	body.ID = id
	if err := gw.mediaStore.Update(r.Context(), defaultTenant, body); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Hot-reload
	if gw.mediaMgr != nil && body.Kind == "image" {
		full, _ := gw.mediaStore.GetByID(r.Context(), defaultTenant, id)
		if full != nil {
			if p, err := mediagen.BuildProvider(*full); err == nil {
				gw.mediaMgr.RegisterImage(p)
			}
		}
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// handleMediaProvidersDelete removes a media provider.
// DELETE /v1/media/providers/{id}
func (gw *Gateway) handleMediaProvidersDelete(w http.ResponseWriter, r *http.Request) {
	if gw.mediaStore == nil {
		http.Error(w, "media store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.mediaStore.Delete(r.Context(), defaultTenant, id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleMediaProvidersSetDefault sets a provider as the default for its kind.
// POST /v1/media/providers/{id}/default
func (gw *Gateway) handleMediaProvidersSetDefault(w http.ResponseWriter, r *http.Request) {
	if gw.mediaStore == nil {
		http.Error(w, "media store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct{ Kind string `json:"kind"` }
	json.NewDecoder(r.Body).Decode(&body)
	kind := body.Kind
	if kind == "" {
		kind = "image"
	}
	if err := gw.mediaStore.SetDefault(r.Context(), defaultTenant, id, kind); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Update manager primary
	if gw.mediaMgr != nil && kind == "image" {
		full, _ := gw.mediaStore.GetByID(r.Context(), defaultTenant, id)
		if full != nil {
			if p, err := mediagen.BuildProvider(*full); err == nil {
				gw.mediaMgr.SetPrimaryImage(p.Name())
			}
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// handleMediaProvidersTest generates a test image.
// POST /v1/media/providers/{id}/test
func (gw *Gateway) handleMediaProvidersTest(w http.ResponseWriter, r *http.Request) {
	if gw.mediaStore == nil {
		http.Error(w, "media store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	full, err := gw.mediaStore.GetByID(r.Context(), defaultTenant, id)
	if err != nil || full == nil {
		http.Error(w, "provider not found", 404)
		return
	}
	p, err := mediagen.BuildProvider(*full)
	if err != nil {
		writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}
	result, err := p.Generate(r.Context(), "a small red circle on white background", mediagen.ImageOptions{Size: "1024x1024"})
	if err != nil {
		writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}
	size := len(result.B64JSON)
	if result.URL != "" {
		size = len(result.URL)
	}
	writeJSON(w, 200, map[string]any{"success": true, "bytes": size, "url": result.URL})
}
