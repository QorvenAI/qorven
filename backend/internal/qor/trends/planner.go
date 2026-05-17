// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// planner.go — LLM-first query planning with deterministic guards.
// Rewritten from last30days planner.py (576 lines).

var AllowedIntents = map[string]bool{
	"factual": true, "product": true, "concept": true, "opinion": true,
	"how_to": true, "comparison": true, "breaking_news": true, "prediction": true,
}

var AllowedClusterModes = map[string]bool{
	"none": true, "story": true, "workflow": true, "market": true, "debate": true,
}

var SourcePriority = map[string][]string{
	"factual":       {"hackernews", "reddit", "x", "youtube"},
	"product":       {"youtube", "reddit", "x", "tiktok", "hackernews"},
	"concept":       {"hackernews", "reddit", "x", "youtube"},
	"opinion":       {"reddit", "x", "youtube", "hackernews"},
	"how_to":        {"youtube", "reddit", "x", "hackernews"},
	"comparison":    {"reddit", "x", "hackernews", "youtube"},
	"breaking_news": {"x", "reddit", "hackernews", "youtube", "polymarket"},
	"prediction":    {"polymarket", "x", "hackernews", "reddit", "youtube"},
}

var QuickSourceLimits = map[string]int{
	"factual": 2, "product": 2, "concept": 2, "opinion": 2,
	"how_to": 2, "comparison": 2, "breaking_news": 2, "prediction": 2,
}

var IntentSourceExclusions = map[string]map[string]bool{
	"concept": {"polymarket": true},
	"how_to":  {"polymarket": true},
}

var SourceCapabilities = map[string]map[string]bool{
	"reddit":       {"discussion": true, "social": true},
	"x":            {"discussion": true, "social": true},
	"youtube":      {"video": true, "video_longform": true, "discussion": true},
	"tiktok":       {"video": true, "video_shortform": true, "social": true},
	"instagram":    {"video": true, "video_shortform": true, "social": true},
	"hackernews":   {"discussion": true, "link": true},
	"bluesky":      {"discussion": true, "social": true},
	"polymarket":   {"market": true},
	"xiaohongshu":  {"video": true, "video_shortform": true, "social": true},
	"github":       {"discussion": true, "link": true},
	"perplexity":   {"web": true, "reference": true, "analysis": true},
}

// ReasoningClient generates JSON from a prompt (implemented by LLM providers).
type ReasoningClient interface {
	GenerateJSON(model, prompt string) (map[string]any, error)
}

// PlanQuery creates a query plan. Comparison queries with extractable entities
// use a deterministic plan; other intents prefer the LLM planner.
func PlanQuery(topic string, availableSources []string, requestedSources []string, depth string, provider ReasoningClient, model string) QueryPlan {
	if shouldForceDeterministic(topic) {
		return fallbackPlan(topic, availableSources, requestedSources, depth, "deterministic-comparison-plan")
	}

	prompt := buildPlannerPrompt(topic, availableSources, requestedSources, depth)

	if provider != nil && model != "" {
		raw, err := provider.GenerateJSON(model, prompt)
		if err == nil {
			plan := sanitizePlan(raw, topic, availableSources, requestedSources, depth)
			if len(plan.SubQueries) > 0 {
				return plan
			}
		}
	}

	return fallbackPlan(topic, availableSources, requestedSources, depth, "fallback-plan")
}

func buildPlannerPrompt(topic string, available, requested []string, depth string) string {
	reqStr := "auto"
	if len(requested) > 0 { reqStr = strings.Join(requested, ", ") }
	return fmt.Sprintf(`You are the query planner for a live last-30-days research pipeline.

Topic: %s
Depth: %s
Available sources: %s
Requested sources: %s

Return JSON only with this shape:
{
  "intent": "factual|product|concept|opinion|how_to|comparison|breaking_news|prediction",
  "freshness_mode": "strict_recent|balanced_recent|evergreen_ok",
  "cluster_mode": "none|story|workflow|market|debate",
  "source_weights": {"source_name": 1.0},
  "subqueries": [
    {
      "label": "short label",
      "search_query": "keyword style query for search APIs",
      "ranking_query": "natural language rewrite for reranking",
      "sources": ["reddit", "x"],
      "weight": 1.0
    }
  ],
  "notes": ["optional short notes"]
}

Rules:
- emit 1 to 4 subqueries
- every subquery must include both search_query and ranking_query
- sources must be drawn from Available sources only
- search_query should be concise and keyword-heavy
- ranking_query should read like a natural-language question
- preserve exact proper nouns and entity strings from the topic
- NEVER include temporal phrases in search_query
- NEVER include meta-research phrases like 'news', 'updates', 'latest'`, topic, depth, strings.Join(available, ", "), reqStr)
}

