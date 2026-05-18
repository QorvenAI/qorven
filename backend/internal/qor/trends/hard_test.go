// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"context"
	"strings"
	"testing"
	"time"
)

// hard_test.go — Diamond-hard tests for Qorven Social Intelligence.
// Every test catches a real bug or verifies critical behavior.

// ── BUG: NormalizeURL appends trailing "?" when no query params ──

func TestHard_NormalizeURL_NoTrailingQuestionMark(t *testing.T) {
	// BUG: NormalizeURL returns "reddit.com/r/golang?" with trailing "?"
	result := NormalizeURL("https://www.reddit.com/r/golang")
	if strings.HasSuffix(result, "?") {
		t.Errorf("BUG: trailing '?' on URL with no params: %q", result)
	}
}

func TestHard_NormalizeURL_StripWWW(t *testing.T) {
	a := NormalizeURL("https://www.reddit.com/r/golang/comments/abc123")
	b := NormalizeURL("https://reddit.com/r/golang/comments/abc123")
	if a != b { t.Errorf("www not stripped: %q != %q", a, b) }
}

func TestHard_NormalizeURL_StripOldReddit(t *testing.T) {
	a := NormalizeURL("https://old.reddit.com/r/golang/comments/abc123")
	b := NormalizeURL("https://reddit.com/r/golang/comments/abc123")
	if a != b { t.Errorf("old. not stripped: %q != %q", a, b) }
}

func TestHard_NormalizeURL_StripUTM(t *testing.T) {
	result := NormalizeURL("https://example.com/page?utm_source=twitter&utm_medium=social&real_param=keep")
	if strings.Contains(result, "utm_source") { t.Error("utm_source not stripped") }
	if strings.Contains(result, "utm_medium") { t.Error("utm_medium not stripped") }
	if !strings.Contains(result, "real_param=keep") { t.Error("real param was stripped") }
}

func TestHard_NormalizeURL_EmptyString(t *testing.T) {
	if NormalizeURL("") != "" { t.Error("empty URL should return empty") }
}

func TestHard_NormalizeURL_MalformedURL(t *testing.T) {
	// Should not panic on malformed URLs
	result := NormalizeURL("not a url at all")
	if result == "" { t.Error("should return something for non-empty input") }
}

// ── Intent Detection — must be accurate for pipeline to work ──

func TestHard_InferIntent_Comparison(t *testing.T) {
	cases := map[string]string{
		"React vs Vue":                    "comparison",
		"difference between Go and Rust":  "comparison",
		"Python compared to JavaScript":   "comparison",
		"Svelte versus React":             "comparison",
	}
	for topic, expected := range cases {
		got := InferIntent(topic)
		if got != expected { t.Errorf("InferIntent(%q) = %q, want %q", topic, got, expected) }
	}
}

func TestHard_InferIntent_Prediction(t *testing.T) {
	cases := map[string]string{
		"Will Bitcoin hit 100k":           "prediction",
		"odds of Trump winning":           "prediction",
		"forecast for AI market 2027":     "prediction",
	}
	for topic, expected := range cases {
		got := InferIntent(topic)
		if got != expected { t.Errorf("InferIntent(%q) = %q, want %q", topic, got, expected) }
	}
}

func TestHard_InferIntent_HowTo(t *testing.T) {
	cases := map[string]string{
		"how to deploy Go on AWS":         "how_to",
		"tutorial for Kubernetes":         "how_to",
		"step by step guide to Docker":    "how_to",
	}
	for topic, expected := range cases {
		got := InferIntent(topic)
		if got != expected { t.Errorf("InferIntent(%q) = %q, want %q", topic, got, expected) }
	}
}

func TestHard_InferIntent_EmptyString(t *testing.T) {
	got := InferIntent("")
	if got == "" { t.Error("empty topic should still return a default intent") }
}

func TestHard_InferIntent_DefaultIsBreakingNews(t *testing.T) {
	// Unrecognized topics should default to breaking_news
	got := InferIntent("Qorven AI platform")
	if got != "breaking_news" { t.Errorf("default intent should be breaking_news, got %q", got) }
}

// ── Engagement Scoring — must handle edge cases ──

func TestHard_EngagementScore_ZeroEngagement(t *testing.T) {
	score := computeEngagementScore(map[string]float64{})
	if score != 0 { t.Errorf("empty engagement should be 0, got %f", score) }
}

func TestHard_EngagementScore_NilMap(t *testing.T) {
	score := computeEngagementScore(nil)
	if score != 0 { t.Errorf("nil engagement should be 0, got %f", score) }
}

func TestHard_EngagementScore_HighEngagement(t *testing.T) {
	score := computeEngagementScore(map[string]float64{"likes": 50000, "comments": 5000})
	if score < 0.7 { t.Errorf("50K likes + 5K comments should score > 0.7, got %f", score) }
	if score > 1.0 { t.Errorf("score should never exceed 1.0, got %f", score) }
}

