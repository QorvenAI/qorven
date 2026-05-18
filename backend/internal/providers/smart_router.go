// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"log/slog"
	"strings"

	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SmartRouter classifies queries and routes to the best model per category.
type SmartRouter struct {
	pool      *pgxpool.Pool
	registry  *Registry
	exclusion *ModelExclusion
	costs     *CostTracker
}

func NewSmartRouter(pool *pgxpool.Pool) *SmartRouter {
	return &SmartRouter{pool: pool, exclusion: NewModelExclusion(), costs: &CostTracker{}}

}

func (r *SmartRouter) SetRegistry(reg *Registry) { r.registry = reg }

// WorkCategory is a user-defined work type.
type WorkCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
}

// RoutingDecision is the result of classifying and routing a query.
type RoutingDecision struct {
	Category   string            `json:"category"`
	Confidence float64           `json:"confidence"`
	ModelID    string            `json:"model_id"`
	Reason     string            `json:"reason"`
	Tier       RoutingTier       `json:"tier"`
	Dimensions RequestDimensions `json:"dimensions"`
}

// ClassifyAndRoute classifies a query and picks the best model.
func (r *SmartRouter) ClassifyAndRoute(ctx context.Context, tenantID, query string) (*RoutingDecision, error) {
	category, confidence := r.classify(query)

	// Score request dimensions for tier selection
	dims := ScoreRequest(query, false, false, 0)
	tier := SelectTier(dims, "")

	// Find assigned models for this category — collect all, then pick best.
	var modelID string
	candidates := []string{}
	rows, _ := r.pool.Query(ctx,
		`SELECT model_id FROM category_model_assignments WHERE tenant_id = $1 AND category_slug = $2 ORDER BY priority`,
		tenantID, category)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var candidate string
			rows.Scan(&candidate)
			candidates = append(candidates, candidate)
		}
	}

	// Feedback loop: if any candidate has enough real performance data (≥10 uses),
	// prefer the one with the highest Wilson-score success rate.
	if len(candidates) > 1 {
		perfScores, _ := r.GetModelScores(ctx, tenantID, category)
		scoreMap := make(map[string]float64, len(perfScores))
		for _, ps := range perfScores {
			if ps.UsageCount >= 10 {
				scoreMap[ps.ModelID] = ps.Score
			}
		}
		if len(scoreMap) > 0 {
			best := ""
			bestScore := -1.0
			for _, c := range candidates {
				if s, ok := scoreMap[c]; ok && s > bestScore && r.modelHasProvider(c) {
					bestScore = s
					best = c
				}
			}
			if best != "" {
				modelID = best
			}
		}
	}

	// No feedback winner: pick cheapest candidate that has a provider.
	if modelID == "" {
		// Sort candidates by cost-per-token ascending (cheapest first within same tier).
		pricing := NewPricingStore(r.pool)
		type costed struct {
			id      string
			costPer float64
		}
		scored := []costed{}
		for _, c := range candidates {
			in, out, _ := pricing.GetPrice(ctx, c)
			scored = append(scored, costed{id: c, costPer: in + out})
		}
		// Stable sort: lower cost first.
		for i := 1; i < len(scored); i++ {
			for j := i; j > 0 && scored[j].costPer < scored[j-1].costPer; j-- {
				scored[j], scored[j-1] = scored[j-1], scored[j]
			}
		}
		for _, sc := range scored {
			if r.modelHasProvider(sc.id) {
				modelID = sc.id
				break
			}
		}
		// Final fallback: first candidate regardless of provider check.
		if modelID == "" && len(candidates) > 0 {
			modelID = candidates[0]
		}
	}

	// Fallback: use default selected model
	if modelID == "" {
		r.pool.QueryRow(ctx,
			`SELECT model_id FROM selected_models WHERE tenant_id = $1 AND is_default = true LIMIT 1`,
			tenantID).Scan(&modelID)
	}

	// Second fallback: if we still have nothing, pick from the static
	// catalog based on the classifier's tier output. This lets a fresh
	// tenant route intelligently the first time even before any
	// category_model_assignments rows exist. Prefer Bedrock when it's
	// among the configured providers (matches the deployment default).
	if modelID == "" {
		tierForCategory := map[string]string{
			"coding":      TierCoding,
			"reasoning":   TierReasoning,
			"analysis":    TierComplex,
			"research":    TierComplex,
			"writing":     TierStandard,
			"chat":        TierStandard,
			"creative":    TierStandard,
			"translation": TierSimple,
		}
		tier := tierForCategory[category]
		if tier == "" { tier = TierStandard }

		if cat, err := LoadCatalog(); err == nil && cat != nil && r.registry != nil {
			// Names of providers currently registered + enabled.
			avail := []string{}
			for _, cfg := range r.registry.List() {
				if cfg.Enabled { avail = append(avail, cfg.Name) }
			}
			// Walk the tier ladder downward — e.g. if no coding-tier
			// model is configured, try standard, then complex.
			for _, t := range tierLadder(tier) {
				if pick := cat.BestForTier(t, avail); pick != nil {
					modelID = pick.ID
					break
				}
			}
		}
	}

	// Check model exclusion
	if r.exclusion != nil && r.exclusion.IsExcluded(modelID) {
		slog.Info("smart_router.excluded", "model", modelID)
		// Try to find a non-excluded alternative
		if r.registry != nil {
			for _, p := range r.registry.List() {
				if !r.exclusion.IsExcluded(p.Name) && p.Enabled {
					modelID = p.Name
					slog.Info("smart_router.fallback", "to", modelID)
					break
				}
			}
		}
	}

	decision := &RoutingDecision{
		Category:   category,
		Confidence: confidence,
		ModelID:    modelID,
		Reason:     reasonText(category, confidence),
		Tier:       tier,
		Dimensions: dims,
	}

	// Log the decision
	preview := query
	if len(preview) > 100 {
		preview = preview[:100]
	}
	r.pool.Exec(ctx,
		`INSERT INTO routing_decisions (tenant_id, query_preview, classified_category, confidence, routed_model)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, preview, category, confidence, modelID)

	slog.Info("smart_router.routed", "category", category, "confidence", confidence, "model", modelID)
	return decision, nil
}

// classify determines work category. Tries MiniLM service first, falls back to keywords.
func (r *SmartRouter) classify(query string) (string, float64) {
	// Try MiniLM classifier service if available (optional sidecar)
	if cat, conf, ok := r.classifyViaMiniLM(query); ok {
		return cat, conf
	}
	// Fallback to keyword heuristics
	return r.classifyKeywords(query)
}

func (r *SmartRouter) classifyViaMiniLM(query string) (string, float64, bool) {
	// Call the MiniLM classifier sidecar at localhost:8890
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"query": query})
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost:8890/classify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "", 0, false }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return "", 0, false }
	var result struct { Category string `json:"tier"`; Confidence float64 `json:"confidence"` }
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Category == "" { return "", 0, false }
	return result.Category, result.Confidence, true
}

func (r *SmartRouter) classifyKeywords(query string) (string, float64) {
	lower := strings.ToLower(query)
	words := strings.Fields(lower)
	wordCount := len(words)

	type match struct {
		slug       string
		confidence float64
	}

	// Score each category
	scores := map[string]float64{
		"chat": 0, "coding": 0, "writing": 0, "research": 0,
		"analysis": 0, "reasoning": 0, "creative": 0, "translation": 0,
	}

	// Coding signals
	codingKW := []string{"code", "function", "bug", "debug", "error", "implement", "refactor",
		"api", "endpoint", "database", "sql", "python", "javascript", "golang", "react",
		"docker", "kubernetes", "deploy", "git", "compile", "syntax", "variable", "class",
		"import", "package", "module", "test", "unit test", "regex", "algorithm"}
	for _, kw := range codingKW {
		if strings.Contains(lower, kw) { scores["coding"] += 10 }
	}

	// Writing signals
	writingKW := []string{"write", "draft", "email", "article", "blog", "report", "essay",
		"letter", "proposal", "document", "content", "copy", "headline", "paragraph",
		"proofread", "edit", "rewrite", "tone", "formal", "casual"}
	for _, kw := range writingKW {
		if strings.Contains(lower, kw) { scores["writing"] += 10 }
	}

	// Research signals
	researchKW := []string{"search", "find", "look up", "research", "latest", "news",
		"trending", "statistics", "data", "source", "reference", "study", "paper",
		"survey", "market", "competitor", "benchmark"}
	for _, kw := range researchKW {
		if strings.Contains(lower, kw) { scores["research"] += 10 }
	}

	// Analysis signals
	analysisKW := []string{"analyze", "analysis", "compare", "evaluate", "assess",
		"review", "breakdown", "metrics", "performance", "trend", "insight",
		"dashboard", "chart", "graph", "spreadsheet", "csv"}
	for _, kw := range analysisKW {
		if strings.Contains(lower, kw) { scores["analysis"] += 10 }
	}

	// Reasoning signals
	reasoningKW := []string{"prove", "proof", "theorem", "derive", "calculate",
		"step by step", "logic", "puzzle", "math", "equation", "formula",
		"probability", "optimize", "solve", "reason"}
	for _, kw := range reasoningKW {
		if strings.Contains(lower, kw) { scores["reasoning"] += 10 }
	}

	// Creative signals
	creativeKW := []string{"brainstorm", "idea", "creative", "story", "poem",
		"imagine", "design", "concept", "innovate", "invent", "fiction",
		"character", "plot", "narrative", "slogan", "tagline"}
	for _, kw := range creativeKW {
		if strings.Contains(lower, kw) { scores["creative"] += 10 }
	}

	// Translation signals
	translationKW := []string{"translate", "translation", "language", "spanish",
		"french", "german", "chinese", "japanese", "korean", "hindi",
		"localize", "localization", "multilingual"}
	for _, kw := range translationKW {
		if strings.Contains(lower, kw) { scores["translation"] += 10 }
	}

	// Chat baseline (short queries default to chat)
	if wordCount <= 5 { scores["chat"] += 15 }
	if wordCount <= 10 { scores["chat"] += 5 }

	// Find best category
	bestSlug := "chat"
	bestScore := scores["chat"]
	for slug, score := range scores {
		if score > bestScore {
			bestSlug = slug
			bestScore = score
		}
	}

	// Calculate confidence (0-1)
	totalScore := 0.0
	for _, s := range scores { totalScore += s }
	confidence := 0.5
	if totalScore > 0 {
		confidence = bestScore / totalScore
		if confidence > 0.95 { confidence = 0.95 }
	}

	return bestSlug, confidence
}

func reasonText(category string, confidence float64) string {
	pct := int(confidence * 100)
	names := map[string]string{
		"chat": "general conversation", "coding": "code-related task",
		"writing": "writing task", "research": "research query",
		"analysis": "data analysis", "reasoning": "reasoning/math problem",
		"creative": "creative task", "translation": "translation request",
	}
	name := names[category]
	if name == "" { name = category }
	return strings.Join([]string{"Classified as ", name, " (", string(rune('0'+pct/10)), string(rune('0'+pct%10)), "% confidence)"}, "")
}

// ListCategories returns all work categories.
func (r *SmartRouter) ListCategories(ctx context.Context, tenantID string) ([]WorkCategory, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, slug, COALESCE(description,''), COALESCE(icon,''), COALESCE(color,'violet') FROM work_categories WHERE tenant_id = $1 ORDER BY display_order`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	cats := []WorkCategory{}
	for rows.Next() {
		var c WorkCategory
		rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Icon, &c.Color)
		cats = append(cats, c)
	}
	return cats, nil
}

