// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"fmt"
	"sort"
	"strings"
)

// render.go — Output formatting, citation rendering, cluster display.
// Rewritten from last30days render.py (642 lines).

// RenderReport produces a formatted markdown report from a Report.
func RenderReport(report *Report) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# %s\n\n", report.Topic))
	b.WriteString(fmt.Sprintf("*%s to %s | Generated %s*\n\n", report.RangeFrom, report.RangeTo, report.GeneratedAt))

	// Summary stats
	totalItems := 0
	for _, items := range report.ItemsBySource { totalItems += len(items) }
	b.WriteString(fmt.Sprintf("**%d results** from %d sources, %d clusters\n\n---\n\n",
		len(report.RankedCandidates), len(report.ItemsBySource), len(report.Clusters)))

	// Render clusters
	if len(report.Clusters) > 0 {
		for i, cluster := range report.Clusters {
			b.WriteString(fmt.Sprintf("## %d. %s\n\n", i+1, cluster.Title))
			b.WriteString(fmt.Sprintf("*Sources: %s*\n\n", strings.Join(cluster.Sources, ", ")))

			// Render representative candidates
			for _, cid := range cluster.RepresentativeIDs {
				c := findCandidate(report.RankedCandidates, cid)
				if c == nil { continue }
				b.WriteString(renderCandidate(c, i+1))
			}
			b.WriteString("\n")
		}
	} else {
		// No clusters — render top candidates directly
		limit := 20
		if len(report.RankedCandidates) < limit { limit = len(report.RankedCandidates) }
		for i, c := range report.RankedCandidates[:limit] {
			b.WriteString(renderCandidate(&c, i+1))
		}
	}

	// Source breakdown
	b.WriteString("\n---\n\n## Sources\n\n")
	for source, items := range report.ItemsBySource {
		b.WriteString(fmt.Sprintf("- **%s**: %d items\n", source, len(items)))
	}

	// Errors
	if len(report.ErrorsBySource) > 0 {
		b.WriteString("\n## Errors\n\n")
		for source, err := range report.ErrorsBySource {
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", source, err))
		}
	}

	// Warnings
	if len(report.Warnings) > 0 {
		b.WriteString("\n## Warnings\n\n")
		for _, w := range report.Warnings { b.WriteString(fmt.Sprintf("- %s\n", w)) }
	}

	return b.String()
}

func renderCandidate(c *Candidate, num int) string {
	var b strings.Builder

	// Engagement badge
	engBadge := ""
	if c.Engagement > 1000 { engBadge = " 🔥" }
	if c.Engagement > 10000 { engBadge = " 🔥🔥" }

	b.WriteString(fmt.Sprintf("### [%d] %s%s\n", num, c.Title, engBadge))
	b.WriteString(fmt.Sprintf("*[%s] Score: %.2f | Engagement: %.0f*\n\n", c.Source, c.FinalScore, c.Engagement))

	if c.Snippet != "" { b.WriteString(c.Snippet + "\n\n") }
	if c.URL != "" { b.WriteString(fmt.Sprintf("[→ %s](%s)\n\n", c.Source, c.URL)) }

	return b.String()
}

// RenderBrief produces a concise brief (for LLM synthesis output).
func RenderBrief(topic string, candidates []Candidate) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Brief: %s\n\n", topic))

	// Group by source
	bySource := map[string][]Candidate{}
	for _, c := range candidates { bySource[c.Source] = append(bySource[c.Source], c) }

	// Sort sources by total engagement
	type sourceGroup struct {
		name       string
		candidates []Candidate
		totalEng   float64
	}
	var groups []sourceGroup
	for name, cands := range bySource {
		total := 0.0
		for _, c := range cands { total += c.Engagement }
		groups = append(groups, sourceGroup{name, cands, total})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].totalEng > groups[j].totalEng })

	for _, g := range groups {
		b.WriteString(fmt.Sprintf("### %s (%d results)\n\n", g.name, len(g.candidates)))
		limit := 5
		if len(g.candidates) < limit { limit = len(g.candidates) }
		for i, c := range g.candidates[:limit] {
			engStr := formatEngagement(c)
			b.WriteString(fmt.Sprintf("%d. **%s** %s\n   %s\n   %s\n\n", i+1, c.Title, engStr, truncateStr(c.Snippet, 150), c.URL))
		}
	}

	// Key signals
	b.WriteString("### Key Signals\n\n")
	topN := candidates
	if len(topN) > 5 { topN = topN[:5] }
	for _, c := range topN {
		b.WriteString(fmt.Sprintf("- [%s] %s (%.0f engagement)\n", c.Source, truncateStr(c.Title, 80), c.Engagement))
	}

	return b.String()
}

func formatEngagement(c Candidate) string {
	parts := []string{}
	for _, item := range c.SourceItems {
		for key, val := range item.Engagement {
			if val > 0 {
				switch key {
				case "upvotes", "points", "likes", "stars": parts = append(parts, fmt.Sprintf("↑%.0f", val))
				case "comments", "replies", "num_comments": parts = append(parts, fmt.Sprintf("💬%.0f", val))
				case "views": parts = append(parts, fmt.Sprintf("👁%.0f", val))
				case "volume": parts = append(parts, fmt.Sprintf("$%.0f", val))
				case "retweets", "reposts", "shares", "forks": parts = append(parts, fmt.Sprintf("🔄%.0f", val))
				}
			}
		}
	}
	if len(parts) == 0 { return "" }
	return "(" + strings.Join(parts, " ") + ")"
}

func findCandidate(candidates []Candidate, id string) *Candidate {
	for i := range candidates {
		if candidates[i].CandidateID == id { return &candidates[i] }
	}
	return nil
}