func TestHard_EngagementScore_PolymarketVolume(t *testing.T) {
	// $100K volume should score high
	score := computeEngagementScore(map[string]float64{"volume": 100000})
	if score < 0.3 { t.Errorf("$100K volume should score > 0.3, got %f", score) }
}

func TestHard_EngagementScore_CommentsWeightedHigher(t *testing.T) {
	// 100 comments should score higher than 100 likes (comments = 2.5x weight)
	likesOnly := computeEngagementScore(map[string]float64{"likes": 100})
	commentsOnly := computeEngagementScore(map[string]float64{"comments": 100})
	if commentsOnly <= likesOnly { t.Errorf("comments should score higher: likes=%f, comments=%f", likesOnly, commentsOnly) }
}

// ── Freshness — time-sensitive scoring ──

func TestHard_Freshness_RecentIsHighest(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)

	f1 := computeFreshness(&hourAgo)
	f2 := computeFreshness(&dayAgo)
	f3 := computeFreshness(&weekAgo)
	f4 := computeFreshness(&monthAgo)

	if f1 <= f2 { t.Errorf("1h ago (%d) should score higher than 1d ago (%d)", f1, f2) }
	if f2 <= f3 { t.Errorf("1d ago (%d) should score higher than 1w ago (%d)", f2, f3) }
	if f3 <= f4 { t.Errorf("1w ago (%d) should score higher than 1m ago (%d)", f3, f4) }
}

func TestHard_Freshness_NilTime(t *testing.T) {
	if computeFreshness(nil) != 0 { t.Error("nil time should return 0 freshness") }
}

// ── Relevance Scoring ──

func TestHard_Relevance_ExactMatch(t *testing.T) {
	score := TokenOverlapRelevance("Go programming language", "Go programming language tutorial")
	if score < 0.8 { t.Errorf("exact match should score > 0.8, got %f", score) }
}

func TestHard_Relevance_NoMatch(t *testing.T) {
	score := TokenOverlapRelevance("Go programming", "Python machine learning")
	if score > 0.3 { t.Errorf("no overlap should score < 0.3, got %f", score) }
}

func TestHard_Relevance_StopwordsIgnored(t *testing.T) {
	// "the" and "is" are stopwords — should not count as matches
	score := TokenOverlapRelevance("the is a", "the is a test")
	if score > 0.5 { t.Errorf("stopwords-only query should score low, got %f", score) }
}

func TestHard_Relevance_LowSignalDetection(t *testing.T) {
	if !IsLowSignalMatch("odds review", "some random text about odds and review") {
		t.Error("'odds review' should be detected as low-signal match")
	}
	if IsLowSignalMatch("Qorven AI platform", "Qorven AI platform is great") {
		t.Error("'Qorven AI platform' should NOT be low-signal")
	}
}

// ── Deduplication ──

func TestHard_ContentSimilarity_Identical(t *testing.T) {
	score := ContentSimilarity("Go is a great programming language", "Go is a great programming language")
	if score < 0.99 { t.Errorf("identical text should score ~1.0, got %f", score) }
}

func TestHard_ContentSimilarity_CompletelyDifferent(t *testing.T) {
	score := ContentSimilarity("Go programming language", "Python machine learning framework")
	if score > 0.3 { t.Errorf("completely different should score < 0.3, got %f", score) }
}

func TestHard_ContentSimilarity_EmptyStrings(t *testing.T) {
	if ContentSimilarity("", "") != 0 { t.Error("empty strings should return 0") }
	if ContentSimilarity("hello", "") != 0 { t.Error("one empty should return 0") }
}

func TestHard_DedupeByURL_RemovesDuplicates(t *testing.T) {
	items := []SourceItem{
		{ItemID: "1", URL: "https://www.reddit.com/r/golang/abc"},
		{ItemID: "2", URL: "https://reddit.com/r/golang/abc"},  // same after normalization
		{ItemID: "3", URL: "https://news.ycombinator.com/item?id=123"},
	}
	deduped := DedupeByURL(items)
	if len(deduped) != 2 { t.Errorf("expected 2 after dedup, got %d", len(deduped)) }
}

// ── Entity Extraction ──

func TestHard_ExtractEntities_TwitterHandle(t *testing.T) {
	entities := ExtractEntities("What is @elonmusk saying about AI?")
	found := false
	for _, e := range entities {
		if e.Handles["twitter"] == "@elonmusk" { found = true }
	}
	if !found { t.Error("should extract @elonmusk as Twitter handle") }
}

func TestHard_ExtractEntities_GitHubRepo(t *testing.T) {
	entities := ExtractEntities("Check out golang/go repository")
	found := false
	for _, e := range entities {
		if e.Handles["github"] == "golang/go" { found = true }
	}
	if !found { t.Error("should extract golang/go as GitHub repo") }
}

// ── Planner ──