func sanitizePlan(raw map[string]any, topic string, available, requested []string, depth string) QueryPlan {
	intentHint := getString(raw, "intent")
	if !AllowedIntents[intentHint] { intentHint = InferIntent(topic) }

	availSet := toSet(available)
	eligible := available
	if len(requested) > 0 { eligible = intersect(available, requested) }

	// Source weights
	sourceWeights := map[string]float64{}
	if sw, ok := raw["source_weights"].(map[string]any); ok {
		for s, w := range sw {
			if availSet[s] { sourceWeights[s] = toFloat(w) }
		}
	}
	if len(requested) > 0 {
		filtered := map[string]float64{}
		for s, w := range sourceWeights {
			if contains(requested, s) { filtered[s] = w }
		}
		sourceWeights = filtered
	}
	if len(sourceWeights) == 0 { sourceWeights = defaultSourceWeights(InferIntent(topic), eligible) }
	for _, s := range eligible { if _, ok := sourceWeights[s]; !ok { sourceWeights[s] = 1.0 } }
	sourceWeights = normalizeWeights(sourceWeights)

	// Subqueries
	var subqueries []SubQuery
	rawSQs, _ := raw["subqueries"].([]any)
	maxSQ := maxSubqueries(intentHint)
	for i, rawSQ := range rawSQs {
		if i >= maxSQ { break }
		sq, ok := rawSQ.(map[string]any)
		if !ok { continue }
		searchQ := getString(sq, "search_query")
		rankQ := getString(sq, "ranking_query")
		if searchQ == "" || rankQ == "" { continue }

		sources := getStringSlice(sq, "sources")
		sources = filterAvailable(sources, sourceWeights)
		if len(sources) == 0 { sources = mapKeys(sourceWeights) }

		label := getString(sq, "label")
		if label == "" { label = fmt.Sprintf("q%d", i+1) }

		subqueries = append(subqueries, SubQuery{
			Label: label, SearchQuery: searchQ, RankingQuery: rankQ,
			Sources: sources, Weight: maxF(0.05, toFloat(sq["weight"])),
		})
	}

	if depth == "quick" && len(subqueries) > 1 { subqueries = subqueries[:1] }
	if len(subqueries) == 0 { return fallbackPlan(topic, available, requested, depth, "fallback-plan") }

	freshness := getString(raw, "freshness_mode")
	if freshness == "" { freshness = defaultFreshness(intentHint) }
	if intentHint == "how_to" { freshness = "evergreen_ok" }

	clusterMode := getString(raw, "cluster_mode")
	if !AllowedClusterModes[clusterMode] { clusterMode = defaultClusterMode(intentHint) }

	subqueries = normalizeSubqueryWeights(trimSubqueriesForDepth(subqueries, intentHint, depth, eligible))

	return QueryPlan{
		Intent: intentHint, FreshnessMode: freshness, ClusterMode: clusterMode,
		RawTopic: topic, SubQueries: subqueries, SourceWeights: sourceWeights,
		Notes: getStringSlice(raw, "notes"),
	}
}

