// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
)

// AliasCategory groups models for alias resolution.
type AliasCategory string

const (
	CategoryFast      AliasCategory = "fast"
	CategoryPowerful  AliasCategory = "powerful"
	CategoryAny       AliasCategory = "any"
	CategoryVision    AliasCategory = "vision"
	CategoryCoding    AliasCategory = "coding"
	CategoryReasoning AliasCategory = "reasoning"
)

// builtinAliasCategories maps alias name → which category to look for.
var builtinAliasCategories = map[string]AliasCategory{
	"fast":   CategoryFast,
	"smart":  CategoryPowerful,
	"cheap":  CategoryAny,
	"vision": CategoryVision,
	"code":   CategoryCoding,
	"reason": CategoryReasoning,
}

// modelCategoryHints maps model-id prefixes/keywords to their categories.
// Used to classify models from the provider registry.
var modelCategoryHints = []struct {
	keyword  string
	category AliasCategory
}{
	{"flash", CategoryFast},
	{"haiku", CategoryFast},
	{"mini", CategoryFast},
	{"llama", CategoryFast},
	{"groq", CategoryFast},
	{"opus", CategoryPowerful},
	{"sonnet", CategoryPowerful},
	{"gpt-4o", CategoryPowerful},
	{"gemini-2.5-pro", CategoryPowerful},
	{"o1", CategoryReasoning},
	{"o3", CategoryReasoning},
	{"deepseek-reasoner", CategoryReasoning},
	{"r1", CategoryReasoning},
	{"thinking", CategoryReasoning},
	{"vision", CategoryVision},
}

// AliasResolverImpl resolves model aliases ("fast", "smart", etc.) to
// concrete model IDs. It checks the DB for admin overrides first, then
// falls back to auto-picking the cheapest available model in the category.
type AliasResolverImpl struct {
	db       *pgxpool.Pool
	reg      *providers.Registry
	tenantID string

	mu        sync.RWMutex
	overrides map[string]string // alias → model_id (from DB)
	loadedAt  time.Time
}

// NewAliasResolver creates an AliasResolverImpl. The resolver refreshes
// DB overrides every 5 minutes.
func NewAliasResolver(db *pgxpool.Pool, reg *providers.Registry, tenantID string) *AliasResolverImpl {
	return &AliasResolverImpl{
		db:        db,
		reg:       reg,
		tenantID:  tenantID,
		overrides: make(map[string]string),
	}
}

// Resolve expands an alias in req.Model to a concrete model ID. If the
// model is not a known alias the call is a no-op.
func (r *AliasResolverImpl) Resolve(ctx context.Context, req *GatewayRequest) error {
	_, isAlias := builtinAliasCategories[req.Model]
	if !isAlias {
		return nil
	}

	// Refresh DB overrides at most every 5 minutes.
	r.mu.RLock()
	stale := time.Since(r.loadedAt) > 5*time.Minute
	r.mu.RUnlock()
	if stale {
		r.refreshOverrides(ctx)
	}

	r.mu.RLock()
	override, hasOverride := r.overrides[req.Model]
	r.mu.RUnlock()

	if hasOverride && override != "" {
		// Verify the override model is still reachable; fall through on failure.
		if r.reg.HasModel(override) {
			req.Model = override
			return nil
		}
		slog.Warn("gateway.aliases: DB override model not found, falling back", "alias", req.Model, "override", override)
	}

	// Auto-pick: cheapest available model in the alias category.
	category := builtinAliasCategories[req.Model]
	resolved := r.autoPick(category)
	if resolved != "" {
		req.Model = resolved
	}
	// If no model found, leave req.Model as-is and let provider selection
	// surface the error with a clearer message.
	return nil
}

func (r *AliasResolverImpl) autoPick(category AliasCategory) string {
	// Iterate all registered providers; for each, query the registry's
	// canonical model list. Pick the cheapest model in the category that
	// has a live provider.
	bestModel := ""
	bestCost := -1.0

	pricingMu.RLock()
	snapshot := make(map[string]ModelPricing, len(pricingTable))
	for k, v := range pricingTable {
		snapshot[k] = v
	}
	pricingMu.RUnlock()

	for modelID := range snapshot {
		if !modelMatchesCategory(modelID, category) {
			continue
		}
		if r.reg.ProviderForModel(modelID) == nil {
			continue
		}
		pricing := snapshot[modelID]
		cost := pricing.InputPer1M
		if bestModel == "" || cost < bestCost {
			bestModel = modelID
			bestCost = cost
		}
	}
	return bestModel
}

func modelMatchesCategory(modelID string, category AliasCategory) bool {
	if category == CategoryAny {
		return true
	}
	for _, hint := range modelCategoryHints {
		if contains(modelID, hint.keyword) && hint.category == category {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		(len(s) > 0 && len(sub) > 0 && func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}()))
}

func (r *AliasResolverImpl) refreshOverrides(ctx context.Context) {
	if r.db == nil {
		return
	}
	rows, err := r.db.Query(ctx,
		`SELECT alias, model_id FROM model_aliases WHERE tenant_id = $1 ORDER BY priority DESC`,
		r.tenantID)
	if err != nil {
		return
	}
	defer rows.Close()

	overrides := make(map[string]string)
	for rows.Next() {
		var alias, modelID string
		if rows.Scan(&alias, &modelID) == nil {
			overrides[alias] = modelID
		}
	}

	r.mu.Lock()
	r.overrides = overrides
	r.loadedAt = time.Now()
	r.mu.Unlock()
}
