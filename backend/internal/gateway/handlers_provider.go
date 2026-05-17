// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/providers"
)

func (gw *Gateway) handleListProviders(w http.ResponseWriter, r *http.Request) {
	// If DB store available, list from DB; otherwise list from registry
	if gw.providerStore != nil {
		list, err := gw.providerStore.List(r.Context(), defaultTenant)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if list == nil {
			list = []providers.ProviderConfig{}
		}
		writeJSON(w, 200, map[string]any{"providers": list})
		return
	}
	// Fallback: list from registry
	writeJSON(w, 200, map[string]any{"providers": gw.providerReg.List()})
}

func (gw *Gateway) handleCreateProviderDB(w http.ResponseWriter, r *http.Request) {
	if gw.providerStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var cfg providers.ProviderConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if cfg.Name == "" || cfg.ProviderType == "" {
		writeJSON(w, 400, map[string]string{"error": "name and provider_type required"})
		return
	}
	if !providers.IsValidName(cfg.Name) {
		writeJSON(w, 400, map[string]string{"error": "name must be lowercase alphanumeric with hyphens only"})
		return
	}
	if !providers.ValidProviderTypes[cfg.ProviderType] {
		writeJSON(w, 400, map[string]string{"error": "unsupported provider_type"})
		return
	}
	if cfg.APIBase == "" {
		cfg.APIBase = providers.DefaultAPIBase(cfg.ProviderType)
	}
	cfg.Enabled = true

	created, err := gw.providerStore.Create(r.Context(), defaultTenant, cfg)
	if err != nil {
		slog.Error("providers.create", "error", err)
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Register in live registry (with the key, before we cleared it)
	cfg.ID = created.ID
	gw.providerReg.Register(cfg)

	slog.Info("provider created", "id", created.ID, "name", cfg.Name, "type", cfg.ProviderType)
	writeJSON(w, 201, created)
}

func (gw *Gateway) handleGetProvider(w http.ResponseWriter, r *http.Request) {
	if gw.providerStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	cfg, err := gw.providerStore.Get(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}
	cfg.APIKey = maskKey(cfg.APIKey) // never return full key
	writeJSON(w, 200, cfg)
}

func (gw *Gateway) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	if gw.providerStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var cfg providers.ProviderConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	if err := gw.providerStore.Update(r.Context(), defaultTenant, id, cfg); err != nil {
		slog.Error("providers.update", "id", id, "error", err)
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Re-register in live registry
	gw.providerReg.Remove(id)
	full, err := gw.providerStore.Get(r.Context(), defaultTenant, id)
	if err == nil {
		gw.providerReg.Register(full)
	}

	slog.Info("provider updated", "id", id)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (gw *Gateway) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	if gw.providerStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	gw.providerStore.Delete(r.Context(), defaultTenant, id)
	gw.providerReg.Remove(id)
	slog.Info("provider deleted", "id", id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleVerifyProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := gw.providerReg.Get(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "provider not found in registry"})
		return
	}

	// 10s timeout for verification (don't hang on slow providers)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "Say hi"}},
		Options:  map[string]any{"max_tokens": 5},
	})
	if err != nil {
		slog.Warn("provider verify failed", "id", id, "error", err)
		writeJSON(w, 200, map[string]any{"status": "error", "error": err.Error()})
		return
	}
	slog.Info("provider verified", "id", id)
	writeJSON(w, 200, map[string]any{"status": "ok", "response": resp.Content})
}

func (gw *Gateway) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string `json:"name"`
		ProviderType string `json:"provider_type"`
		APIBase      string `json:"api_base"`
		APIKey       string `json:"api_key"`
		Region       string `json:"region"` // bedrock only — overrides api_base
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"success": false, "error": "invalid json"})
		return
	}
	if body.ProviderType == "" {
		writeJSON(w, 400, map[string]any{"success": false, "error": "provider_type required"})
		return
	}
	if !providers.ValidProviderTypes[body.ProviderType] {
		writeJSON(w, 400, map[string]any{"success": false, "error": "unsupported provider_type: " + body.ProviderType})
		return
	}

	// Build a throwaway provider config.
	cfg := providers.ProviderConfig{
		Name:         firstNonEmpty(body.Name, body.ProviderType),
		ProviderType: body.ProviderType,
		APIBase:      body.APIBase,
		APIKey:       body.APIKey,
		Enabled:      true,
	}
	if cfg.ProviderType == providers.TypeBedrock && body.Region != "" {
		cfg.APIBase = body.Region // Bedrock uses api_base as region
	}
	if cfg.APIBase == "" {
		cfg.APIBase = providers.DefaultAPIBase(cfg.ProviderType)
	}

	p, err := providers.NewProvider(cfg)
	if err != nil {
		writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}

	// Chat ping — 20s cap keeps the wizard responsive while tolerating
	// cold Bedrock inference profiles (first invoke after registration
	// can take 8-12s to warm up).
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "Say hi"}},
		Options:  map[string]any{"max_tokens": 5},
	})
	if err != nil {
		writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}

	// Model list — optional. Only some providers implement ListModels.
	models := []string{}
	type modelLister interface {
		ListModels(ctx context.Context) ([]string, error)
	}
	if ml, ok := p.(modelLister); ok {
		if ms, err := ml.ListModels(ctx); err == nil {
			models = ms
		}
	}
	if models == nil {
		models = []string{p.DefaultModel()}
	}

	writeJSON(w, 200, map[string]any{
		"success": true,
		"sample":  resp.Content,
		"models":  models,
	})
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func (gw *Gateway) handleListProviderModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, ok := gw.providerReg.Get(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}

	// Check if provider supports ListModels
	type modelLister interface {
		ListModels(ctx context.Context) ([]string, error)
	}
	if lister, ok := p.(modelLister); ok {
		models, err := lister.ListModels(r.Context())
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"models": models})
		return
	}
	writeJSON(w, 200, map[string]any{"models": []string{p.DefaultModel()}})
}

// handleUpdateProviderCapabilities handles PATCH /v1/providers/:id/capabilities.
func (gw *Gateway) handleUpdateProviderCapabilities(w http.ResponseWriter, r *http.Request) {
	if gw.providerStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var caps providers.ProviderCapabilityFlags
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&caps); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if err := gw.providerStore.UpdateCapabilities(r.Context(), defaultTenant, id, caps); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("provider.capabilities_updated", "id", id)
	writeJSON(w, 200, map[string]any{"id": id, "capabilities": caps})
}

func (gw *Gateway) handleListModels(w http.ResponseWriter, r *http.Request) {
	type modelLister interface {
		ListModels(ctx context.Context) ([]string, error)
	}
	allModels := []map[string]string{}
	for _, cfg := range gw.providerReg.List() {
		p, ok := gw.providerReg.Get(cfg.ID)
		if !ok {
			continue
		}
		if lister, ok := p.(modelLister); ok {
			models, err := lister.ListModels(r.Context())
			if err == nil {
				for _, m := range models {
					allModels = append(allModels, map[string]string{"id": m, "provider": cfg.Name})
				}
				continue
			}
		}
		allModels = append(allModels, map[string]string{"id": p.DefaultModel(), "provider": cfg.Name})
	}
	writeJSON(w, 200, map[string]any{"data": allModels, "object": "list"})
}
