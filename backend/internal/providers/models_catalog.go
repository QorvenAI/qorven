// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

// ─── Static Model Catalog ──────────────────────────────────────────────────
//
// models_catalog.json is generated from model_registry.go + curated metadata
// (tier, category strengths, display names). It ships embedded in the
// binary so fresh installs have a working catalog with zero external
// dependencies. The LLM Stats integration (see llmstats package) can
// enrich this at runtime with live benchmark data.

//go:embed models_catalog.json
var modelsCatalogJSON []byte

// ModelCatalogEntry is one row in the static catalog. The JSON shape
// matches the "Phase F" specification from INTEGRATION prompts — keep
// the field names stable; the web UI consumes them directly.
type ModelCatalogEntry struct {
	ID               string            `json:"id"`
	Provider         string            `json:"provider"`
	DisplayName      string            `json:"display_name"`
	ContextWindow    int               `json:"context_window"`
	MaxOutput        int               `json:"max_output"`
	Pricing          CatalogPricing    `json:"pricing"`
	Capabilities     []string          `json:"capabilities"`
	Tier             string            `json:"tier"`
	CategoryStrengths []string         `json:"category_strengths"`
	Released         string            `json:"released,omitempty"`

	// Enrichment fields (populated by llmstats, never by the static file).
	BenchmarkScores  map[string]float64 `json:"benchmark_scores,omitempty"`
	OverallRank      int                `json:"overall_rank,omitempty"`
	CategoryRanks    map[string]int     `json:"category_ranks,omitempty"`
	LLMStatsUpdated  string             `json:"llm_stats_updated,omitempty"`
}

type CatalogPricing struct {
	InputPerM  float64 `json:"input_per_m"`
	OutputPerM float64 `json:"output_per_m"`
}

// Valid tier IDs. Smart Router maps classification → tier → best model.
const (
	TierSimple    = "simple"    // fast / cheap
	TierStandard  = "standard"  // balanced
	TierComplex   = "complex"   // high intelligence
	TierReasoning = "reasoning" // deep thinking
	TierCoding    = "coding"    // code specialist
)

// StaticModelCatalog holds the loaded + enriched catalog. Thread-safe.
// Distinct type from the pre-existing ModelCatalog in model_catalog.go
// which has a different internal shape.
type StaticModelCatalog struct {
	mu       sync.RWMutex
	entries  []ModelCatalogEntry
	byID     map[string]*ModelCatalogEntry
}

var (
	staticCatalog     *StaticModelCatalog
	staticCatalogOnce sync.Once
	staticCatalogErr  error
)

// LoadCatalog returns the process-wide catalog, loading from the embedded
// JSON on first call. Subsequent calls return the same instance.
func LoadCatalog() (*StaticModelCatalog, error) {
	staticCatalogOnce.Do(func() {
		c := &StaticModelCatalog{byID: make(map[string]*ModelCatalogEntry)}
		var entries []ModelCatalogEntry
		if err := json.Unmarshal(modelsCatalogJSON, &entries); err != nil {
			staticCatalogErr = err; return
		}
		c.entries = entries
		for i := range c.entries {
			c.byID[c.entries[i].ID] = &c.entries[i]
		}
		staticCatalog = c
	})
	return staticCatalog, staticCatalogErr
}

// GetModelInfo returns catalog data for one model ID, or nil if unknown.
// Safe to call with unknown IDs — callers can fall back to the registry
// or heuristics.
func (c *StaticModelCatalog) GetModelInfo(id string) *ModelCatalogEntry {
	if c == nil { return nil }
	c.mu.RLock()
	defer c.mu.RUnlock()
	if e, ok := c.byID[id]; ok {
		cp := *e
		return &cp
	}
	// Secondary match: some IDs differ only in prefix (us.anthropic.
	// vs bare anthropic.) — try a loose match so Bedrock inference
	// profiles resolve to their foundation counterpart.
	for _, candidate := range looseVariants(id) {
		if e, ok := c.byID[candidate]; ok {
			cp := *e
			return &cp
		}
	}
	return nil
}

// looseVariants returns plausible alternate forms of a model ID so that
// bedrock inference profiles (us.anthropic.*) resolve to their
// foundation-model entry (anthropic.*). Keeps the catalog small without
// duplicating every entry for every regional variant.
func looseVariants(id string) []string {
	var out []string
	if strings.HasPrefix(id, "us.")     { out = append(out, id[3:]) }
	if strings.HasPrefix(id, "global.") { out = append(out, id[7:]) }
	if strings.HasPrefix(id, "eu.")     { out = append(out, id[3:]) }
	return out
}

// ListByTier returns every entry in the given tier, sorted by provider + ID
// so the list is stable for UI consumption.
func (c *StaticModelCatalog) ListByTier(tier string) []ModelCatalogEntry {
	if c == nil { return nil }
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []ModelCatalogEntry
	for _, e := range c.entries {
		if e.Tier == tier { out = append(out, e) }
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider { return out[i].Provider < out[j].Provider }
		return out[i].ID < out[j].ID
	})
	return out
}

