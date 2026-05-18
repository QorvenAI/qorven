// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"
	"sort"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// handleModelRegistry returns the full model registry with context windows, costs, and capabilities.
// GET /v1/providers/model-registry?provider=openai&search=gpt
//
// Data source priority:
//  1. Enriched StaticModelCatalog (loaded from embedded JSON + LLM Stats live merge at startup)
//  2. Hardcoded ModelRegistry map (LiteLLM-generated, ~1800 entries) for models absent from catalog
//
// The static catalog carries llm_stats_updated timestamps when enriched; those
// entries show live pricing and context windows from the LLM Stats API.
func (gw *Gateway) handleModelRegistry(w http.ResponseWriter, r *http.Request) {
	providerFilter := r.URL.Query().Get("provider")
	search := strings.ToLower(r.URL.Query().Get("search"))

	type modelEntry struct {
		Name        string  `json:"name"`
		MaxInput    int     `json:"max_input_tokens"`
		MaxOutput   int     `json:"max_output_tokens"`
		InputCost   float64 `json:"input_cost_per_1m"`
		OutputCost  float64 `json:"output_cost_per_1m"`
		Tools       bool    `json:"supports_tools"`
		Vision      bool    `json:"supports_vision"`
		Reasoning   bool    `json:"supports_reasoning"`
		Provider    string  `json:"provider"`
		Enriched    bool    `json:"llm_stats_enriched,omitempty"`
		EnrichedAt  string  `json:"llm_stats_updated,omitempty"`
	}

	matches := func(name, provider string) bool {
		if providerFilter != "" && !strings.EqualFold(provider, providerFilter) {
			return false
		}
		if search != "" {
			hay := strings.ToLower(name + " " + provider)
			if !strings.Contains(hay, search) {
				return false
			}
		}
		return true
	}

	hasCap := func(caps []string, cap string) bool {
		for _, c := range caps {
			if c == cap {
				return true
			}
		}
		return false
	}

	// Build result set, catalog-first.
	seen := make(map[string]struct{})
	models := []modelEntry{}

	cat, err := providers.LoadCatalog()
	if err == nil && cat != nil {
		for _, e := range cat.All() {
			if !matches(e.ID, e.Provider) {
				continue
			}
			seen[e.ID] = struct{}{}
			models = append(models, modelEntry{
				Name:       e.ID,
				MaxInput:   e.ContextWindow,
				InputCost:  e.Pricing.InputPerM,
				OutputCost: e.Pricing.OutputPerM,
				Tools:      hasCap(e.Capabilities, "function_calling") || hasCap(e.Capabilities, "tools"),
				Vision:     hasCap(e.Capabilities, "vision"),
				Reasoning:  hasCap(e.Capabilities, "reasoning") || e.Tier == "reasoning",
				Provider:   e.Provider,
				Enriched:   e.LLMStatsUpdated != "",
				EnrichedAt: e.LLMStatsUpdated,
			})
		}
	}

	// Supplement with ModelRegistry entries not already covered by the catalog.
	for name, spec := range providers.ModelRegistry {
		if _, ok := seen[name]; ok {
			continue
		}
		if !matches(name, spec.Provider) {
			continue
		}
		models = append(models, modelEntry{
			Name:       name,
			MaxInput:   spec.MaxInputTokens,
			MaxOutput:  spec.MaxOutputTokens,
			InputCost:  spec.InputCostPer1M,
			OutputCost: spec.OutputCostPer1M,
			Tools:      spec.SupportsTools,
			Vision:     spec.SupportsVision,
			Reasoning:  spec.SupportsReasoning,
			Provider:   spec.Provider,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].Name < models[j].Name
	})

	writeJSON(w, 200, map[string]any{"models": models, "total": len(models)})
}