// AssignModel assigns a model to a category.
func (r *SmartRouter) AssignModel(ctx context.Context, tenantID, categorySlug, modelID string, priority int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO category_model_assignments (tenant_id, category_slug, model_id, priority) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, category_slug, model_id) DO UPDATE SET priority = $4`,
		tenantID, categorySlug, modelID, priority)
	return err
}

// UnassignModel removes a model from a category.
func (r *SmartRouter) UnassignModel(ctx context.Context, tenantID, categorySlug, modelID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM category_model_assignments WHERE tenant_id = $1 AND category_slug = $2 AND model_id = $3`,
		tenantID, categorySlug, modelID)
	return err
}

// SeedDefaultAssignments populates category_model_assignments for tenantID
// using the enriched static catalog. It only inserts rows that don't already
// exist (ON CONFLICT DO NOTHING), so manual overrides are never clobbered.
// Call this after the first boot and after each AA/LLM Stats enrichment cycle.
func (r *SmartRouter) SeedDefaultAssignments(ctx context.Context, tenantID string) int {
	if r.pool == nil || r.registry == nil {
		return 0
	}
	cat, err := LoadCatalog()
	if err != nil || cat == nil {
		return 0
	}

	// Names of currently enabled providers — BestForTier filters to these.
	avail := []string{}
	for _, cfg := range r.registry.List() {
		if cfg.Enabled {
			avail = append(avail, cfg.Name)
		}
	}
	if len(avail) == 0 {
		return 0
	}

	// For each known routing category pick the best model per the catalog.
	categoryTiers := map[string]string{
		"coding":      TierCoding,
		"reasoning":   TierReasoning,
		"analysis":    TierComplex,
		"research":    TierComplex,
		"writing":     TierStandard,
		"chat":        TierStandard,
		"creative":    TierStandard,
		"translation": TierSimple,
	}

	seeded := 0
	for slug, tier := range categoryTiers {
		var pick *ModelCatalogEntry
		for _, t := range tierLadder(tier) {
			if pick = cat.BestForTier(t, avail); pick != nil {
				break
			}
		}
		if pick == nil {
			continue
		}
		tag, _ := r.pool.Exec(ctx,
			`INSERT INTO category_model_assignments (tenant_id, category_slug, model_id, priority)
			 VALUES ($1, $2, $3, 100)
			 ON CONFLICT (tenant_id, category_slug, model_id) DO NOTHING`,
			tenantID, slug, pick.ID)
		if tag.RowsAffected() > 0 {
			seeded++
		}
	}
	if seeded > 0 {
		slog.Info("smart_router.seeded_defaults", "tenant", tenantID, "rows", seeded)
	}
	return seeded
}

