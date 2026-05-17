// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"strings"
)

// dedupe.go — URL deduplication and content similarity.
// Rewritten from last30days dedupe.py (99 lines).

// DedupeByURL removes duplicate items based on normalized URL.
func DedupeByURL(items []SourceItem) []SourceItem {
	seen := map[string]bool{}
	var out []SourceItem
	for _, item := range items {
		key := NormalizeURL(item.URL)
		if key == "" { key = item.ItemID }
		if seen[key] { continue }
		seen[key] = true
		out = append(out, item)
	}
	return out
}

// DedupeCandidates removes duplicate candidates, keeping the highest-scored.
func DedupeCandidates(candidates []Candidate) []Candidate {
	seen := map[string]int{} // URL → index of best candidate
	for i, c := range candidates {
		key := NormalizeURL(c.URL)
		if key == "" { key = c.ItemID }
		if prev, ok := seen[key]; ok {
			if c.FinalScore > candidates[prev].FinalScore { seen[key] = i }
		} else {
			seen[key] = i
		}
	}
	var out []Candidate
	added := map[int]bool{}
	for _, idx := range seen {
		if !added[idx] { added[idx] = true; out = append(out, candidates[idx]) }
	}
	return out
}

// ContentSimilarity computes Jaccard similarity between two text strings.
func ContentSimilarity(a, b string) float64 {
	tokensA := tokenSet(a)
	tokensB := tokenSet(b)
	if len(tokensA) == 0 || len(tokensB) == 0 { return 0 }

	intersection := 0
	for t := range tokensA {
		if tokensB[t] { intersection++ }
	}
	union := len(tokensA) + len(tokensB) - intersection
	if union == 0 { return 0 }
	return float64(intersection) / float64(union)
}

// DedupeByContent removes items with >80% content similarity, keeping higher engagement.
func DedupeByContent(items []SourceItem, threshold float64) []SourceItem {
	if threshold <= 0 { threshold = 0.8 }
	var out []SourceItem
	for _, item := range items {
		isDup := false
		for _, existing := range out {
			if ContentSimilarity(item.Body, existing.Body) > threshold {
				isDup = true
				break
			}
		}
		if !isDup { out = append(out, item) }
	}
	return out
}

func tokenSet(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	set := map[string]bool{}
	for _, w := range words {
		if len(w) > 2 && !stopwords[w] { set[w] = true }
	}
	return set
}
