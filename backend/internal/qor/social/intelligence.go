// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package social

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// intelligence.go — Qorven Social Intelligence Engine.
// Searches multiple platforms in parallel, scores by real engagement,
// and synthesizes results into a grounded brief.

// IntelligenceEngine orchestrates multi-platform search and synthesis.
type IntelligenceEngine struct {
	registry  *ReaderRegistry
	synthesize func(ctx context.Context, query string, results []ScoredResult) (string, error)
}

// NewIntelligenceEngine creates an engine with all available readers.
func NewIntelligenceEngine(githubToken string, synthesizeFn func(ctx context.Context, query string, results []ScoredResult) (string, error)) *IntelligenceEngine {
	client := &http.Client{Timeout: 15 * time.Second}
	reg := NewReaderRegistry()
	reg.Register(NewRedditReader(client))
	reg.Register(NewHNReader(client))
	reg.Register(NewGitHubReader(client, githubToken))
	reg.Register(NewRSSReader(client))
	return &IntelligenceEngine{registry: reg, synthesize: synthesizeFn}
}

// SearchAll queries all available platforms in parallel and returns scored, ranked results.
func (e *IntelligenceEngine) SearchAll(ctx context.Context, query string, opts SearchOpts) ([]ScoredResult, error) {
	platforms := e.registry.Available()
	if len(platforms) == 0 { return nil, fmt.Errorf("no platforms available") }

	type platformResult struct {
		platform Platform
		results  []ScoredResult
		err      error
	}

	ch := make(chan platformResult, len(platforms))
	var wg sync.WaitGroup

	for _, p := range platforms {
		if p == PlatformRSS { continue } // RSS needs explicit feed URL
		wg.Add(1)
		go func(platform Platform) {
			defer wg.Done()
			reader, _ := e.registry.Get(platform)
			results, err := reader.Search(ctx, query, opts)
			ch <- platformResult{platform: platform, results: results, err: err}
		}(p)
	}

	go func() { wg.Wait(); close(ch) }()

	var all []ScoredResult
	for pr := range ch {
		if pr.err != nil {
			slog.Warn("social.search.error", "platform", pr.platform, "error", pr.err)
			continue
		}
		slog.Info("social.search.results", "platform", pr.platform, "count", len(pr.results))
		all = append(all, pr.results...)
	}

	// Score and rank
	rankResults(all, query)
	return all, nil
}

// SearchPlatforms queries specific platforms only.
func (e *IntelligenceEngine) SearchPlatforms(ctx context.Context, query string, platforms []Platform, opts SearchOpts) ([]ScoredResult, error) {
	type platformResult struct {
		results []ScoredResult
		err     error
	}

	ch := make(chan platformResult, len(platforms))
	var wg sync.WaitGroup

	for _, p := range platforms {
		reader, ok := e.registry.Get(p)
		if !ok { continue }
		wg.Add(1)
		go func(r PlatformReader) {
			defer wg.Done()
			results, err := r.Search(ctx, query, opts)
			ch <- platformResult{results: results, err: err}
		}(reader)
	}

	go func() { wg.Wait(); close(ch) }()

	var all []ScoredResult
	for pr := range ch {
		if pr.err != nil { continue }
		all = append(all, pr.results...)
	}

	rankResults(all, query)
	return all, nil
}

// Synthesize searches all platforms and produces an AI-generated brief.
func (e *IntelligenceEngine) Synthesize(ctx context.Context, query string, opts SearchOpts) (string, []ScoredResult, error) {
	results, err := e.SearchAll(ctx, query, opts)
	if err != nil { return "", nil, err }
	if len(results) == 0 { return "No results found across any platform.", nil, nil }

	// Take top 20 for synthesis
	top := results
	if len(top) > 20 { top = top[:20] }

	if e.synthesize == nil {
		// No LLM — return formatted results
		return formatResults(query, top), top, nil
	}

	brief, err := e.synthesize(ctx, query, top)
	if err != nil {
		// Fallback to formatted results
		return formatResults(query, top), top, nil
	}
	return brief, results, nil
}

