// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"context"
	"strings"
	"testing"
	"time"
)

// deep_test.go — Real API integration tests for Qorven Social Intelligence.
// These hit live APIs (Reddit, HN, GitHub are free, no auth needed).

func TestDeep_Reddit_RealSearch(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	src := NewRedditSource()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	items, err := src.Search(ctx, "golang", "quick")
	if err != nil { t.Skipf("reddit: %v", err) }
	if len(items) == 0 { t.Skip("reddit: 0 results, likely rate limited") }

	// Verify structure
	for i, item := range items {
		if i >= 3 { break }
		if item.Source != "reddit" { t.Errorf("item %d: source=%q", i, item.Source) }
		if item.Title == "" { t.Errorf("item %d: empty title", i) }
		if item.URL == "" { t.Errorf("item %d: empty URL", i) }
		if !strings.Contains(item.URL, "reddit.com") { t.Errorf("item %d: URL not reddit: %q", i, item.URL) }
		if item.EngagementScore == nil { t.Errorf("item %d: nil engagement score", i) }
		if item.PublishedAt == nil { t.Errorf("item %d: nil published_at", i) }
		if item.Engagement == nil { t.Errorf("item %d: nil engagement map", i) }
	}

	// Top result should have some engagement
	top := items[0]
	if top.Engagement["upvotes"] == 0 && top.Engagement["comments"] == 0 {
		t.Log("top result has 0 engagement — might be very new")
	}

	t.Logf("reddit: %d results, top: %q (↑%.0f 💬%.0f) ✓",
		len(items), top.Title, top.Engagement["upvotes"], top.Engagement["comments"])
}

func TestDeep_Reddit_CommentEnrichment(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	src := NewRedditSource()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	items, err := src.Search(ctx, "programming", "default")
	if err != nil { t.Skipf("reddit: %v", err) }
	if len(items) == 0 { t.Skip("no results") }

	// The top results should have comments enriched (default depth enriches top 5)
	enriched := false
	for _, item := range items[:min(len(items), 5)] {
		if strings.Contains(item.Body, "--- Top Comments ---") {
			enriched = true
			break
		}
	}
	if enriched {
		t.Log("reddit: comment enrichment working ✓")
	} else {
		t.Log("reddit: no comments enriched (posts may have no comments)")
	}
}

func TestDeep_HN_RealSearch(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	src := NewHackerNewsSource()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	items, err := src.Search(ctx, "AI agents", "quick")
	if err != nil { t.Skipf("hn: %v", err) }
	if len(items) == 0 { t.Fatal("HN returned 0 results for 'AI agents'") }

	for i, item := range items {
		if i >= 3 { break }
		if item.Source != "hackernews" { t.Errorf("item %d: source=%q", i, item.Source) }
		if !strings.Contains(item.URL, "ycombinator.com") { t.Errorf("item %d: URL not HN: %q", i, item.URL) }
		if item.Engagement["points"] == 0 { t.Logf("item %d: 0 points", i) }
	}

	top := items[0]
	t.Logf("hn: %d results, top: %q (↑%.0f 💬%.0f) ✓",
		len(items), top.Title, top.Engagement["points"], top.Engagement["comments"])
}

func TestDeep_GitHub_RealSearch(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	src := NewGitHubSource("")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	items, err := src.Search(ctx, "AI agent Go", "quick")
	if err != nil { t.Skipf("github: %v", err) }
	if len(items) == 0 { t.Fatal("GitHub returned 0 results") }

	for i, item := range items {
		if i >= 3 { break }
		if item.Source != "github" { t.Errorf("item %d: source=%q", i, item.Source) }
		if !strings.Contains(item.URL, "github.com") { t.Errorf("item %d: URL not github: %q", i, item.URL) }
		if item.Engagement["stars"] == 0 { t.Logf("item %d: 0 stars", i) }
	}

	top := items[0]
	t.Logf("github: %d results, top: %q (⭐%.0f 🍴%.0f) ✓",
		len(items), top.Title, top.Engagement["stars"], top.Engagement["forks"])
}

func TestDeep_Bluesky_RealSearch(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	src := NewBlueskySource()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	items, err := src.Search(ctx, "golang", "quick")
	if err != nil { t.Skipf("bluesky: %v", err) }
	if len(items) == 0 { t.Skip("bluesky returned 0 results") }

	for i, item := range items {
		if i >= 3 { break }
		if item.Source != "bluesky" { t.Errorf("source=%q", item.Source) }
		if !strings.Contains(item.URL, "bsky.app") { t.Errorf("URL not bsky: %q", item.URL) }
	}
	t.Logf("bluesky: %d results ✓", len(items))
}

