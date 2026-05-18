// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/search"
)

type ResearchTool struct {
	pipeline *search.Pipeline
}

func NewResearchTool(pipeline *search.Pipeline) *ResearchTool {
	return &ResearchTool{pipeline: pipeline}
}

func (t *ResearchTool) Name() string { return "research" }
func (t *ResearchTool) Description() string {
	return "Deep web research — searches multiple sources, fetches page content, synthesizes findings. Use for multi-source questions requiring actual article content."
}
func (t *ResearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Research question"},
			"depth": map[string]any{"type": "string", "enum": []string{"quick", "standard", "deep"}, "description": "Research depth (quick=3 sources, standard=5, deep=8)"},
		},
		"required": []string{"query"},
	}
}

func (t *ResearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	query, _ := args["query"].(string)
	depth, _ := args["depth"].(string)
	if query == "" {
		return ErrorResult("query is required")
	}
	if depth == "" {
		depth = "standard"
	}

	maxSources := map[string]int{"quick": 3, "standard": 5, "deep": 8}[depth]
	if maxSources == 0 {
		maxSources = 5
	}

	if t.pipeline == nil {
		return ErrorResult("research: search pipeline not configured")
	}

	// SearchAndFetch: search → fetch top pages → extract content
	results, rewritten, err := t.pipeline.SearchAndFetch(ctx, query, maxSources)
	if err != nil {
		return ErrorResult("research search failed: " + err.Error())
	}
	if len(results) == 0 {
		return ErrorResult("no search results found for: " + query)
	}

	var report strings.Builder
	fmt.Fprintf(&report, "# Research: %s\n", query)
	if rewritten != query {
		fmt.Fprintf(&report, "Search query used: %s\n", rewritten)
	}
	fmt.Fprintf(&report, "Depth: %s | Sources: %d\n\n", depth, len(results))

	for i, r := range results {
		fmt.Fprintf(&report, "## %d. %s\n%s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			snip := r.Snippet
			if len(snip) > 3000 { snip = snip[:3000] + "…" }
			fmt.Fprintf(&report, "\n%s\n", snip)
		} else {
			report.WriteString("\n[Content not extractable — use web_fetch on this URL for full text]\n")
		}
		report.WriteString("\n")
	}

	result := report.String()
	if len(result) > MaxToolOutput {
		result = result[:MaxToolOutput]
	}
	return TextResult(result)
}

type sourceResult struct {
	Title, Snippet, Query string
}

func expandQueries(query string, n int) []string {
	queries := []string{query}
	if n >= 5 {
		queries = append(queries, query+" latest 2026", query+" comparison")
	}
	if n >= 8 {
		queries = append(queries, query+" pros cons", query+" expert opinion")
	}
	return queries
}