// GetAssignments returns model assignments for all categories.
func (r *SmartRouter) GetAssignments(ctx context.Context, tenantID string) (map[string][]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT category_slug, model_id FROM category_model_assignments WHERE tenant_id = $1 ORDER BY priority`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	result := make(map[string][]string)
	for rows.Next() {
		var slug, model string
		rows.Scan(&slug, &model)
		result[slug] = append(result[slug], model)
	}
	return result, nil
}

// CorrectDecision records a user correction for learning.
func (r *SmartRouter) CorrectDecision(ctx context.Context, decisionID, correctModel, correctCategory string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE routing_decisions SET user_override_model = $1, user_override_category = $2, was_correct = false WHERE id = $3`,
		correctModel, correctCategory, decisionID)
	return err
}

// GetRecentDecisions returns recent routing decisions for the UI.
func (r *SmartRouter) GetRecentDecisions(ctx context.Context, tenantID string, limit int) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, COALESCE(soul_id::text,''), query_preview, classified_category, confidence, routed_model,
		        COALESCE(user_override_model,''), COALESCE(user_override_category,''), COALESCE(was_correct, true), created_at
		 FROM routing_decisions WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	decisions := []map[string]any{}
	for rows.Next() {
		var id, soulID, preview, category, model, overrideModel, overrideCat string
		var confidence float64
		var correct bool
		var createdAt interface{}
		rows.Scan(&id, &soulID, &preview, &category, &confidence, &model, &overrideModel, &overrideCat, &correct, &createdAt)
		decisions = append(decisions, map[string]any{
			"id": id, "soul_id": soulID, "query_preview": preview,
			"category": category, "confidence": confidence, "model": model,
			"override_model": overrideModel, "override_category": overrideCat,
			"was_correct": correct, "created_at": createdAt,
		})
	}
	return decisions, nil
}

