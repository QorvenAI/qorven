// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ModelMeta is enriched model metadata for the UI.
type ModelMeta struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Provider     string   `json:"provider"`
	Description  string   `json:"description,omitempty"`
	ContextWindow int    `json:"context_window,omitempty"`
	InputCost    float64  `json:"input_cost_per_1m,omitempty"`
	OutputCost   float64  `json:"output_cost_per_1m,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"` // vision, tools, reasoning, streaming
	Tier         string   `json:"tier,omitempty"`         // free, cheap, standard, premium
	IsNew        bool     `json:"is_new,omitempty"`
	IsTrending   bool     `json:"is_trending,omitempty"`
	IsRecommended bool   `json:"is_recommended,omitempty"`
}

// ModelCatalog manages enriched model metadata with caching.
type ModelCatalog struct {
	mu       sync.RWMutex
	models   []ModelMeta
	fetchedAt time.Time
	ttl      time.Duration
}

var globalCatalog = &ModelCatalog{ttl: 24 * time.Hour}

// GetModelCatalog returns the enriched model catalog, refreshing if stale.
func GetModelCatalog(ctx context.Context) []ModelMeta {
	globalCatalog.mu.RLock()
	if time.Since(globalCatalog.fetchedAt) < globalCatalog.ttl && len(globalCatalog.models) > 0 {
		defer globalCatalog.mu.RUnlock()
		return globalCatalog.models
	}
	globalCatalog.mu.RUnlock()

	// Refresh
	models := buildCatalog(ctx)
	globalCatalog.mu.Lock()
	globalCatalog.models = models
	globalCatalog.fetchedAt = time.Now()
	globalCatalog.mu.Unlock()
	return models
}

func buildCatalog(ctx context.Context) []ModelMeta {
	// Start with curated recommendations
	models := curatedModels()

	// Try to fetch from OpenRouter for pricing + capabilities
	orModels, err := fetchOpenRouterCatalog(ctx)
	if err == nil && len(orModels) > 0 {
		// Merge OpenRouter data into our catalog
		orMap := make(map[string]ModelMeta)
		for _, m := range orModels {
			orMap[m.ID] = m
		}
		// Enrich curated models with pricing
		for i, m := range models {
			if or, ok := orMap[m.ID]; ok {
				if m.InputCost == 0 { models[i].InputCost = or.InputCost }
				if m.OutputCost == 0 { models[i].OutputCost = or.OutputCost }
				if m.ContextWindow == 0 { models[i].ContextWindow = or.ContextWindow }
				if len(m.Capabilities) == 0 { models[i].Capabilities = or.Capabilities }
			}
		}
		// Add trending OpenRouter models not in curated list
		existing := make(map[string]bool)
		for _, m := range models { existing[m.ID] = true }
		for _, m := range orModels {
			if !existing[m.ID] && m.IsTrending {
				models = append(models, m)
			}
		}
	}

	slog.Info("model_catalog.built", "total", len(models))
	return models
}

