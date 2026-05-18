// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"encoding/json"
	"fmt"
)

// ── Providers ─────────────────────────────────────────────────────────────────

type ProviderInfo struct {
	ID      string // truncated for display
	FullID  string // full UUID for API calls
	Name    string
	Type    string
	APIBase string
	Enabled string
}

func (a *apiClient) listProviders() []ProviderInfo {
	data, err := a.http.Get("/v1/providers")
	if err != nil {
		return nil
	}
	var resp struct {
		Providers []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			ProviderType string `json:"provider_type"`
			APIBase      string `json:"api_base"`
			Enabled      bool   `json:"enabled"`
		} `json:"providers"`
	}
	json.Unmarshal(data, &resp)
	var providers []ProviderInfo
	for _, p := range resp.Providers {
		enabled := "no"
		if p.Enabled {
			enabled = "yes"
		}
		displayID := p.ID
		if len(displayID) > 8 {
			displayID = displayID[:8]
		}
		providers = append(providers, ProviderInfo{
			ID: displayID, FullID: p.ID, Name: p.Name, Type: p.ProviderType,
			APIBase: p.APIBase, Enabled: enabled,
		})
	}
	return providers
}

func (a *apiClient) listProviderCatalog() []string {
	data, err := a.http.Get("/v1/providers/catalog")
	if err != nil {
		return nil
	}
	var catalog []struct {
		ID       string `json:"id"`
		Category string `json:"category"`
	}
	if json.Unmarshal(data, &catalog) != nil {
		return nil
	}
	llmCategories := map[string]bool{"cloud": true, "openai_compat": true, "local": true, "enterprise": true, "custom": true}
	var ids []string
	for _, p := range catalog {
		if llmCategories[p.Category] {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// ── Key pool ──────────────────────────────────────────────────────────────────

type PoolInfo struct {
	Strategy     string        `json:"strategy"`
	FailoverMode string        `json:"failover_mode"`
	Keys         []PoolKeyInfo `json:"keys,omitempty"`
}

type PoolKeyInfo struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Status       string  `json:"status"`
	UsageCount   int     `json:"usage_count"`
	BudgetUSD    float64 `json:"budget_usd_monthly"`
	SpentUSD     float64 `json:"spent_usd_month"`
	BudgetTokens int64   `json:"budget_tokens_monthly"`
	SpentTokens  int64   `json:"spent_tokens_month"`
}

type DiscoveredModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
}

func (a *apiClient) getPoolConfig(providerID string) PoolInfo {
	data, err := a.http.Get("/v1/providers/" + providerID + "/pool")
	if err != nil {
		return PoolInfo{Strategy: "priority", FailoverMode: "on_exhaust"}
	}
	var cfg PoolInfo
	json.Unmarshal(data, &cfg)

	keysData, err := a.http.Get("/v1/providers/" + providerID + "/keys")
	if err == nil {
		var records []struct {
			ID                  string  `json:"id"`
			Label               string  `json:"label"`
			Status              string  `json:"status"`
			TotalRequests       int     `json:"total_requests"`
			BudgetUSDMonthly    float64 `json:"budget_usd_monthly"`
			SpentUSDMonth       float64 `json:"spent_usd_month"`
			BudgetTokensMonthly int64   `json:"budget_tokens_monthly"`
			SpentTokensMonth    int64   `json:"spent_tokens_month"`
		}
		if json.Unmarshal(keysData, &records) == nil {
			cfg.Keys = make([]PoolKeyInfo, 0, len(records))
			for _, r := range records {
				cfg.Keys = append(cfg.Keys, PoolKeyInfo{
					ID:           r.ID,
					Label:        r.Label,
					Status:       r.Status,
					UsageCount:   r.TotalRequests,
					BudgetUSD:    r.BudgetUSDMonthly,
					SpentUSD:     r.SpentUSDMonth,
					BudgetTokens: r.BudgetTokensMonthly,
					SpentTokens:  r.SpentTokensMonth,
				})
			}
		}
	}
	return cfg
}

func (a *apiClient) savePoolConfig(providerID, strategy, failoverMode string) error {
	_, err := a.http.Put("/v1/providers/"+providerID+"/pool",
		map[string]string{"strategy": strategy, "failover_mode": failoverMode})
	return err
}

func (a *apiClient) testKey(keyID string) ([]string, error) {
	data, err := a.http.Post("/v1/providers/keys/"+keyID+"/test", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Verified bool   `json:"verified"`
		Error    string `json:"error"`
		Models   []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if json.Unmarshal(data, &resp) != nil || !resp.Verified {
		msg := resp.Error
		if msg == "" {
			msg = "test failed"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	var ids []string
	for _, m := range resp.Models {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func (a *apiClient) deleteProviderKey(keyID string) error {
	_, err := a.http.Delete("/v1/providers/keys/" + keyID)
	return err
}

func (a *apiClient) setKeyBudget(keyID string, budgetUSD float64, budgetTokens int64) error {
	_, err := a.http.Put("/v1/providers/keys/"+keyID+"/budget", map[string]any{
		"budget_usd_monthly":    budgetUSD,
		"budget_tokens_monthly": budgetTokens,
	})
	return err
}

// ── Model hub ─────────────────────────────────────────────────────────────────

type ModelRegistryEntry struct {
	Name      string  `json:"name"`
	MaxInput  int     `json:"max_input_tokens"`
	InputCost float64 `json:"input_cost_per_1m"`
	Tools     bool    `json:"supports_tools"`
	Vision    bool    `json:"supports_vision"`
	Provider  string  `json:"provider"`
}

type ModelHubEntry struct {
	ProviderID   string
	ProviderName string
	ModelID      string
	Selected     bool
	IsDefault    bool
	MaxInput     int
	InputCost    float64
	HasTools     bool
}

type SelectedModelInfo struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	IsDefault  bool   `json:"is_default"`
}

func (a *apiClient) listModelRegistry(search string) []ModelRegistryEntry {
	path := "/v1/providers/model-registry"
	if search != "" {
		path += "?search=" + search
	}
	data, err := a.http.Get(path)
	if err != nil {
		return nil
	}
	var resp struct {
		Models []ModelRegistryEntry `json:"models"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	return resp.Models
}

func (a *apiClient) listSelectedModels() []SelectedModelInfo {
	data, err := a.http.Get("/v1/models/selected")
	if err != nil {
		return nil
	}
	var resp struct {
		Models []SelectedModelInfo `json:"models"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	return resp.Models
}

func (a *apiClient) selectModel(providerID, modelID string) error {
	_, err := a.http.Post("/v1/models/select", map[string]string{
		"provider_id": providerID,
		"model_id":    modelID,
	})
	return err
}

func (a *apiClient) deselectModel(providerID, modelID string) error {
	_, err := a.http.DeleteWithBody("/v1/models/select", map[string]string{
		"provider_id": providerID,
		"model_id":    modelID,
	})
	return err
}

func (a *apiClient) listModelHub() []ModelHubEntry {
	registry := a.listModelRegistry("")
	selected := a.listSelectedModels()

	selectedSet := make(map[string]bool, len(selected))
	defaultSet := make(map[string]bool, len(selected))
	for _, s := range selected {
		selectedSet[s.ProviderID+"/"+s.ModelID] = true
		if s.IsDefault {
			defaultSet[s.ProviderID+"/"+s.ModelID] = true
		}
	}

	providers := a.listProviders()
	providerNames := make(map[string]string, len(providers))
	for _, p := range providers {
		providerNames[p.FullID] = p.Name
	}

	out := make([]ModelHubEntry, 0, len(registry))
	for _, r := range registry {
		key := r.Provider + "/" + r.Name
		name := providerNames[r.Provider]
		if name == "" {
			name = r.Provider
		}
		out = append(out, ModelHubEntry{
			ProviderID:   r.Provider,
			ProviderName: name,
			ModelID:      r.Name,
			Selected:     selectedSet[key],
			IsDefault:    defaultSet[key],
			MaxInput:     r.MaxInput,
			InputCost:    r.InputCost,
			HasTools:     r.Tools,
		})
	}
	return out
}

func (a *apiClient) listDiscoveredModels() []DiscoveredModel {
	data, err := a.http.Get("/v1/models/discovered?unnotified=1")
	if err != nil {
		return nil
	}
	var resp struct {
		Discoveries []DiscoveredModel `json:"discoveries"`
	}
	json.Unmarshal(data, &resp)
	return resp.Discoveries
}

func (a *apiClient) actionDiscoveredModel(id, action string) error {
	// Backend actions are "enable" and "dismiss"; callers use "add" and "ignore".
	backendAction := map[string]string{"add": "enable", "ignore": "dismiss"}[action]
	if backendAction == "" {
		backendAction = action
	}
	_, err := a.http.Post("/v1/models/discovered/"+id+"/"+backendAction, nil)
	return err
}
