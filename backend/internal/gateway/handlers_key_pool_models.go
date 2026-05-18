// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/artificialanalysis"
	"github.com/qorvenai/qorven/internal/crypto"
	"github.com/qorvenai/qorven/internal/llmstats"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
)

func (gw *Gateway) handleListProviderKeys(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	providerID := chi.URLParam(r, "provider_id")
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	keys, err := store.ListKeys(r.Context(), defaultTenant, providerID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(keys)
}

func (gw *Gateway) handleAddProviderKey(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	providerID := chi.URLParam(r, "provider_id")
	var body struct {
		Label string `json:"label"`
		Key   string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Key == "" {
		http.Error(w, `{"error":"key required"}`, 400)
		return
	}
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	kr, err := store.AddKey(r.Context(), defaultTenant, providerID, body.Label, body.Key)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(kr)
}

func (gw *Gateway) handleVerifyProviderKey(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	keyID := chi.URLParam(r, "key_id")
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	store.VerifyKey(r.Context(), keyID)
	json.NewEncoder(w).Encode(map[string]string{"status": "verified"})
}

func (gw *Gateway) handleRetireProviderKey(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	keyID := chi.URLParam(r, "key_id")
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	store.RetireKey(r.Context(), keyID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleKeyUsageLogs(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	providerID := chi.URLParam(r, "provider_id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT l.id, l.key_id, pk.label, l.model, l.tokens_in, l.tokens_out, l.latency_ms, l.status, l.created_at
		 FROM key_usage_log l JOIN provider_keys pk ON l.key_id = pk.id
		 WHERE pk.provider_id = $1 AND pk.tenant_id = $2 ORDER BY l.created_at DESC LIMIT 100`,
		providerID, defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	logs := []map[string]any{}
	for rows.Next() {
		var id, keyID, label, model, status string
		var tokIn, tokOut, latency int
		var createdAt time.Time
		rows.Scan(&id, &keyID, &label, &model, &tokIn, &tokOut, &latency, &status, &createdAt)
		logs = append(logs, map[string]any{
			"id": id, "key_id": keyID, "key_label": label, "model": model,
			"tokens_in": tokIn, "tokens_out": tokOut, "latency_ms": latency,
			"status": status, "created_at": createdAt,
		})
	}
	json.NewEncoder(w).Encode(logs)
}

func (gw *Gateway) handleFetchLiveModels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	providerID := chi.URLParam(r, "provider_id")

	// Load the provider record from DB (providerID is a UUID, not a catalog key)
	var provType, apiBase string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT provider_type, COALESCE(api_base,'') FROM providers WHERE id = $1 AND tenant_id = $2`,
		providerID, defaultTenant).Scan(&provType, &apiBase)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}
	if apiBase == "" {
		apiBase = providers.DefaultAPIBase(provType)
	}

	// Get any key for this provider (not just verified — this is called right after add).
	kpStore := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	keys, _ := kpStore.ListKeys(r.Context(), defaultTenant, providerID)
	var apiKey string
	// Prefer verified, fall back to any key
	for _, k := range keys {
		if k.Status == "verified" {
			if dec, err := crypto.Decrypt(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey); err == nil {
				apiKey = string(dec)
				break
			}
		}
	}
	if apiKey == "" {
		for _, k := range keys {
			if dec, err := crypto.Decrypt(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey); err == nil {
				apiKey = string(dec)
				break
			}
		}
	}

	// Try live API first
	if apiKey != "" {
		models, err := providers.FetchModelsLive(r.Context(), provType, apiBase, apiKey)
		if err == nil && len(models) > 0 {
			json.NewEncoder(w).Encode(map[string]any{"models": models})
			return
		}
	}

	// Fall back to static list from the provider catalog
	staticModels := providers.StaticModelsForProviderType(provType)
	json.NewEncoder(w).Encode(map[string]any{"models": staticModels})
}

func (gw *Gateway) handleModelCatalog(w http.ResponseWriter, r *http.Request) {
	static, err := providers.LoadCatalog()
	if err != nil || static == nil {
		// Legacy fallback — keep old behaviour for tests / old clients.
		catalog := providers.GetModelCatalog(r.Context())
		cap := r.URL.Query().Get("capability")
		tier := r.URL.Query().Get("tier")
		search := r.URL.Query().Get("search")
		filtered := []providers.ModelMeta{}
		for _, m := range catalog {
			if cap != "" {
				found := false
				for _, c := range m.Capabilities {
					if c == cap {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if tier != "" && m.Tier != tier {
				continue
			}
			if search != "" {
				s := strings.ToLower(search)
				if !strings.Contains(strings.ToLower(m.Name), s) && !strings.Contains(strings.ToLower(m.ID), s) && !strings.Contains(strings.ToLower(m.Provider), s) {
					continue
				}
			}
			filtered = append(filtered, m)
		}
		json.NewEncoder(w).Encode(filtered)
		return
	}

	q := r.URL.Query()
	tier := q.Get("tier")
	provider := q.Get("provider")
	capability := q.Get("capability")
	category := q.Get("category")
	search := strings.ToLower(q.Get("search"))

	entries := static.All()
	filtered := entries[:0:0]
	for _, e := range entries {
		if tier != "" && e.Tier != tier {
			continue
		}
		if provider != "" && e.Provider != provider {
			continue
		}
		if capability != "" {
			ok := false
			for _, c := range e.Capabilities {
				if c == capability {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if category != "" {
			ok := false
			for _, c := range e.CategoryStrengths {
				if c == category {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		if search != "" {
			hay := strings.ToLower(e.ID + " " + e.DisplayName + " " + e.Provider)
			if !strings.Contains(hay, search) {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	writeJSON(w, 200, map[string]any{
		"models": filtered,
		"count":  len(filtered),
		"total":  len(entries),
	})
}

func (gw *Gateway) handleRecommendedModels(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(providers.GetRecommended())
}

func (gw *Gateway) handleListSelectedModels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"models": []any{}})
		return
	}
	providerID := r.URL.Query().Get("provider_id")
	query := `SELECT provider_id, model_id, is_default, display_order FROM selected_models WHERE tenant_id = $1`
	args := []any{defaultTenant}
	if providerID != "" {
		query += ` AND provider_id = $2`
		args = append(args, providerID)
	}
	query += ` ORDER BY is_default DESC, display_order, created_at`
	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	models := []map[string]any{}
	for rows.Next() {
		var pid, mid string
		var isDefault bool
		var order int
		rows.Scan(&pid, &mid, &isDefault, &order)
		models = append(models, map[string]any{"provider_id": pid, "model_id": mid, "is_default": isDefault, "display_order": order})
	}
	if models == nil {
		models = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"models": models})
}

func (gw *Gateway) handleSelectModel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	var body struct {
		ProviderID string `json:"provider_id"`
		ModelID    string `json:"model_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO selected_models (tenant_id, provider_id, model_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		defaultTenant, body.ProviderID, body.ModelID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleDeselectModel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	var body struct {
		ProviderID string `json:"provider_id"`
		ModelID    string `json:"model_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	gw.db.Pool.Exec(r.Context(),
		`DELETE FROM selected_models WHERE tenant_id = $1 AND provider_id = $2 AND model_id = $3`,
		defaultTenant, body.ProviderID, body.ModelID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleSetDefaultModel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	var body struct {
		ProviderID string `json:"provider_id"`
		ModelID    string `json:"model_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	// Clear existing default for this provider
	gw.db.Pool.Exec(r.Context(),
		`UPDATE selected_models SET is_default = false WHERE tenant_id = $1 AND provider_id = $2`,
		defaultTenant, body.ProviderID)
	// Set new default
	gw.db.Pool.Exec(r.Context(),
		`UPDATE selected_models SET is_default = true WHERE tenant_id = $1 AND provider_id = $2 AND model_id = $3`,
		defaultTenant, body.ProviderID, body.ModelID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleSoulUsage(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	soulID := chi.URLParam(r, "soul_id")
	ps := providers.NewPricingStore(gw.db.Pool)
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	monthCost, monthCalls, monthTokens := ps.GetSoulSpend(r.Context(), soulID, monthStart)
	allCost, allCalls, allTokens := ps.GetSoulSpend(r.Context(), soulID, time.Time{})

	// Top models
	rows, _ := gw.db.Pool.Query(r.Context(),
		`SELECT model_id, COUNT(*), COALESCE(SUM(cost_usd), 0), COALESCE(SUM(input_tokens + output_tokens), 0)
		 FROM soul_usage WHERE soul_id = $1 AND called_at >= $2 GROUP BY model_id ORDER BY 3 DESC LIMIT 5`, soulID, monthStart)
	topModels := []map[string]any{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var model string
			var calls int
			var cost float64
			var tokens int
			rows.Scan(&model, &calls, &cost, &tokens)
			topModels = append(topModels, map[string]any{"model": model, "calls": calls, "cost": cost, "tokens": tokens})
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"this_month": map[string]any{"cost": monthCost, "calls": monthCalls, "tokens": monthTokens},
		"all_time":   map[string]any{"cost": allCost, "calls": allCalls, "tokens": allTokens},
		"top_models": topModels,
	})
}

func (gw *Gateway) handleAccountUsage(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	ps := providers.NewPricingStore(gw.db.Pool)
	totalCost := ps.GetAccountSpend(r.Context(), defaultTenant)

	// Per-soul breakdown
	rows, _ := gw.db.Pool.Query(r.Context(),
		`SELECT a.id, a.display_name, COALESCE(SUM(u.cost_usd), 0), COUNT(u.id)
		 FROM agents a LEFT JOIN soul_usage u ON a.id = u.soul_id AND u.called_at >= date_trunc('month', now())
		 WHERE a.tenant_id = $1 AND a.deleted_at IS NULL GROUP BY a.id, a.display_name ORDER BY 3 DESC`, defaultTenant)
	souls := []map[string]any{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var cost float64
			var calls int
			rows.Scan(&id, &name, &cost, &calls)
			souls = append(souls, map[string]any{"id": id, "name": name, "cost": cost, "calls": calls})
		}
	}

	json.NewEncoder(w).Encode(map[string]any{"total_cost_this_month": totalCost, "souls": souls})
}

func (gw *Gateway) handleRefreshPricing(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	ps := providers.NewPricingStore(gw.db.Pool)
	if err := ps.FetchAndCacheModelPricing(r.Context()); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "refreshed"})
}

func (gw *Gateway) handleListCategories(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	router := providers.NewSmartRouter(gw.db.Pool)
	cats, err := router.ListCategories(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(cats)
}

func (gw *Gateway) handleGetAssignments(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	router := providers.NewSmartRouter(gw.db.Pool)
	assignments, err := router.GetAssignments(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(assignments)
}

func (gw *Gateway) handleAssignModel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	var body struct {
		Category string `json:"category"`
		ModelID  string `json:"model_id"`
		Priority int    `json:"priority"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	router := providers.NewSmartRouter(gw.db.Pool)
	router.AssignModel(r.Context(), defaultTenant, body.Category, body.ModelID, body.Priority)
	w.WriteHeader(204)
}

func (gw *Gateway) handleUnassignModel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	var body struct {
		Category string `json:"category"`
		ModelID  string `json:"model_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	router := providers.NewSmartRouter(gw.db.Pool)
	router.UnassignModel(r.Context(), defaultTenant, body.Category, body.ModelID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleClassifyQuery(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Query string `json:"query"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	router := providers.NewSmartRouter(gw.db.Pool)
	decision, err := router.ClassifyAndRoute(r.Context(), defaultTenant, body.Query)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(decision)
}

func (gw *Gateway) handleRecentDecisions(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	router := providers.NewSmartRouter(gw.db.Pool)
	decisions, err := router.GetRecentDecisions(r.Context(), defaultTenant, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if decisions == nil {
		decisions = []map[string]any{}
	}
	json.NewEncoder(w).Encode(decisions)
}

func (gw *Gateway) handleCorrectDecision(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	var body struct {
		DecisionID string `json:"decision_id"`
		Model      string `json:"model"`
		Category   string `json:"category"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	router := providers.NewSmartRouter(gw.db.Pool)
	router.CorrectDecision(r.Context(), body.DecisionID, body.Model, body.Category)
	w.WriteHeader(204)
}

func (gw *Gateway) handleSearchSessions(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		writeJSON(w, 200, map[string]any{"sessions": []any{}, "count": 0, "query": r.URL.Query().Get("q")})
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, 400, map[string]string{"error": "q parameter required"})
		return
	}
	sessions, err := gw.sessions.Search(r.Context(), defaultTenant, query, 10)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions, "count": len(sessions), "query": query})
}

func (gw *Gateway) handleSetKeyBudget(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	keyID := chi.URLParam(r, "key_id")
	var body struct {
		BudgetUSD    *float64 `json:"budget_usd_monthly"`
		BudgetTokens *int64   `json:"budget_tokens_monthly"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	if err := store.SetKeyBudget(r.Context(), keyID, body.BudgetUSD, body.BudgetTokens); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}

func (gw *Gateway) handleTestKeyAndFetchModels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	keyID := chi.URLParam(r, "key_id")
	kpStore := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)

	// Resolve provider_type + api_base + encrypted key from DB
	var providerID, provType, apiBase string
	var keyEncBytes []byte
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT pk.provider_id, pk.key_enc, p.provider_type, COALESCE(p.api_base,'')
		 FROM provider_keys pk JOIN providers p ON p.id::text = pk.provider_id
		 WHERE pk.id = $1 AND pk.tenant_id = $2`,
		keyID, defaultTenant).Scan(&providerID, &keyEncBytes, &provType, &apiBase)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "key not found"})
		return
	}
	if apiBase == "" {
		apiBase = providers.DefaultAPIBase(provType)
	}

	apiKey, decErr := providers.DecryptKeyBytes(keyEncBytes, gw.cfg.Auth.EncryptionKey)
	if decErr != nil || apiKey == "" {
		writeJSON(w, 400, map[string]string{"error": "could not decrypt key"})
		return
	}

	// Actually test the key against the live API
	models, liveErr := providers.FetchModelsLive(r.Context(), provType, apiBase, apiKey)
	if liveErr != nil {
		// Mark as failed so the UI shows an error badge
		kpStore.MarkKeyFailed(r.Context(), keyID)
		writeJSON(w, 200, map[string]any{
			"verified": false,
			"error":    "Verification failed: " + liveErr.Error(),
			"models":   []any{},
		})
		return
	}

	// Key works — mark verified
	kpStore.VerifyKey(r.Context(), keyID)

	if len(models) == 0 {
		models = providers.StaticModelsForProviderType(provType)
	}
	if models == nil {
		models = []providers.ModelInfo{}
	}
	writeJSON(w, 200, map[string]any{"verified": true, "models": models})
}

func (gw *Gateway) handleGetPoolConfig(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	providerID := chi.URLParam(r, "provider_id")
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	cfg, err := store.LoadPoolConfig(r.Context(), defaultTenant, providerID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, cfg)
}

func (gw *Gateway) handleSavePoolConfig(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	providerID := chi.URLParam(r, "provider_id")
	var cfg providers.PoolConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	store := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	if err := store.SavePoolConfig(r.Context(), defaultTenant, providerID, cfg); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}

func (gw *Gateway) handleGetAvailableModels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"models": []any{}, "count": 0})
		return
	}
	category := r.URL.Query().Get("category")
	query := `SELECT sm.provider_id, sm.model_id, sm.is_default, sm.display_order, sm.category
	          FROM selected_models sm WHERE sm.tenant_id = $1`
	args := []any{defaultTenant}
	if category != "" {
		query += ` AND sm.category = $2`
		args = append(args, category)
	}
	query += ` ORDER BY sm.is_default DESC, sm.display_order, sm.model_id`
	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	models := []map[string]any{}
	for rows.Next() {
		var pid, mid, cat string
		var isDefault bool
		var order int
		rows.Scan(&pid, &mid, &isDefault, &order, &cat)
		models = append(models, map[string]any{
			"provider_id": pid, "model_id": mid,
			"is_default": isDefault, "display_order": order, "category": cat,
			"context_window": providers.GetContextWindow(mid),
		})
	}
	if models == nil {
		models = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"models": models, "count": len(models)})
}

func (gw *Gateway) handleListDiscoveredModels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"discoveries": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, provider_id, model_id, first_seen_at, user_action
		 FROM model_discoveries WHERE tenant_id = $1
		 ORDER BY first_seen_at DESC LIMIT 200`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	discoveries := []map[string]any{}
	for rows.Next() {
		var id, providerID, modelID, userAction string
		var firstSeen time.Time
		rows.Scan(&id, &providerID, &modelID, &firstSeen, &userAction)
		discoveries = append(discoveries, map[string]any{
			"id": id, "provider_id": providerID, "model_id": modelID,
			"first_seen_at": firstSeen, "user_action": userAction,
		})
	}
	if discoveries == nil {
		discoveries = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"discoveries": discoveries})
}

func (gw *Gateway) handleActionDiscoveredModel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	id := chi.URLParam(r, "id")
	action := chi.URLParam(r, "action") // "enable" | "dismiss"
	if action != "enable" && action != "dismiss" {
		writeJSON(w, 400, map[string]string{"error": "action must be 'enable' or 'dismiss'"})
		return
	}
	userAction := action + "d"
	gw.db.Pool.Exec(r.Context(),
		`UPDATE model_discoveries SET user_action = $1 WHERE id = $2 AND tenant_id = $3`,
		userAction, id, defaultTenant)

	if action == "enable" {
		var providerID, modelID string
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT provider_id, model_id FROM model_discoveries WHERE id = $1`, id).
			Scan(&providerID, &modelID)
		if providerID != "" && modelID != "" {
			gw.db.Pool.Exec(r.Context(),
				`INSERT INTO selected_models (tenant_id, provider_id, model_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				defaultTenant, providerID, modelID)
		}
	}
	w.WriteHeader(204)
}

func (gw *Gateway) handleGetSearchProviders(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []any{})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT provider_type, config FROM provider_configs
		 WHERE tenant_id = $1 AND provider_type LIKE 'search_%'`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	providers := []map[string]any{}
	for rows.Next() {
		var ptype, cfgJSON string
		rows.Scan(&ptype, &cfgJSON)
		var cfg map[string]any
		json.Unmarshal([]byte(cfgJSON), &cfg)
		slug := strings.TrimPrefix(ptype, "search_")
		entry := map[string]any{"slug": slug, "provider_type": ptype}
		if key, ok := cfg["api_key"].(string); ok && key != "" {
			entry["configured"] = true
			entry["key_hint"] = maskKey(key)
		} else {
			entry["configured"] = false
		}
		providers = append(providers, entry)
	}
	if providers == nil {
		providers = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"providers": providers})
}

func (gw *Gateway) handleSaveSearchProvider(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Slug   string `json:"slug"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Slug == "" {
		writeJSON(w, 400, map[string]string{"error": "slug and api_key required"})
		return
	}
	cfgJSON, _ := json.Marshal(map[string]string{"api_key": body.APIKey})
	_, err := gw.db.Pool.Exec(r.Context(),
		`INSERT INTO provider_configs (tenant_id, provider_type, config) VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, provider_type) DO UPDATE SET config = $3`,
		defaultTenant, "search_"+body.Slug, string(cfgJSON))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}

// ─── Intelligence Integrations (LLM Stats + Artificial Analysis) ─────────────
//
// Keys are stored in provider_configs under well-known type slugs and
// encrypted with the same AES-256 key used for LLM provider API keys.
// The GET endpoint returns only a masked hint — the raw key is never sent
// back to the browser.

const (
	integrationLLMStats           = "integration_llmstats"
	integrationArtificialAnalysis = "integration_artificialanalysis"
)

var integrationCatalog = []map[string]string{
	{
		"id":      "llmstats",
		"name":    "LLM Stats",
		"desc":    "Live pricing, context windows and modality data for the model catalog.",
		"key_url": "https://llm-stats.com",
	},
	{
		"id":      "artificialanalysis",
		"name":    "Artificial Analysis",
		"desc":    "Model benchmark scores, speed metrics and quality rankings.",
		"key_url": "https://artificialanalysis.ai",
	},
}

func (gw *Gateway) handleGetIntegrations(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Desc       string `json:"desc"`
		KeyURL     string `json:"key_url"`
		Configured bool   `json:"configured"`
		KeyHint    string `json:"key_hint,omitempty"`
	}
	out := make([]entry, 0, len(integrationCatalog))
	for _, meta := range integrationCatalog {
		e := entry{
			ID:     meta["id"],
			Name:   meta["name"],
			Desc:   meta["desc"],
			KeyURL: meta["key_url"],
		}
		if gw.db != nil {
			var cfgJSON string
			err := gw.db.Pool.QueryRow(r.Context(),
				`SELECT config FROM provider_configs WHERE tenant_id = $1 AND provider_type = $2`,
				defaultTenant, "integration_"+meta["id"],
			).Scan(&cfgJSON)
			if err == nil && cfgJSON != "" {
				var cfg map[string]any
				if json.Unmarshal([]byte(cfgJSON), &cfg) == nil {
					if enc, ok := cfg["api_key_enc"].(string); ok && enc != "" {
						plain, decErr := gw.decryptIntegrationKey(enc)
						if decErr == nil && plain != "" {
							e.Configured = true
							e.KeyHint = maskKey(plain)
						}
					}
				}
			}
		}
		out = append(out, e)
	}
	writeJSON(w, 200, map[string]any{"integrations": out})
}

func (gw *Gateway) handleSaveIntegration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || body.APIKey == "" {
		writeJSON(w, 400, map[string]string{"error": "id and api_key required"})
		return
	}

	// Validate id is one of the known integrations.
	known := false
	for _, meta := range integrationCatalog {
		if meta["id"] == body.ID {
			known = true
			break
		}
	}
	if !known {
		writeJSON(w, 400, map[string]string{"error": "unknown integration id"})
		return
	}

	enc, err := gw.encryptIntegrationKey(body.APIKey)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "encrypt failed"})
		return
	}
	cfgJSON, _ := json.Marshal(map[string]string{"api_key_enc": enc})

	if gw.db != nil {
		_, err = gw.db.Pool.Exec(r.Context(),
			`INSERT INTO provider_configs (tenant_id, provider_type, config) VALUES ($1, $2, $3)
			 ON CONFLICT (tenant_id, provider_type) DO UPDATE SET config = EXCLUDED.config`,
			defaultTenant, "integration_"+body.ID, string(cfgJSON))
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}

	// Hot-reload the in-process client so the next refresh cycle picks up
	// the new key without requiring a server restart.
	switch body.ID {
	case "artificialanalysis":
		gw.aaClient = artificialanalysis.New(body.APIKey)
	case "llmstats":
		// llmstats client is local to the boot goroutine; restart the loop.
		gw.startLLMStatsLoop(body.APIKey, 24)
	}

	w.WriteHeader(204)
}

// loadIntegrationKey reads and decrypts a stored integration API key from
// provider_configs. Returns "" when not set, so callers can fall back to
// the config.toml value.
func (gw *Gateway) loadIntegrationKey(ctx context.Context, id string) string {
	if gw.db == nil {
		return ""
	}
	var cfgJSON string
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT config FROM provider_configs WHERE tenant_id = $1 AND provider_type = $2`,
		defaultTenant, "integration_"+id,
	).Scan(&cfgJSON)
	if err != nil || cfgJSON == "" {
		return ""
	}
	var cfg map[string]any
	if json.Unmarshal([]byte(cfgJSON), &cfg) != nil {
		return ""
	}
	enc, _ := cfg["api_key_enc"].(string)
	if enc == "" {
		return ""
	}
	plain, _ := gw.decryptIntegrationKey(enc)
	return plain
}

// startLLMStatsLoop launches (or re-launches) the LLM Stats background
// refresh goroutine. Called at startup and on hot-reload via
// handleSaveIntegration.
func (gw *Gateway) startLLMStatsLoop(apiKey string, hours int) {
	if apiKey == "" || gw.db == nil {
		return
	}
	// Stop any previously running loop before spawning a new one.
	if gw.llmStatsStop != nil {
		close(gw.llmStatsStop)
	}
	stop := make(chan struct{})
	gw.llmStatsStop = stop

	client := llmstats.New(apiKey)
	maxAge := time.Duration(hours) * time.Hour
	go func() {
		bgCtx := context.Background()
		llmstats.RefreshAndMerge(bgCtx, client, gw.db.Pool, maxAge)
		gw.seedModelDefaults(bgCtx)
		t := time.NewTicker(maxAge)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				llmstats.RefreshAndMerge(bgCtx, client, gw.db.Pool, maxAge)
				gw.seedModelDefaults(bgCtx)
			}
		}
	}()
}

// seedModelDefaults populates category_model_assignments with catalog-derived
// defaults for the default tenant. No-op if SmartRouter is not wired or
// the catalog has no providers available. Safe to call multiple times.
func (gw *Gateway) seedModelDefaults(ctx context.Context) {
	if gw.agentLoop == nil || gw.agentLoop.SmartRouter == nil {
		return
	}
	gw.agentLoop.SmartRouter.SeedDefaultAssignments(ctx, defaultTenant)
}

// encryptIntegrationKey encrypts an API key using the gateway's
// encryption key (same AES-256 as provider keys). Falls back to
// storing plain text when no encryption key is configured.
func (gw *Gateway) encryptIntegrationKey(plain string) (string, error) {
	if gw.cfg == nil || gw.cfg.Auth.EncryptionKey == "" {
		return plain, nil
	}
	enc, err := crypto.Encrypt([]byte(plain), gw.cfg.Auth.EncryptionKey)
	if err != nil {
		return "", err
	}
	return string(enc), nil
}

func (gw *Gateway) decryptIntegrationKey(enc string) (string, error) {
	if gw.cfg == nil || gw.cfg.Auth.EncryptionKey == "" {
		return enc, nil
	}
	plain, err := crypto.Decrypt([]byte(enc), gw.cfg.Auth.EncryptionKey)
	if err != nil {
		return enc, nil // fallback: treat as plain text (legacy unencrypted)
	}
	return string(plain), nil
}

func (gw *Gateway) handleRoutingSuggestions(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"suggestions": []any{}, "category": r.URL.Query().Get("category")})
		return
	}
	category := r.URL.Query().Get("category")
	router := providers.NewSmartRouter(gw.db.Pool)
	scores, err := router.GetModelScores(r.Context(), defaultTenant, category)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"suggestions": scores, "category": category})
}

// handleModelRankings returns the Artificial Analysis ranked model list
// from the Postgres cache. When no API key is configured it returns an
// empty list with a setup prompt so the UI can show a callout.
//
// Response shape:
//
//	{
//	  "models":    [...],
//	  "source":    "artificial_analysis",
//	  "fetched_at": "RFC3339",   // omitted when no data
//	  "configured": true|false,
//	  "key_url":   "https://artificialanalysis.ai"
//	}
func (gw *Gateway) handleModelRankings(w http.ResponseWriter, r *http.Request) {
	const keyURL = "https://artificialanalysis.ai"

	if gw.aaClient == nil || gw.db == nil {
		writeJSON(w, 200, map[string]any{
			"models":     []any{},
			"source":     "artificial_analysis",
			"configured": false,
			"key_url":    keyURL,
		})
		return
	}

	models, fetchedAt, err := artificialanalysis.GetRankedModels(r.Context(), gw.db.Pool)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if len(models) == 0 {
		// Key configured but cache not yet populated (first boot still running).
		writeJSON(w, 200, map[string]any{
			"models":     []any{},
			"source":     "artificial_analysis",
			"configured": true,
			"key_url":    keyURL,
		})
		return
	}
	writeJSON(w, 200, map[string]any{
		"models":     models,
		"source":     "artificial_analysis",
		"fetched_at": fetchedAt.UTC().Format(time.RFC3339),
		"configured": true,
		"key_url":    keyURL,
	})
}

func (gw *Gateway) handleExportTrajectory(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		w.Header().Set("Content-Type", "application/jsonl")
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	format := r.URL.Query().Get("format") // sharegpt, jsonl
	if format == "" {
		format = "sharegpt"
	}

	var sessions []*session.Session
	if sessionID != "" {
		sess, err := gw.sessions.GetByID(r.Context(), sessionID)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "session not found"})
			return
		}
		sessions = []*session.Session{sess}
	} else {
		var err error
		sessions, err = gw.sessions.List(r.Context(), defaultTenant, "", 100)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}

	w.Header().Set("Content-Type", "application/jsonl")
	w.Header().Set("Content-Disposition", "attachment; filename=trajectories.jsonl")

	enc := json.NewEncoder(w)
	for _, sess := range sessions {
		var msgs []struct{ Role, Content string }
		json.Unmarshal(sess.Messages, &msgs)
		if len(msgs) < 2 {
			continue
		}

		if format == "sharegpt" {
			// ShareGPT format: {"conversations": [{"from":"human","value":"..."}, {"from":"gpt","value":"..."}]}
			convs := []map[string]string{}
			for _, m := range msgs {
				from := "human"
				if m.Role == "assistant" {
					from = "gpt"
				}
				if m.Role == "system" {
					from = "system"
				}
				if m.Content != "" {
					convs = append(convs, map[string]string{"from": from, "value": m.Content})
				}
			}
			enc.Encode(map[string]any{"conversations": convs})
		} else {
			// JSONL: one message per line
			for _, m := range msgs {
				enc.Encode(m)
			}
		}
	}
}
