// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"encoding/json"
	"fmt"
	"strings"
)

// rerank.go — LLM-based reranking with intent-specific scoring hints.
// Rewritten from last30days rerank.py (296 lines).

var IntentScoringHints = map[string]string{
	"comparison": "Prefer items that directly compare, contrast, or benchmark the entities. Head-to-head comparisons score higher.",
	"how_to":     "Prefer step-by-step guides, tutorials, and practical implementations. Working code examples score highest.",
	"opinion":    "Prefer items with strong, well-argued positions. Upvoted discussions with nuanced takes score higher.",
	"prediction": "Prefer items with concrete odds, forecasts, or data-backed predictions. Real money signals (Polymarket) score highest.",
	"breaking_news": "Prefer the most recent, most-discussed items. First-hand reports and primary sources score higher.",
	"factual":    "Prefer authoritative sources with verifiable claims. Documentation and official announcements score highest.",
	"product":    "Prefer hands-on reviews, benchmarks, and real user experiences. Paid reviews score lower.",
	"concept":    "Prefer clear explanations with examples. Academic papers and well-written blog posts score higher.",
}

// RerankWithLLM uses an LLM to rerank candidates based on intent-specific criteria.
func RerankWithLLM(provider ReasoningClient, model string, candidates []Candidate, topic string, intent string) []Candidate {
	if provider == nil || len(candidates) == 0 { return candidates }

	// Build reranking prompt
	prompt := buildRerankPrompt(candidates, topic, intent)

	raw, err := provider.GenerateJSON(model, prompt)
	if err != nil { return candidates } // fallback to existing order

	// Parse rerank scores
	rankings, ok := raw["rankings"].([]any)
	if !ok { return candidates }

	scoreMap := map[string]float64{}
	for _, r := range rankings {
		rm, ok := r.(map[string]any)
		if !ok { continue }
		id := getString(rm, "id")
		score := toFloat(rm["score"])
		if id != "" { scoreMap[id] = score }
	}

	// Apply rerank scores
	for i := range candidates {
		if score, ok := scoreMap[candidates[i].CandidateID]; ok {
			candidates[i].RerankScore = &score
			candidates[i].FinalScore = candidates[i].FinalScore*0.4 + score*0.6
		}
	}

	return candidates
}

func buildRerankPrompt(candidates []Candidate, topic, intent string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Rerank these %d results for the topic: %q\n\n", len(candidates), topic))

	hint := IntentScoringHints[intent]
	if hint != "" { b.WriteString("Scoring guidance: " + hint + "\n\n") }

	b.WriteString("Results:\n")
	for _, c := range candidates {
		engStr := ""
		if c.Engagement > 0 { engStr = fmt.Sprintf(" (engagement: %.0f)", c.Engagement) }
		b.WriteString(fmt.Sprintf("- ID: %s | [%s] %s%s\n  %s\n\n", c.CandidateID, c.Source, c.Title, engStr, truncateStr(c.Snippet, 200)))
	}

	b.WriteString(`Return JSON: {"rankings": [{"id": "c_0", "score": 0.95, "reason": "why"}]}
Score 0.0-1.0. Higher = more relevant to the topic and intent.`)
	return b.String()
}

// RerankByEngagement is a non-LLM fallback that reranks purely by engagement.
func RerankByEngagement(candidates []Candidate) []Candidate {
	for i := range candidates {
		engScore := normalizeEng(candidates[i].Engagement)
		candidates[i].RerankScore = &engScore
		candidates[i].FinalScore = candidates[i].RRFScore*0.2 + candidates[i].LocalRelevance*0.2 +
			engScore*0.4 + float64(candidates[i].Freshness)*0.001*0.1 + candidates[i].SourceQuality*0.1
	}
	return candidates
}

// ParseRerankResponse parses the LLM rerank response.
func ParseRerankResponse(data []byte) map[string]float64 {
	var resp struct{ Rankings []struct{ ID string; Score float64 } }
	json.Unmarshal(data, &resp)
	out := map[string]float64{}
	for _, r := range resp.Rankings { out[r.ID] = r.Score }
	return out
}
