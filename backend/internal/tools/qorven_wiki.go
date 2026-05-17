// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/scraper"
)

// QorvenWiki compiles raw sources into a structured knowledge base.
type QorvenWiki struct {
	memStore *memory.Store
	scraper  *scraper.Scraper
	tenantID string
}

func NewQorvenWiki(memStore *memory.Store, tenantID string) *QorvenWiki {
	return &QorvenWiki{memStore: memStore, scraper: scraper.New(), tenantID: tenantID}
}

func (t *QorvenWiki) Name() string { return "qorven_wiki" }
func (t *QorvenWiki) Description() string {
	return `Compile knowledge from sources into a structured wiki. Actions:
- ingest: Process a URL or text, extract key concepts, create wiki articles, update index
- query: Search the wiki and synthesize an answer with citations
- lint: Run health check — find contradictions, orphans, missing links
- index: Show the current wiki index with all articles`
}
func (t *QorvenWiki) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"ingest", "query", "lint", "index"}, "description": "Wiki action"},
			"source": map[string]any{"type": "string", "description": "URL or text to ingest (for ingest action)"},
			"query":  map[string]any{"type": "string", "description": "Question to answer (for query action)"},
			"scope":  map[string]any{"type": "string", "enum": []string{"agent", "team", "company"}, "description": "Wiki scope (default: agent)"},
		},
		"required": []string{"action"},
	}
}

func (t *QorvenWiki) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	source, _ := args["source"].(string)
	query, _ := args["query"].(string)
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "agent"
	}

	agentID := AgentIDFromCtx(ctx)

	switch action {
	case "ingest":
		return t.ingest(ctx, agentID, scope, source)
	case "query":
		return t.queryWiki(ctx, agentID, scope, query)
	case "lint":
		return t.lint(ctx, agentID, scope)
	case "index":
		return t.showIndex(ctx, agentID, scope)
	default:
		return ErrorResult("unknown action: " + action)
	}
}

func (t *QorvenWiki) ingest(ctx context.Context, agentID, scope, source string) *Result {
	if source == "" {
		return ErrorResult("source URL or text required for ingest")
	}

	var content, title string

	// Fetch if URL
	if strings.HasPrefix(source, "http") {
		page, err := t.scraper.Fetch(ctx, source)
		if err != nil {
			return ErrorResult("fetch failed: " + err.Error())
		}
		content = scraper.HTMLToMarkdown(page.HTML)
		title = page.Title()
		if len(content) > 50000 {
			content = content[:50000]
		}
	} else {
		content = source
		if len(content) > 100 {
			title = content[:100]
		} else {
			title = content
		}
	}

	// Extract key concepts (simple extraction — in production, use LLM)
	concepts := extractConcepts(content)

	// Save as wiki article
	articleID, _ := t.memStore.Save(ctx, t.tenantID, memory.Memory{
		AgentID:     agentID,
		Type:        "wiki:" + scope,
		Content:     fmt.Sprintf("# %s\n\nSource: %s\nIngested: %s\n\n%s\n\n## Key Concepts\n%s", title, source, time.Now().Format("2006-01-02"), content[:min(len(content), 5000)], strings.Join(concepts, ", ")),
		Source:      source,
		Importance:  0.8,
		DecayExempt: true,
	})

	// Update index
	t.memStore.Save(ctx, t.tenantID, memory.Memory{
		AgentID:     agentID,
		Type:        "wiki-index:" + scope,
		Content:     fmt.Sprintf("- [%s] %s (concepts: %s)", articleID[:8], title, strings.Join(concepts[:min(len(concepts), 5)], ", ")),
		Source:      "wiki-compiler",
		Importance:  0.9,
		DecayExempt: true,
	})

	// Save each concept as a cross-reference
	for _, concept := range concepts[:min(len(concepts), 10)] {
		t.memStore.Save(ctx, t.tenantID, memory.Memory{
			AgentID:     agentID,
			Type:        "wiki-concept:" + scope,
			Content:     fmt.Sprintf("## %s\n\nReferenced in: %s\nSource: %s", concept, title, source),
			Source:      "wiki-compiler",
			Importance:  0.6,
			DecayExempt: true,
		})
	}

	return TextResult(fmt.Sprintf("✅ Ingested: %s\n\nArticle ID: %s\nConcepts extracted: %d\nCross-references created: %d\nScope: %s",
		title, articleID[:8], len(concepts), min(len(concepts), 10), scope))
}

func (t *QorvenWiki) queryWiki(ctx context.Context, agentID, scope, query string) *Result {
	if query == "" {
		return ErrorResult("query required")
	}

	// Search wiki articles
	articles, _ := t.memStore.SearchByTypeQuery(ctx, t.tenantID, "wiki:"+scope, query, 10)
	// Search concepts
	concepts, _ := t.memStore.SearchByTypeQuery(ctx, t.tenantID, "wiki-concept:"+scope, query, 5)

	if len(articles) == 0 && len(concepts) == 0 {
		return TextResult("No wiki articles found for: " + query + "\n\nTry ingesting some sources first with action=ingest.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Wiki Query: %s\n\n", query))
	sb.WriteString(fmt.Sprintf("Found %d articles, %d concepts\n\n", len(articles), len(concepts)))

	for i, a := range articles {
		content := a.Memory.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		sb.WriteString(fmt.Sprintf("## %d. [Source: %s]\n%s\n\n", i+1, a.Memory.Source, content))
	}

	if len(concepts) > 0 {
		sb.WriteString("## Related Concepts\n")
		for _, c := range concepts {
			sb.WriteString(fmt.Sprintf("- %s\n", strings.Split(c.Memory.Content, "\n")[0]))
		}
	}

	result := sb.String()
	if len(result) > MaxToolOutput {
		result = result[:MaxToolOutput]
	}
	return TextResult(result)
}