func fallbackPlan(topic string, available, requested []string, depth, note string) QueryPlan {
	intent := InferIntent(topic)
	allowed := available
	if len(requested) > 0 { allowed = requested }
	sourceWeights := defaultSourceWeights(intent, allowed)

	core := extractCoreSubject(topic)
	baseSearch := keywordQuery(topic, core)
	baseRanking := rankingQuery(topic, core)

	subqueries := []SubQuery{{
		Label: "primary", SearchQuery: baseSearch, RankingQuery: baseRanking,
		Sources: mapKeys(sourceWeights), Weight: 1.0,
	}}

	if depth != "quick" {
		switch intent {
		case "comparison":
			for i, entity := range comparisonEntities(topic) {
				subqueries = append(subqueries, SubQuery{
					Label: fmt.Sprintf("entity-%d", i+1), SearchQuery: entity,
					RankingQuery: fmt.Sprintf("What recent evidence is most relevant to %s in the comparison '%s'?", entity, topic),
					Sources: mapKeys(sourceWeights), Weight: 0.65,
				})
			}
		case "prediction":
			subqueries = append(subqueries, SubQuery{
				Label: "odds", SearchQuery: baseSearch + " odds forecast",
				RankingQuery: fmt.Sprintf("What are the current odds, forecasts, or market signals about %s?", topic),
				Sources: filterSources(sourceWeights, "polymarket", "x", "reddit"), Weight: 0.7,
			})
		case "breaking_news":
			subqueries = append(subqueries, SubQuery{
				Label: "reaction", SearchQuery: baseSearch + " reaction update",
				RankingQuery: fmt.Sprintf("What new reactions or follow-up reporting matter for %s?", topic),
				Sources: filterSources(sourceWeights, "x", "reddit", "hackernews"), Weight: 0.7,
			})
		}
	}

	maxSQ := maxSubqueries(intent)
	if len(subqueries) > maxSQ { subqueries = subqueries[:maxSQ] }

	return QueryPlan{
		Intent: intent, FreshnessMode: defaultFreshness(intent), ClusterMode: defaultClusterMode(intent),
		RawTopic: topic, SubQueries: normalizeSubqueryWeights(trimSubqueriesForDepth(subqueries, intent, depth, mapKeys(sourceWeights))),
		SourceWeights: normalizeWeights(sourceWeights), Notes: []string{note},
	}
}

// InferIntent detects the query intent from the topic string.
func InferIntent(topic string) string {
	t := strings.ToLower(strings.TrimSpace(topic))
	patterns := []struct{ re string; intent string }{
		{`\b(vs|versus|compare|compared to|difference between)\b`, "comparison"},
		{`(?i)\b(odds|predict|prediction|forecast|chance|probability)\b`, "prediction"},
		{`(?i)\bwill .* (win|hit|reach|pass|exceed)\b`, "prediction"},
		{`\b(how to|tutorial|guide|setup|step by step|deploy|install)\b`, "how_to"},
		{`\b(what is|what are|who is|who acquired|when did|parameter count|release date)\b`, "factual"},
		{`\b(thoughts on|worth it|should i|opinion|review)\b`, "opinion"},
		{`\b(latest|news|announced|just shipped|launched|released|update)\b`, "breaking_news"},
		{`\b(pricing|feature|features|best .* for|top .* for)\b`, "product"},
		{`\b(explain|concept|protocol|architecture|what does)\b`, "concept"},
		{`\b(tournament|championship|playoffs|world cup|olympics|super bowl|ceremony|awards|keynote)\b`, "breaking_news"},
	}
	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p.re, t); matched { return p.intent }
	}
	return "breaking_news"
}

func defaultFreshness(intent string) string {
	switch intent {
	case "breaking_news", "prediction": return "strict_recent"
	case "concept", "how_to": return "evergreen_ok"
	default: return "balanced_recent"
	}
}

func defaultClusterMode(intent string) string {
	m := map[string]string{
		"breaking_news": "story", "comparison": "debate", "opinion": "debate",
		"prediction": "market", "how_to": "workflow",
	}
	if v, ok := m[intent]; ok { return v }
	return "none"
}

func defaultSourceWeights(intent string, sources []string) map[string]float64 {
	base := map[string]float64{}
	for _, s := range sources { base[s] = 1.0 }
	bonuses := map[string]map[string]float64{
		"prediction":    {"polymarket": 3.5, "x": 1.3, "reddit": 0.5},
		"breaking_news": {"x": 1.5, "reddit": 1.3, "hackernews": 0.8},
		"how_to":        {"youtube": 2.0, "hackernews": 0.8},
		"factual":       {"reddit": 0.8, "x": 0.5},
	}
	if b, ok := bonuses[intent]; ok {
		for s, bonus := range b { if _, ok := base[s]; ok { base[s] += bonus } }
	}
	return base
}

func normalizeWeights(w map[string]float64) map[string]float64 {
	total := 0.0
	for _, v := range w { if v > 0 { total += v } }
	if total == 0 { total = 1 }
	out := map[string]float64{}
	for k, v := range w { out[k] = maxF(v, 0) / total }
	return out
}