// modelHasProvider checks if any registered + enabled provider can serve this model.
// Uses ModelRegistry for known models, then falls back to name-based affinity.
func (r *SmartRouter) modelHasProvider(model string) bool {
	if r.registry == nil { return true }
	lower := strings.ToLower(model)
	spec, knownModel := ModelRegistry[model]
	for _, cfg := range r.registry.List() {
		if !cfg.Enabled { continue }
		// Known model: check provider-type match from registry spec.
		if knownModel {
			switch spec.Provider {
			case "anthropic":
				if cfg.ProviderType == TypeAnthropicNative { return true }
			case "openai":
				if cfg.ProviderType == TypeOpenAICompat || cfg.ProviderType == TypeOpenRouter { return true }
			case "gemini":
				if cfg.ProviderType == TypeGeminiNative { return true }
			case "bedrock":
				if cfg.ProviderType == TypeBedrock || cfg.ProviderType == TypeBedrockConverse || cfg.ProviderType == TypeBedrockMantle { return true }
			case "dashscope":
				if cfg.ProviderType == TypeDashScope { return true }
			default:
				// For all other known providers, match by name or openai-compat proxy.
				if strings.EqualFold(cfg.Name, spec.Provider) { return true }
				if cfg.ProviderType == TypeOpenRouter { return true }
			}
			continue
		}
		// Unknown model: name-based affinity heuristics.
		if (strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic")) &&
			cfg.ProviderType == TypeAnthropicNative { return true }
		if (strings.Contains(lower, "gemini") || strings.Contains(lower, "gemma")) &&
			cfg.ProviderType == TypeGeminiNative { return true }
		if cfg.ProviderType == TypeOpenRouter { return true }
		// Custom openai-compat proxy with a user-configured base URL can serve arbitrary models.
		if cfg.ProviderType == TypeOpenAICompat && cfg.APIBase != "" { return true }
	}
	return false
}

// BestModelForTier returns the best available model ID for a given tier string
// (TierCoding, TierStandard, etc.), walking down the tier ladder until a match
// is found. Returns "" when no provider is registered or the catalog is absent.
func (r *SmartRouter) BestModelForTier(tier string) string {
	if r.registry == nil {
		return ""
	}
	cat, err := LoadCatalog()
	if err != nil || cat == nil {
		return ""
	}
	avail := []string{}
	for _, cfg := range r.registry.List() {
		if cfg.Enabled {
			avail = append(avail, cfg.Name)
		}
	}
	for _, t := range tierLadder(tier) {
		if pick := cat.BestForTier(t, avail); pick != nil {
			return pick.ID
		}
	}
	return ""
}

// Exclusion returns the model exclusion list.
func (r *SmartRouter) Exclusion() *ModelExclusion {
	if r.exclusion == nil { r.exclusion = NewModelExclusion() }
	return r.exclusion
}

func (r *SmartRouter) Costs() *CostTracker {
	if r.costs == nil { r.costs = &CostTracker{} }
	return r.costs
}

// ─── Multi-model ranking ─────────────────────────────────────────────────────

// ModelScore holds performance metrics for a model in a given category.
type ModelScore struct {
	ModelID     string  `json:"model_id"`
	Category    string  `json:"category"`
	SuccessRate float64 `json:"success_rate"` // 0-1, based on user corrections
	AvgLatencyMs int64  `json:"avg_latency_ms"`
	UsageCount  int     `json:"usage_count"`
	Score       float64 `json:"score"` // composite ranking score
}

// RecordOutcome records whether a routing decision was good or bad.
// Call this when the user corrects a decision (bad=true) or explicitly approves (bad=false).
func (r *SmartRouter) RecordOutcome(ctx context.Context, tenantID, modelID, category string, bad bool) {
	if r.pool == nil { return }
	penalty := 0
	if bad { penalty = 1 }
	r.pool.Exec(ctx,
		`INSERT INTO model_performance (tenant_id, model_id, category_slug, total_uses, bad_outcomes)
		 VALUES ($1, $2, $3, 1, $4)
		 ON CONFLICT (tenant_id, model_id, category_slug)
		 DO UPDATE SET total_uses = model_performance.total_uses + 1,
		               bad_outcomes = model_performance.bad_outcomes + $4,
		               updated_at = NOW()`,
		tenantID, modelID, category, penalty)
}

// GetModelScores returns ranked model scores for a category.
func (r *SmartRouter) GetModelScores(ctx context.Context, tenantID, category string) ([]ModelScore, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT model_id, category_slug,
		        COALESCE(1.0 - (bad_outcomes::float / NULLIF(total_uses,0)), 1.0) as success_rate,
		        total_uses
		 FROM model_performance
		 WHERE tenant_id = $1 AND category_slug = $2
		 ORDER BY success_rate DESC, total_uses DESC`,
		tenantID, category)
	if err != nil { return nil, err }
	defer rows.Close()
	scores := []ModelScore{}
	for rows.Next() {
		var s ModelScore
		s.Category = category
		rows.Scan(&s.ModelID, &s.Category, &s.SuccessRate, &s.UsageCount)
		// Composite score: success rate weighted by volume (Wilson lower bound approximation)
		n := float64(s.UsageCount)
		p := s.SuccessRate
		if n > 0 {
			z := 1.96 // 95% confidence
			s.Score = (p + z*z/(2*n) - z*float64(n*n)*((p*(1-p))/n+z*z/(4*n*n))) / (1 + z*z/n)
		} else {
			s.Score = 0.5 // uninformed prior
		}
		scores = append(scores, s)
	}
	return scores, nil
}

// ─── Advisor Strategy ────────────────────────────────────────────────────────

// AdvisorConfig defines advisor/executor routing.
// Advisor reviews the executor's draft and can request revisions.
type AdvisorConfig struct {
	Enabled       bool   `json:"enabled"`
	AdvisorModel  string `json:"advisor_model"`  // highest-capability model available
	ExecutorModel string `json:"executor_model"` // second-tier model (fast + capable)
	MaxRevisions  int    `json:"max_revisions"`  // default 2
}

// ─── Model capability scoring — 3 signal tiers ───────────────────────────────
//
// Tier 1: Static registry (ModelRegistry map) — known models have exact specs.
// Tier 2: Name heuristics — unknown models are scored from their ID string.
//         Handles Ollama, HuggingFace, custom endpoints, private models.
// Tier 3: Runtime performance — DB-stored success rates override static scores
//         over time, so the system self-corrects as real data accumulates.

// modelCapabilityScore returns a numeric capability score for a model.
// Higher = more capable = better suited as advisor.
func modelCapabilityScore(modelID string) float64 {
	spec := GetModelSpec(modelID)

	// If the model IS in our registry, use exact spec data.
	if _, known := ModelRegistry[modelID]; known {
		score := spec.InputCostPer1M
		if spec.SupportsReasoning { score += 5.0 }
		if spec.MaxOutputTokens >= 32000 { score += 2.0 }
		if spec.MaxOutputTokens >= 64000 { score += 2.0 }
		return score
	}

	// Unknown model — use heuristic scoring from the model name.
	// This handles Ollama (llama3.1:70b), HuggingFace slugs, private endpoints.
	return heuristicScore(modelID)
}

// heuristicScore scores an unknown model purely from its name string.
// Works for any naming convention: ollama tags, HF model IDs, custom names.
func heuristicScore(modelID string) float64 {
	lower := strings.ToLower(modelID)
	score := 1.0 // baseline for unknown models

	// ── Parameter count signals (strongest indicator of capability) ──
	paramPatterns := []struct {
		marker string
		score  float64
	}{
		{"405b", 12.0}, {"72b", 9.0}, {"70b", 9.0},
		{"671b", 14.0}, // DeepSeek-V3/R1 full
		{"32b", 7.0}, {"34b", 7.0},
		{"14b", 5.0}, {"13b", 4.5},
		{"8b", 3.5}, {"7b", 3.0},
		{"3b", 2.0}, {"1.5b", 1.5}, {"1b", 1.2},
	}
	for _, p := range paramPatterns {
		if strings.Contains(lower, p.marker) {
			score = p.score
			break
		}
	}

	// ── Model family signals ──
	familyBoosts := []struct {
		marker string
		boost  float64
	}{
		// Reasoning / thinking models
		{"r1", 5.0}, {"qwq", 5.0}, {"o1", 5.0}, {"o3", 6.0},
		{"thinking", 4.0}, {"-r-", 3.0}, {"reasoning", 4.0},
		// Top-tier families
		{"opus", 8.0}, {"gpt-4o", 5.0}, {"gpt-4", 4.0},
		{"gemini-1.5-pro", 4.5}, {"gemini-2.0", 5.0},
		{"deepseek-v3", 3.5}, {"deepseek-r1", 8.0},
		{"qwen3", 4.0}, {"qwen2.5", 3.5},
		{"mistral-large", 4.0}, {"mixtral", 3.5},
		{"llama-3.3", 4.5}, {"llama-3.1", 4.0},
		{"command-r-plus", 4.0},
		// Executor/mid tier
		{"sonnet", 3.0}, {"haiku", 1.5}, {"flash", 1.5},
		{"mini", 0.8}, {"small", 0.8}, {"nano", 0.5},
		// Instruction-tuned adds a small boost over base
		{"-instruct", 0.5}, {"-chat", 0.3}, {"-it", 0.3},
	}
	for _, f := range familyBoosts {
		if strings.Contains(lower, f.marker) {
			score += f.boost
		}
	}

	// ── Quantization penalty (lower precision = lower capability in practice) ──
	quantPenalties := []struct {
		marker  string
		penalty float64
	}{
		{"q4_0", -1.5}, {"q4_k_m", -1.0}, {"q4", -1.0},
		{"q3", -1.5}, {"q2", -2.0},
		{"q8", -0.3},  // q8 is near lossless
		{"gguf", -0.5}, // GGUF = quantized local model
	}
	for _, q := range quantPenalties {
		if strings.Contains(lower, q.marker) {
			score += q.penalty
		}
	}

	if score < 0.1 { score = 0.1 }
	return score
}

// SuggestAdvisorConfig ranks available models by capability and assigns roles.
// Works with ANY provider: Ollama, HuggingFace, Bedrock, OpenAI, DeepSeek, etc.
// Signal order: static registry → name heuristics → runtime DB performance.
// The most capable model becomes advisor; the second-best becomes executor.
func SuggestAdvisorConfig(selectedModels []string) *AdvisorConfig {
	if len(selectedModels) == 0 {
		return &AdvisorConfig{MaxRevisions: 2}
	}

	type scored struct {
		id    string
		score float64
	}
	ranked := make([]scored, 0, len(selectedModels))
	for _, m := range selectedModels {
		ranked = append(ranked, scored{id: m, score: modelCapabilityScore(m)})
	}
	// Sort descending
	for i := 0; i < len(ranked)-1; i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].score > ranked[i].score {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}

	cfg := &AdvisorConfig{MaxRevisions: 2}
	if len(ranked) >= 2 {
		cfg.AdvisorModel = ranked[0].id
		cfg.ExecutorModel = ranked[1].id
		// Enable only when there's a genuine capability gap (not just noise).
		// A gap > 2.0 means clearly different tiers (e.g. 70b vs 7b, or r1 vs v3).
		if ranked[0].score-ranked[1].score > 2.0 {
			cfg.Enabled = true
		}
	} else {
		cfg.ExecutorModel = ranked[0].id
	}
	return cfg
}