func fetchOpenRouterCatalog(ctx context.Context) ([]ModelMeta, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, nil }

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Context     int    `json:"context_length"`
			Pricing     struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			Architecture struct {
				Modality string `json:"modality"`
			} `json:"architecture"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	json.Unmarshal(body, &result)

	models := []ModelMeta{}
	for _, m := range result.Data {
		caps := []string{}
		if strings.Contains(m.Architecture.Modality, "image") { caps = append(caps, "vision") }
		caps = append(caps, "tools", "streaming")

		tier := "standard"
		if m.Pricing.Prompt == "0" { tier = "free" }

		models = append(models, ModelMeta{
			ID: m.ID, Name: m.Name, Provider: strings.Split(m.ID, "/")[0],
			Description: m.Description, ContextWindow: m.Context,
			Capabilities: caps, Tier: tier,
		})
	}
	return models, nil
}

// curatedModels returns hand-picked recommended models.
func curatedModels() []ModelMeta {
	return []ModelMeta{
		// Recommended for Qorven
		{ID: "anthropic/claude-sonnet-4-5", Name: "Claude Sonnet 4.5", Provider: "anthropic", Description: "Best balance of quality and cost for agents", ContextWindow: 200000, InputCost: 3, OutputCost: 15, Capabilities: []string{"vision", "tools", "reasoning", "streaming"}, Tier: "premium", IsRecommended: true},
		{ID: "openai/gpt-4o", Name: "GPT-4o", Provider: "openai", Description: "Fast, multimodal, great for tools", ContextWindow: 128000, InputCost: 2.5, OutputCost: 10, Capabilities: []string{"vision", "tools", "streaming"}, Tier: "standard", IsRecommended: true},
		{ID: "google/gemini-2.5-flash", Name: "Gemini 2.5 Flash", Provider: "google", Description: "Fastest, cheapest with thinking", ContextWindow: 1000000, InputCost: 0.15, OutputCost: 0.6, Capabilities: []string{"vision", "tools", "reasoning", "streaming"}, Tier: "cheap", IsRecommended: true},
		{ID: "deepseek/deepseek-chat", Name: "DeepSeek V3", Provider: "deepseek", Description: "Strong reasoning, very affordable", ContextWindow: 64000, InputCost: 0.14, OutputCost: 0.28, Capabilities: []string{"tools", "streaming"}, Tier: "cheap", IsRecommended: true},
		{ID: "meta-llama/llama-3.3-70b-instruct:free", Name: "Llama 3.3 70B", Provider: "meta", Description: "Best free open-source model", ContextWindow: 131072, Capabilities: []string{"tools", "streaming"}, Tier: "free", IsRecommended: true},
		{ID: "qwen/qwen3-235b", Name: "Qwen3 235B", Provider: "qwen", Description: "Largest open model, strong reasoning", ContextWindow: 128000, InputCost: 0.14, OutputCost: 0.28, Capabilities: []string{"tools", "reasoning", "streaming"}, Tier: "cheap", IsRecommended: true},

		// Popular models
		{ID: "anthropic/claude-opus-4-5", Name: "Claude Opus 4.5", Provider: "anthropic", Description: "Most capable, highest quality", ContextWindow: 200000, InputCost: 15, OutputCost: 75, Capabilities: []string{"vision", "tools", "reasoning", "streaming"}, Tier: "premium"},
		{ID: "openai/o1", Name: "OpenAI o1", Provider: "openai", Description: "Advanced reasoning model", ContextWindow: 200000, InputCost: 15, OutputCost: 60, Capabilities: []string{"reasoning"}, Tier: "premium"},
		{ID: "openai/gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai", Description: "Fast and cheap for simple tasks", ContextWindow: 128000, InputCost: 0.15, OutputCost: 0.6, Capabilities: []string{"vision", "tools", "streaming"}, Tier: "cheap"},
		{ID: "google/gemini-2.0-flash", Name: "Gemini 2.0 Flash", Provider: "google", Description: "Previous gen, still fast", ContextWindow: 1000000, InputCost: 0.1, OutputCost: 0.4, Capabilities: []string{"vision", "tools", "streaming"}, Tier: "cheap"},
		{ID: "groq/llama-3.3-70b-versatile", Name: "Llama 3.3 70B (Groq)", Provider: "groq", Description: "Ultra-fast inference via Groq", ContextWindow: 131072, Capabilities: []string{"tools", "streaming"}, Tier: "cheap"},
		{ID: "mistral/mistral-large-latest", Name: "Mistral Large", Provider: "mistral", Description: "Strong European model", ContextWindow: 128000, InputCost: 2, OutputCost: 6, Capabilities: []string{"tools", "streaming"}, Tier: "standard"},

		// Bedrock models (for our setup)
		{ID: "kimi-k2.5", Name: "Kimi K2.5", Provider: "moonshot", Description: "Via AWS Bedrock", ContextWindow: 128000, Capabilities: []string{"tools", "streaming"}, Tier: "standard"},
		{ID: "deepseek-v3.2", Name: "DeepSeek V3.2", Provider: "deepseek", Description: "Via AWS Bedrock", ContextWindow: 64000, Capabilities: []string{"tools", "streaming"}, Tier: "cheap"},
		{ID: "qwen3-235b", Name: "Qwen3 235B", Provider: "qwen", Description: "Via AWS Bedrock", ContextWindow: 128000, Capabilities: []string{"tools", "reasoning", "streaming"}, Tier: "cheap"},
	}
}

// GetRecommended returns the curated recommended models.
func GetRecommended() []ModelMeta {
	rec := []ModelMeta{}
	for _, m := range curatedModels() {
		if m.IsRecommended { rec = append(rec, m) }
	}
	return rec
}