func normalizeSubqueryWeights(sqs []SubQuery) []SubQuery {
	total := 0.0
	for _, sq := range sqs { total += sq.Weight }
	if total == 0 { total = 1 }
	out := make([]SubQuery, len(sqs))
	for i, sq := range sqs { out[i] = sq; out[i].Weight = sq.Weight / total }
	return out
}

func trimSubqueriesForDepth(sqs []SubQuery, intent, depth string, available []string) []SubQuery {
	if depth != "quick" { return sqs }
	limit := QuickSourceLimits[intent]
	if limit == 0 { limit = 3 }
	priority := SourcePriority[intent]
	if len(priority) == 0 { priority = SourcePriority["breaking_news"] }
	ranked := filterAvailableOrdered(priority, toSet(available))
	if len(ranked) == 0 { ranked = available }
	out := make([]SubQuery, len(sqs))
	for i, sq := range sqs {
		out[i] = sq
		preferred := ranked
		if len(preferred) > limit { preferred = preferred[:limit] }
		out[i].Sources = preferred
	}
	return out
}

func maxSubqueries(intent string) int {
	switch intent {
	case "comparison": return 4
	case "factual", "concept": return 2
	default: return 3
	}
}

var vsRe = regexp.MustCompile(`(?i)\bvs\.?\b|\bversus\b|/`)
var trailingCtxRe = regexp.MustCompile(`(?i)\s+\b(?:for|in|on|at|to|with|about|from|by|during|since|after|before|using|via)\b.*$`)

func comparisonEntities(topic string) []string {
	normalized := regexp.MustCompile(`(?i)\bdifference between\s+(.+?)\s+and\s+`).ReplaceAllString(topic, "$1 vs ")
	normalized = regexp.MustCompile(`(?i)\bcompared to\b`).ReplaceAllString(normalized, " vs ")
	parts := vsRe.Split(normalized, -1)
	var entities []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = trailingCtxRe.ReplaceAllString(p, "")
		p = strings.TrimSpace(p)
		if p != "" { entities = append(entities, p) }
	}
	if len(entities) > 4 { entities = entities[:4] }
	return entities
}

func shouldForceDeterministic(topic string) bool {
	return InferIntent(topic) == "comparison" && len(comparisonEntities(topic)) >= 2
}

func keywordQuery(topic, core string) string {
	if core == "" { return topic }
	return core
}

func rankingQuery(topic, core string) string {
	if strings.HasSuffix(strings.TrimSpace(topic), "?") { return strings.TrimSpace(topic) }
	if core != "" && !strings.EqualFold(core, topic) {
		return fmt.Sprintf("What recent evidence from the last 30 days is most relevant to %s, especially about %s?", topic, core)
	}
	return fmt.Sprintf("What recent evidence from the last 30 days is most relevant to %s?", topic)
}

// ── Helpers ──

func getString(m map[string]any, key string) string { v, _ := m[key].(string); return strings.TrimSpace(v) }
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64: return x
	case json.Number: f, _ := x.Float64(); return f
	case int: return float64(x)
	default: return 1.0
	}
}
func getStringSlice(m map[string]any, key string) []string {
	arr, _ := m[key].([]any)
	var out []string
	for _, v := range arr { if s, ok := v.(string); ok && s != "" { out = append(out, s) } }
	return out
}
func maxF(a, b float64) float64 { if a > b { return a }; return b }
func toSet(ss []string) map[string]bool { m := map[string]bool{}; for _, s := range ss { m[s] = true }; return m }
func contains(ss []string, s string) bool { for _, x := range ss { if x == s { return true } }; return false }
func intersect(a, b []string) []string { bs := toSet(b); var out []string; for _, s := range a { if bs[s] { out = append(out, s) } }; return out }
func mapKeys(m map[string]float64) []string { var out []string; for k := range m { out = append(out, k) }; return out }
func filterAvailable(sources []string, weights map[string]float64) []string {
	var out []string; for _, s := range sources { if _, ok := weights[s]; ok { out = append(out, s) } }; return out
}
func filterAvailableOrdered(priority []string, available map[string]bool) []string {
	var out []string; for _, s := range priority { if available[s] { out = append(out, s) } }; return out
}
func filterSources(weights map[string]float64, preferred ...string) []string {
	var out []string; for _, s := range preferred { if _, ok := weights[s]; ok { out = append(out, s) } }
	if len(out) == 0 { return mapKeys(weights) }; return out
}