// ListByCategory returns entries whose category_strengths include the
// given category, ranked by tier preference (coding/reasoning → higher
// before simple) and then by provider.
func (c *StaticModelCatalog) ListByCategory(category string) []ModelCatalogEntry {
	if c == nil { return nil }
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []ModelCatalogEntry
	for _, e := range c.entries {
		for _, s := range e.CategoryStrengths {
			if s == category { out = append(out, e); break }
		}
	}
	// Preference order mirrors the tier ladder — specialist tiers first
	// for their specialty, then complex/standard/simple. We don't rank
	// by price here; smart router can do that separately.
	tierPref := map[string]int{TierComplex:0, TierReasoning:1, TierCoding:2, TierStandard:3, TierSimple:4}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := tierPref[out[i].Tier], tierPref[out[j].Tier]
		if ti != tj { return ti < tj }
		return out[i].ID < out[j].ID
	})
	return out
}

// ListByProvider returns every entry for a given provider name.
func (c *StaticModelCatalog) ListByProvider(provider string) []ModelCatalogEntry {
	if c == nil { return nil }
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []ModelCatalogEntry
	for _, e := range c.entries {
		if e.Provider == provider { out = append(out, e) }
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// All returns a copy of every entry. Used by the REST endpoint for a
// default list response.
func (c *StaticModelCatalog) All() []ModelCatalogEntry {
	if c == nil { return nil }
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ModelCatalogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}

// MergeEnrichment merges LLM Stats (or any external) data into catalog
// entries. Fields left zero in updates are ignored. Keyed by ID plus
// looseVariants so Bedrock inference profiles inherit from their
// foundation counterpart when LLM Stats only has the bare ID.
func (c *StaticModelCatalog) MergeEnrichment(updates map[string]ModelCatalogEntry) int {
	if c == nil { return 0 }
	c.mu.Lock()
	defer c.mu.Unlock()
	merged := 0
	for i := range c.entries {
		e := &c.entries[i]
		candidates := append([]string{e.ID}, looseVariants(e.ID)...)
		for _, cand := range candidates {
			u, ok := updates[cand]
			if !ok { continue }
			if u.BenchmarkScores != nil { e.BenchmarkScores = u.BenchmarkScores }
			if u.OverallRank > 0       { e.OverallRank = u.OverallRank }
			if u.CategoryRanks != nil  { e.CategoryRanks = u.CategoryRanks }
			if u.LLMStatsUpdated != "" { e.LLMStatsUpdated = u.LLMStatsUpdated }
			merged += 1
			break
		}
	}
	return merged
}

// tierLadder returns the preferred fallback order starting at the given
// tier — used by smart router when the requested tier has no available
// model. Unspecialised tiers fall back to standard, specialist tiers
// fall back to complex (which handles most tasks well).
func tierLadder(start string) []string {
	switch start {
	case TierCoding:    return []string{TierCoding, TierComplex, TierStandard, TierSimple}
	case TierReasoning: return []string{TierReasoning, TierComplex, TierStandard, TierSimple}
	case TierComplex:   return []string{TierComplex, TierStandard, TierSimple}
	case TierSimple:    return []string{TierSimple, TierStandard, TierComplex}
	default:            return []string{TierStandard, TierComplex, TierSimple}
	}
}

// BestForTier picks the best model in the given tier that's also
// available on one of the listed provider names.
//
// Selection order:
//  1. If any candidate has AA/LLMStats benchmark scores, pick the highest
//     intelligence_index — subject to a cost guard: a benchmark winner is
//     only preferred over the cheapest if its output price is ≤ 3× the
//     cheapest option in the same tier. This prevents a marginally better
//     score from selecting a 10× more expensive model.
//  2. If no benchmark data is present, fall back to cheapest output price
//     (original behaviour).
func (c *StaticModelCatalog) BestForTier(tier string, availableProviders []string) *ModelCatalogEntry {
	if c == nil { return nil }
	avail := make(map[string]struct{})
	for _, p := range availableProviders { avail[p] = struct{}{} }
	c.mu.RLock()
	defer c.mu.RUnlock()

	var candidates []*ModelCatalogEntry
	for i := range c.entries {
		e := &c.entries[i]
		if e.Tier != tier { continue }
		if _, ok := avail[e.Provider]; !ok { continue }
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 { return nil }

	// Find cheapest for the cost-guard baseline.
	var cheapest *ModelCatalogEntry
	for _, e := range candidates {
		if cheapest == nil || e.Pricing.OutputPerM < cheapest.Pricing.OutputPerM {
			cheapest = e
		}
	}

	// Try benchmark-driven selection: prefer highest intelligence_index
	// within the 3× cost guard.
	const costGuardMultiplier = 3.0
	maxAllowedCost := cheapest.Pricing.OutputPerM * costGuardMultiplier
	var benchBest *ModelCatalogEntry
	var benchBestScore float64
	for _, e := range candidates {
		if len(e.BenchmarkScores) == 0 { continue }
		score, ok := e.BenchmarkScores["intelligence_index"]
		if !ok { continue }
		if e.Pricing.OutputPerM > maxAllowedCost { continue }
		if benchBest == nil || score > benchBestScore {
			benchBest = e
			benchBestScore = score
		}
	}
	if benchBest != nil {
		cp := *benchBest
		return &cp
	}

	// No benchmark data — cheapest wins.
	cp := *cheapest
	return &cp
}