// ReadItem reads a single item from a specific platform.
func (e *IntelligenceEngine) ReadItem(ctx context.Context, platform Platform, id string) (*ScoredResult, error) {
	reader, ok := e.registry.Get(platform)
	if !ok { return nil, fmt.Errorf("platform %s not available", platform) }
	return reader.Read(ctx, id)
}

// Registry returns the underlying reader registry for custom configuration.
func (e *IntelligenceEngine) Registry() *ReaderRegistry { return e.registry }

// rankResults computes final scores and sorts by engagement.
func rankResults(results []ScoredResult, query string) {
	queryWords := strings.Fields(strings.ToLower(query))

	for i := range results {
		r := &results[i]

		// Relevance: how many query words appear in title+content
		text := strings.ToLower(r.Title + " " + r.Content)
		matches := 0
		for _, w := range queryWords {
			if strings.Contains(text, w) { matches++ }
		}
		if len(queryWords) > 0 {
			r.FinalScore = r.EngagementScore*0.6 + (float64(matches)/float64(len(queryWords)))*0.3
		} else {
			r.FinalScore = r.EngagementScore
		}

		// Recency boost: content from last 7 days gets a 10% boost
		if time.Since(r.PublishedAt) < 7*24*time.Hour {
			r.FinalScore *= 1.1
		}
		// Content from last 24h gets 20% boost
		if time.Since(r.PublishedAt) < 24*time.Hour {
			r.FinalScore *= 1.1
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i].FinalScore > results[j].FinalScore })
}

// formatResults creates a human-readable summary without LLM.
func formatResults(query string, results []ScoredResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Social Intelligence: %q\n\n", query))
	b.WriteString(fmt.Sprintf("Found %d results across platforms.\n\n", len(results)))

	// Group by platform
	byPlatform := map[Platform][]ScoredResult{}
	for _, r := range results { byPlatform[r.Platform] = append(byPlatform[r.Platform], r) }

	for platform, items := range byPlatform {
		b.WriteString(fmt.Sprintf("### %s (%d results)\n\n", platform, len(items)))
		for i, r := range items {
			if i >= 5 { break }
			engagement := ""
			if r.Upvotes > 0 { engagement += fmt.Sprintf("↑%d ", r.Upvotes) }
			if r.Likes > 0 { engagement += fmt.Sprintf("♥%d ", r.Likes) }
			if r.Comments > 0 { engagement += fmt.Sprintf("💬%d ", r.Comments) }
			if r.Views > 0 { engagement += fmt.Sprintf("👁%d ", r.Views) }

			b.WriteString(fmt.Sprintf("- **%s** %s\n  %s\n  %s\n\n",
				r.Title, engagement, truncate(r.Content, 200), r.URL))
		}
	}
	return b.String()
}

// BuildSynthesisPrompt creates the prompt for LLM synthesis of social results.
func BuildSynthesisPrompt(query string, results []ScoredResult) string {
	var b strings.Builder
	b.WriteString("You are a social intelligence analyst. Synthesize the following results from multiple platforms into a concise, grounded brief.\n\n")
	b.WriteString(fmt.Sprintf("Query: %q\n\n", query))
	b.WriteString("Results (sorted by engagement score):\n\n")

	for i, r := range results {
		engagement := ""
		if r.Upvotes > 0 { engagement += fmt.Sprintf("↑%d ", r.Upvotes) }
		if r.Likes > 0 { engagement += fmt.Sprintf("♥%d ", r.Likes) }
		if r.Comments > 0 { engagement += fmt.Sprintf("💬%d ", r.Comments) }

		b.WriteString(fmt.Sprintf("[%d] [%s] %s %s(score: %.2f)\n%s\nURL: %s\n\n",
			i+1, r.Platform, r.Title, engagement, r.FinalScore,
			truncate(r.Content, 500), r.URL))
	}

	b.WriteString("Instructions:\n")
	b.WriteString("1. Identify the key themes and consensus across platforms\n")
	b.WriteString("2. Highlight the highest-engagement signals (most upvoted, most discussed)\n")
	b.WriteString("3. Note any disagreements or contrarian views\n")
	b.WriteString("4. Cite specific results by [number] and platform\n")
	b.WriteString("5. Keep it under 500 words\n")
	b.WriteString("6. End with 'Key Signals' bullet points\n")

	return b.String()
}
