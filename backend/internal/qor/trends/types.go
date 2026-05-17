// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import "time"

// types.go — Core data model for Qorven Social Intelligence pipeline.

type ProviderRuntime struct {
	ReasoningProvider string `json:"reasoning_provider"` // gemini, openai, deepseek
	PlannerModel      string `json:"planner_model"`
	RerankModel       string `json:"rerank_model"`
	XSearchBackend    string `json:"x_search_backend,omitempty"` // xai, bird
}

type SubQuery struct {
	Label        string   `json:"label"`
	SearchQuery  string   `json:"search_query"`
	RankingQuery string   `json:"ranking_query"`
	Sources      []string `json:"sources"`
	Weight       float64  `json:"weight"`
}

type QueryPlan struct {
	Intent        string             `json:"intent"`
	FreshnessMode string             `json:"freshness_mode"`
	ClusterMode   string             `json:"cluster_mode"`
	RawTopic      string             `json:"raw_topic"`
	SubQueries    []SubQuery         `json:"subqueries"`
	SourceWeights map[string]float64 `json:"source_weights"`
	Notes         []string           `json:"notes,omitempty"`
}

type SourceItem struct {
	ItemID         string            `json:"item_id"`
	Source         string            `json:"source"`
	Title          string            `json:"title"`
	Body           string            `json:"body"`
	URL            string            `json:"url"`
	Author         string            `json:"author,omitempty"`
	Container      string            `json:"container,omitempty"` // subreddit, channel, etc.
	PublishedAt    *time.Time        `json:"published_at,omitempty"`
	DateConfidence string            `json:"date_confidence"` // high, med, low
	Engagement     map[string]float64 `json:"engagement,omitempty"`
	RelevanceHint  float64           `json:"relevance_hint"`
	WhyRelevant    string            `json:"why_relevant,omitempty"`
	Snippet        string            `json:"snippet,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	// Signal fields populated after construction
	LocalRelevance  *float64 `json:"local_relevance,omitempty"`
	Freshness       *int     `json:"freshness,omitempty"`
	EngagementScore *float64 `json:"engagement_score,omitempty"`
	SourceQuality   *float64 `json:"source_quality,omitempty"`
	LocalRankScore  *float64 `json:"local_rank_score,omitempty"`
}

type Candidate struct {
	CandidateID    string            `json:"candidate_id"`
	ItemID         string            `json:"item_id"`
	Source         string            `json:"source"`
	Title          string            `json:"title"`
	URL            string            `json:"url"`
	Snippet        string            `json:"snippet"`
	SubQueryLabels []string          `json:"subquery_labels"`
	NativeRanks    map[string]int    `json:"native_ranks"`
	LocalRelevance float64           `json:"local_relevance"`
	Freshness      int               `json:"freshness"`
	Engagement     float64           `json:"engagement"`
	SourceQuality  float64           `json:"source_quality"`
	RRFScore       float64           `json:"rrf_score"`
	Sources        []string          `json:"sources,omitempty"`
	SourceItems    []SourceItem      `json:"source_items,omitempty"`
	RerankScore    *float64          `json:"rerank_score,omitempty"`
	FinalScore     float64           `json:"final_score"`
	Explanation    string            `json:"explanation,omitempty"`
	ClusterID      string            `json:"cluster_id,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
}

type Cluster struct {
	ClusterID        string   `json:"cluster_id"`
	Title            string   `json:"title"`
	CandidateIDs     []string `json:"candidate_ids"`
	RepresentativeIDs []string `json:"representative_ids"`
	Sources          []string `json:"sources"`
	Score            float64  `json:"score"`
	Uncertainty      string   `json:"uncertainty,omitempty"` // single-source, thin-evidence
}

type Report struct {
	Topic           string             `json:"topic"`
	RangeFrom       string             `json:"range_from"`
	RangeTo         string             `json:"range_to"`
	GeneratedAt     string             `json:"generated_at"`
	ProviderRuntime ProviderRuntime    `json:"provider_runtime"`
	QueryPlan       QueryPlan          `json:"query_plan"`
	Clusters        []Cluster          `json:"clusters"`
	RankedCandidates []Candidate       `json:"ranked_candidates"`
	ItemsBySource   map[string][]SourceItem `json:"items_by_source"`
	ErrorsBySource  map[string]string  `json:"errors_by_source"`
	Warnings        []string           `json:"warnings,omitempty"`
}

type RetrievalBundle struct {
	ItemsBySourceAndQuery map[string][]SourceItem `json:"items_by_source_and_query"`
	ItemsBySource         map[string][]SourceItem `json:"items_by_source"`
	ErrorsBySource        map[string]string       `json:"errors_by_source"`
}

func NewRetrievalBundle() *RetrievalBundle {
	return &RetrievalBundle{
		ItemsBySourceAndQuery: make(map[string][]SourceItem),
		ItemsBySource:         make(map[string][]SourceItem),
		ErrorsBySource:        make(map[string]string),
	}
}

func (b *RetrievalBundle) AddItems(label, source string, items []SourceItem) {
	key := label + "|" + source
	b.ItemsBySourceAndQuery[key] = append(b.ItemsBySourceAndQuery[key], items...)
	b.ItemsBySource[source] = append(b.ItemsBySource[source], items...)
}

// DepthConfig controls how many results to fetch per source.
type DepthConfig struct {
	Quick   int
	Default int
	Deep    int
}

func (d DepthConfig) Get(depth string) int {
	switch depth {
	case "quick": return d.Quick
	case "deep": return d.Deep
	default: return d.Default
	}
}