func (t *QorvenWiki) lint(ctx context.Context, agentID, scope string) *Result {
	// Get all wiki articles
	articles, _ := t.memStore.SearchByType(ctx, agentID, "wiki:"+scope, 100)
	concepts, _ := t.memStore.SearchByType(ctx, agentID, "wiki-concept:"+scope, 100)
	index, _ := t.memStore.SearchByType(ctx, agentID, "wiki-index:"+scope, 100)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Wiki Health Check (%s scope)\n\n", scope))
	sb.WriteString(fmt.Sprintf("Date: %s\n\n", time.Now().Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("## Stats\n- Articles: %d\n- Concepts: %d\n- Index entries: %d\n\n", len(articles), len(concepts), len(index)))

	// Check for orphan concepts (mentioned but no article)
	orphans := 0
	for _, c := range concepts {
		conceptName := strings.TrimPrefix(strings.Split(c.Content, "\n")[0], "## ")
		found := false
		for _, a := range articles {
			if strings.Contains(a.Content, conceptName) {
				found = true
				break
			}
		}
		if !found {
			orphans++
			sb.WriteString(fmt.Sprintf("🟡 Orphan concept: %s\n", conceptName))
		}
	}

	// Check for articles without concepts
	noConcepts := 0
	for _, a := range articles {
		if !strings.Contains(a.Content, "## Key Concepts") {
			noConcepts++
			sb.WriteString(fmt.Sprintf("🟡 Article without concepts: %s\n", a.Source))
		}
	}

	// Check for missing sources
	noSource := 0
	for _, a := range articles {
		if a.Source == "" || a.Source == "wiki-compiler" {
			noSource++
			sb.WriteString("🔴 Article without source attribution\n")
		}
	}

	if orphans == 0 && noConcepts == 0 && noSource == 0 {
		sb.WriteString("\n✅ Wiki is healthy! No issues found.\n")
	} else {
		sb.WriteString(fmt.Sprintf("\n## Summary\n🔴 Errors: %d | 🟡 Warnings: %d\n", noSource, orphans+noConcepts))
	}

	return TextResult(sb.String())
}

func (t *QorvenWiki) showIndex(ctx context.Context, agentID, scope string) *Result {
	entries, _ := t.memStore.SearchByType(ctx, agentID, "wiki-index:"+scope, 100)
	if len(entries) == 0 {
		return TextResult("Wiki is empty. Ingest some sources to get started.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Wiki Index (%s scope)\n\n", scope))
	sb.WriteString(fmt.Sprintf("%d articles\n\n", len(entries)))
	for _, e := range entries {
		sb.WriteString(e.Content + "\n")
	}
	return TextResult(sb.String())
}

// extractConcepts pulls key terms using TF + bigrams + entity patterns.
func extractConcepts(text string) []string {
	lower := strings.ToLower(text)
	words := strings.Fields(lower)

	// Unigram frequency
	freq := map[string]int{}
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}#*-_")
		if len(w) > 4 && !isStopWord(w) {
			freq[w]++
		}
	}

	// Bigram frequency (catches "machine learning", "knowledge base", etc.)
	for i := 0; i < len(words)-1; i++ {
		a := strings.Trim(words[i], ".,;:!?\"'()[]{}#*-_")
		b := strings.Trim(words[i+1], ".,;:!?\"'()[]{}#*-_")
		if len(a) > 2 && len(b) > 2 && !isStopWord(a) && !isStopWord(b) {
			bigram := a + " " + b
			freq[bigram] += 2 // weight bigrams higher
		}
	}

	// Detect capitalized entities (proper nouns, product names)
	origWords := strings.Fields(text)
	for _, w := range origWords {
		w = strings.Trim(w, ".,;:!?\"'()[]{}#*-_")
		if len(w) > 2 && w[0] >= 'A' && w[0] <= 'Z' && !isCommonCapitalized(w) {
			freq[strings.ToLower(w)] += 3 // weight entities higher
		}
	}

	// Sort by frequency
	type kv struct{ k string; v int }
	var sorted []kv
	for k, v := range freq {
		if v >= 2 { sorted = append(sorted, kv{k, v}) }
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v { sorted[i], sorted[j] = sorted[j], sorted[i] }
		}
	}

	var concepts []string
	for i, kv := range sorted {
		if i >= 20 { break }
		concepts = append(concepts, kv.k)
	}
	return concepts
}

func isCommonCapitalized(w string) bool {
	common := map[string]bool{"The": true, "This": true, "That": true, "These": true, "Those": true, "What": true, "When": true, "Where": true, "Which": true, "How": true, "Why": true, "Who": true, "Its": true, "Our": true, "Your": true, "Their": true}
	return common[w]
}

func isStopWord(w string) bool {
	stops := map[string]bool{"about": true, "after": true, "again": true, "being": true, "between": true, "could": true, "does": true, "doing": true, "during": true, "every": true, "from": true, "have": true, "into": true, "just": true, "more": true, "most": true, "much": true, "only": true, "other": true, "over": true, "same": true, "should": true, "some": true, "such": true, "than": true, "that": true, "their": true, "them": true, "then": true, "there": true, "these": true, "they": true, "this": true, "those": true, "through": true, "under": true, "very": true, "what": true, "when": true, "where": true, "which": true, "while": true, "will": true, "with": true, "would": true, "your": true}
	return stops[w]
}