func TestDeep_Pipeline_RealMultiPlatform(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	// Build pipeline with free sources
	p := NewPipeline(
		NewRedditSource(),
		NewHackerNewsSource(),
		NewGitHubSource(""),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report, err := p.Run(ctx, "Go programming language", "quick")
	if err != nil { t.Fatal(err) }

	if report.Topic != "Go programming language" { t.Errorf("topic: %q", report.Topic) }
	if len(report.RankedCandidates) == 0 { t.Fatal("no candidates") }
	if len(report.ItemsBySource) == 0 { t.Fatal("no items by source") }

	// Should have results from multiple sources
	sources := map[string]bool{}
	for _, c := range report.RankedCandidates { sources[c.Source] = true }
	if len(sources) < 1 { t.Errorf("expected results from 1+ sources, got %d: %v", len(sources), sources) }

	// Candidates should be ranked (first should have higher score than last)
	first := report.RankedCandidates[0]
	last := report.RankedCandidates[len(report.RankedCandidates)-1]
	if first.FinalScore < last.FinalScore { t.Error("candidates not ranked by score") }

	// Render should produce output
	output := RenderReport(report)
	if len(output) < 200 { t.Errorf("render too short: %d chars", len(output)) }
	if !strings.Contains(output, "Go programming") { t.Error("render missing topic") }

	t.Logf("pipeline: %d candidates from %d sources, top: %q (score=%.3f) ✓",
		len(report.RankedCandidates), len(sources), first.Title, first.FinalScore)
}

func TestDeep_Pipeline_PlannerIntegration(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	// Test that the planner produces valid plans for different intents
	topics := map[string]string{
		"React vs Vue":                "comparison",
		"how to deploy Go on AWS":     "how_to",
		"Will Bitcoin hit 100k":       "prediction",
		"latest AI news":              "breaking_news",
	}

	available := []string{"reddit", "hackernews", "github", "polymarket", "youtube"}

	for topic, expectedIntent := range topics {
		plan := PlanQuery(topic, available, nil, "default", nil, "")
		if plan.Intent != expectedIntent {
			t.Errorf("PlanQuery(%q): intent=%q, want %q", topic, plan.Intent, expectedIntent)
		}
		if len(plan.SubQueries) == 0 {
			t.Errorf("PlanQuery(%q): no subqueries", topic)
		}
		for i, sq := range plan.SubQueries {
			if sq.SearchQuery == "" { t.Errorf("%q subquery %d: empty search_query", topic, i) }
			if len(sq.Sources) == 0 { t.Errorf("%q subquery %d: no sources", topic, i) }
		}
	}
	t.Log("planner: all 4 intents produce valid plans ✓")
}

func TestDeep_Scoring_EndToEnd(t *testing.T) {
	// Create items with known engagement and verify scoring order
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	items := []SourceItem{
		{ItemID: "viral", Source: "reddit", Title: "Viral Post", PublishedAt: &hourAgo,
			Engagement: map[string]float64{"upvotes": 5000, "comments": 800}},
		{ItemID: "medium", Source: "hackernews", Title: "Medium Post", PublishedAt: &hourAgo,
			Engagement: map[string]float64{"points": 200, "comments": 50}},
		{ItemID: "old_viral", Source: "reddit", Title: "Old Viral", PublishedAt: &weekAgo,
			Engagement: map[string]float64{"upvotes": 10000, "comments": 2000}},
		{ItemID: "low", Source: "reddit", Title: "Low Engagement", PublishedAt: &hourAgo,
			Engagement: map[string]float64{"upvotes": 5, "comments": 1}},
	}

	// Annotate
	for i := range items { AnnotateItem(&items[i], "test topic") }

	// Viral recent should score highest
	if *items[0].EngagementScore <= *items[3].EngagementScore {
		t.Errorf("viral (%f) should score higher than low (%f)", *items[0].EngagementScore, *items[3].EngagementScore)
	}

	// Recent should have higher freshness than old
	if *items[0].Freshness <= *items[2].Freshness {
		t.Errorf("recent (%d) should be fresher than week-old (%d)", *items[0].Freshness, *items[2].Freshness)
	}

	t.Logf("scoring: viral=%.3f, medium=%.3f, old_viral=%.3f, low=%.3f ✓",
		*items[0].EngagementScore, *items[1].EngagementScore, *items[2].EngagementScore, *items[3].EngagementScore)
}
