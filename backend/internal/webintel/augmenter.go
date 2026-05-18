// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package webintel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Augmenter automatically enriches agent prompts with web search results.
// This is the Perplexity-grade feature: every query gets web context.
type Augmenter struct {
	intel *WebIntel
}

func NewAugmenter(intel *WebIntel) *Augmenter {
	return &Augmenter{intel: intel}
}

// AugmentResult contains the web context and sources for the LLM.
type AugmentResult struct {
	Context string         // formatted context to inject into prompt
	Sources []SearchResult // raw sources for citation
	Elapsed time.Duration
}

// Augment searches the web for the query and returns formatted context.
// mode: "speed" (1 query, no scrape), "balanced" (3 queries, scrape top 3), "quality" (3 queries, scrape top 5)
// ShouldSkipWebSearch uses a fast heuristic to decide if web search is needed.
// This replaces the hardcoded skip list with a smarter approach:
// 1. Very short messages → skip (conversational)
// 2. Messages about the AI itself → skip (meta/identity)
// 3. Messages that are clearly commands/actions → skip
// 4. Everything else → search (when in doubt, search)
func ShouldSkipWebSearch(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))

	// Too short — likely conversational ("ok", "thanks", "yes")
	if len(q) < 12 { return true }

	// Single word
	words := strings.Fields(q)
	if len(words) <= 1 { return true }

	// Meta/identity — about the AI itself (contains "you" + identity context)
	youWords := []string{"you", "your", "yourself"}
	identityWords := []string{"model", "name", "who", "what are", "version", "made", "built", "created", "powered", "ai", "gpt", "claude", "gemini", "llm"}
	hasYou := false
	for _, w := range youWords { if strings.Contains(q, w) { hasYou = true; break } }
	if hasYou {
		for _, w := range identityWords { if strings.Contains(q, w) { return true } }
	}

	// Greetings and social
	greetings := []string{"hello", "hi ", "hey ", "good morning", "good evening", "good night", "bye", "goodbye", "thank", "thanks", "great", "perfect", "awesome", "nice", "cool", "ok ", "okay", "sure", "yes", "no ", "alright", "got it", "understood"}
	for _, g := range greetings {
		if strings.HasPrefix(q, g) || q == strings.TrimSpace(g) { return true }
	}

	// Action requests that don't need search
	actions := []string{"write me", "create a", "generate a", "make a", "build a", "code a", "draft a", "compose", "translate", "summarize this", "explain this", "fix this", "debug this", "refactor", "review this"}
	for _, a := range actions {
		if strings.HasPrefix(q, a) { return true }
	}

	// If it contains a question word + topic → likely needs search
	// Default: search (when in doubt, search is better than not searching)
	return false
}

func (a *Augmenter) Augment(ctx context.Context, query, mode string) *AugmentResult {
	start := time.Now()

	queries := RewriteQueries(query)
	var results []SearchResult

	switch mode {
	case "speed":
		// Single query, snippets only
		results = a.intel.Search(ctx, queries[:1], 5)
	case "quality":
		// All queries, scrape top 5
		results = a.intel.SearchAndScrape(ctx, queries, 5)
	default: // balanced
		// All queries, scrape top 3
		results = a.intel.SearchAndScrape(ctx, queries, 3)
	}

	if len(results) == 0 {
		return &AugmentResult{Elapsed: time.Since(start)}
	}

	context := FormatForLLM(results)
	slog.Info("webintel.augmented", "query", query[:min(len(query), 50)], "sources", len(results), "mode", mode, "elapsed_ms", time.Since(start).Milliseconds())

	return &AugmentResult{
		Context: context,
		Sources: results,
		Elapsed: time.Since(start),
	}
}

// InjectIntoPrompt adds web context to the system prompt with citation instructions.
func InjectIntoPrompt(systemPrompt, webContext string, sourceCount int) string {
	if webContext == "" {
		return systemPrompt
	}
	var sb strings.Builder
	sb.WriteString(systemPrompt)
	sb.WriteString(`

## Web Search Results
You have access to real-time web search results below. You MUST use them to provide accurate, detailed, up-to-date answers.

### Response Requirements
- **Comprehensive**: Give thorough, detailed answers. Never give one-liners. Explain the topic in depth.
- **Well-structured**: Use headings, bullet points, and clear formatting for readability.
- **Cited**: Cite EVERY fact using [number] notation matching the sources below. Every sentence should have at least one citation.
- **Current**: Prioritize the most recent information from the sources.
- **Engaging**: Write like a knowledgeable expert explaining to a colleague — detailed but clear.

### For Weather Queries
Provide: current temperature, conditions, humidity, wind, feels-like temperature, today's high/low, precipitation chance, and a brief multi-day forecast. Include any warnings or notable conditions.

### For Research/Analysis Queries  
Provide: comprehensive overview, key findings, multiple perspectives, recent developments, and actionable insights. Minimum 200 words for complex topics.

### Citation Format
- Use [1], [2], etc. inline after each fact
- Example: "Dubai is experiencing thunderstorms with temperatures around 26°C [1], with a high chance of flooding [3]."

`)
	sb.WriteString(webContext)
	sb.WriteString(fmt.Sprintf("\n\nYou have %d sources. Cite them inline. Give a DETAILED answer, not a one-liner.\nIMPORTANT: Your response MUST be at least 3-4 paragraphs. Include current conditions, forecast, and any notable weather events. Never respond with just one sentence.\n", sourceCount))
	return sb.String()
}

func min(a, b int) int { if a < b { return a }; return b }