func TestHard_FallbackPlan_AlwaysProducesSubqueries(t *testing.T) {
	plan := fallbackPlan("AI agents", []string{"reddit", "hackernews", "github"}, nil, "default", "test")
	if len(plan.SubQueries) == 0 { t.Fatal("fallback plan must produce at least 1 subquery") }
	if plan.Intent == "" { t.Error("plan must have an intent") }
	if len(plan.SourceWeights) == 0 { t.Error("plan must have source weights") }

	// Every subquery must have sources
	for i, sq := range plan.SubQueries {
		if len(sq.Sources) == 0 { t.Errorf("subquery %d has no sources", i) }
		if sq.SearchQuery == "" { t.Errorf("subquery %d has no search query", i) }
		if sq.RankingQuery == "" { t.Errorf("subquery %d has no ranking query", i) }
	}
}

func TestHard_FallbackPlan_ComparisonGeneratesEntitySubqueries(t *testing.T) {
	plan := fallbackPlan("React vs Vue", []string{"reddit", "hackernews"}, nil, "default", "test")
	if plan.Intent != "comparison" { t.Errorf("intent should be comparison, got %q", plan.Intent) }
	// Should have entity subqueries for React and Vue
	if len(plan.SubQueries) < 2 { t.Errorf("comparison should have entity subqueries, got %d", len(plan.SubQueries)) }
}

func TestHard_FallbackPlan_PredictionIncludesPolymarket(t *testing.T) {
	plan := fallbackPlan("Will Bitcoin hit 100k", []string{"reddit", "polymarket", "hackernews"}, nil, "default", "test")
	if plan.Intent != "prediction" { t.Errorf("intent should be prediction, got %q", plan.Intent) }
	// Polymarket should have highest weight for predictions
	if plan.SourceWeights["polymarket"] <= plan.SourceWeights["reddit"] {
		t.Error("polymarket should have higher weight than reddit for predictions")
	}
}

// ── Pipeline Integration ──

func TestHard_Pipeline_EmptySources(t *testing.T) {
	p := NewPipeline()
	report, err := p.Run(context.Background(), "test", "quick")
	if err != nil { t.Fatal(err) }
	if report == nil { t.Fatal("report should not be nil even with no sources") }
	if report.Topic != "test" { t.Errorf("topic: %q", report.Topic) }
}

func TestHard_Pipeline_RetrievalBundleAddItems(t *testing.T) {
	b := NewRetrievalBundle()
	items := []SourceItem{
		{ItemID: "1", Source: "reddit", Title: "Test"},
		{ItemID: "2", Source: "reddit", Title: "Test 2"},
	}
	b.AddItems("primary", "reddit", items)

	if len(b.ItemsBySource["reddit"]) != 2 { t.Errorf("expected 2 items, got %d", len(b.ItemsBySource["reddit"])) }
	if len(b.ItemsBySourceAndQuery["primary|reddit"]) != 2 { t.Errorf("expected 2 items in query key") }
}

// ── Source Quality Weights ──

func TestHard_SourceQuality_PolymarketHighest(t *testing.T) {
	pm := sourceQuality("polymarket")
	reddit := sourceQuality("reddit")
	if pm <= reddit { t.Errorf("polymarket (%f) should rank higher than reddit (%f)", pm, reddit) }
}

func TestHard_SourceQuality_UnknownSource(t *testing.T) {
	score := sourceQuality("unknown_platform")
	if score <= 0 { t.Error("unknown source should still have a positive quality score") }
}

// ── Render ──

func TestHard_RenderReport_NotEmpty(t *testing.T) {
	report := &Report{
		Topic: "AI Agents", RangeFrom: "2026-03-10", RangeTo: "2026-04-09",
		GeneratedAt: time.Now().Format(time.RFC3339),
		RankedCandidates: []Candidate{
			{CandidateID: "c_0", Source: "reddit", Title: "AI agents are amazing", FinalScore: 0.9, Engagement: 500},
		},
		ItemsBySource: map[string][]SourceItem{"reddit": {{ItemID: "1"}}},
	}
	output := RenderReport(report)
	if !strings.Contains(output, "AI Agents") { t.Error("report should contain topic") }
	if !strings.Contains(output, "reddit") { t.Error("report should contain source") }
	if len(output) < 100 { t.Errorf("report too short: %d chars", len(output)) }
}

// ── Comparison Entity Extraction ──

func TestHard_ComparisonEntities_ReactVsVue(t *testing.T) {
	entities := comparisonEntities("React vs Vue")
	if len(entities) < 2 { t.Fatalf("expected 2 entities, got %d", len(entities)) }
	if entities[0] != "React" { t.Errorf("first entity: %q", entities[0]) }
	if entities[1] != "Vue" { t.Errorf("second entity: %q", entities[1]) }
}

func TestHard_ComparisonEntities_DifferenceBetween(t *testing.T) {
	entities := comparisonEntities("difference between Go and Rust")
	if len(entities) < 2 { t.Fatalf("expected 2 entities, got %d", len(entities)) }
}

func TestHard_ComparisonEntities_ThreeWay(t *testing.T) {
	entities := comparisonEntities("React vs Vue vs Svelte")
	if len(entities) < 3 { t.Errorf("expected 3 entities, got %d: %v", len(entities), entities) }
}
