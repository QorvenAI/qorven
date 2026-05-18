// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"github.com/qorvenai/qorven/internal/search"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// --- web_search ---

type WebSearchTool struct {
	pipeline *search.Pipeline
}

func NewWebSearchTool(pipeline *search.Pipeline) *WebSearchTool {
	return &WebSearchTool{pipeline: pipeline}
}

func (t *WebSearchTool) Name() string        { return "web_search" }
func (t *WebSearchTool) Description() string { return "Search the web for current information, news, facts, or anything you need to verify. Uses Perplexity Sonar for high-quality results with citations." }
func (t *WebSearchTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query":       map[string]any{"type": "string", "description": "Search query"},
		"max_results": map[string]any{"type": "integer", "description": "Max results (default 5, max 20)"},
	}, "required": []string{"query"}}
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	query, _ := args["query"].(string)
	if query == "" { return ErrorResult("query is required") }
	maxResults := 5
	if n, ok := toInt(args["max_results"]); ok && n > 0 { maxResults = n }
	if maxResults > 20 { maxResults = 20 }

	if t.pipeline != nil {
		// SearchAndFetch = search + fetch+extract top-3 pages so the agent
		// receives actual page content (live scores, prices, news body, etc.)
		// not just a list of bare URLs.
		results, rewritten, err := t.pipeline.SearchAndFetch(ctx, query, maxResults)
		if err != nil { return ErrorResult("search failed: " + err.Error()) }
		var sb strings.Builder
		if rewritten != query { fmt.Fprintf(&sb, "Search query: %s\n\n", rewritten) }
		anyContent := false
		for i, r := range results {
			fmt.Fprintf(&sb, "[%d] %s\n%s\n", i+1, r.Title, r.URL)
			if r.Snippet != "" {
				snip := r.Snippet
				if len(snip) > 2000 { snip = snip[:2000] + "…" }
				fmt.Fprintf(&sb, "%s\n", snip)
				anyContent = true
			}
			sb.WriteString("\n")
		}
		if anyContent {
			sb.WriteString("\n[Content extracted from sources above. Use web_fetch on a specific URL if you need more detail from one source.]")
		} else {
			// All fetches failed (403/JS-rendered/paywalled) — tell agent to use web_fetch explicitly
			sb.WriteString("\n[Page content could not be auto-extracted (sites may block bots). Use web_fetch on the most relevant URL above to get the actual content.]")
		}
		return &Result{ForLLM: sb.String()}
	}
	return t.searchDDG(ctx, query, maxResults)
}

func (t *WebSearchTool) searchDDG(ctx context.Context, query string, max int) *Result {
	u := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "Qorven/1.0")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil { return ErrorResult(fmt.Sprintf("search failed: %v", err)) }
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500000))
	html := string(body)

	// Parse DDG HTML results
	re := regexp.MustCompile(`<a rel="nofollow" class="result__a" href="([^"]+)"[^>]*>([^<]+)</a>`)
	matches := re.FindAllStringSubmatch(html, max)

	snippetRe := regexp.MustCompile(`<a class="result__snippet"[^>]*>([^<]+)</a>`)
	snippets := snippetRe.FindAllStringSubmatch(html, max)

	var b strings.Builder
	for i, m := range matches {
		title := strings.TrimSpace(m[2])
		link := m[1]
		snippet := ""
		if i < len(snippets) { snippet = strings.TrimSpace(snippets[i][1]) }
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n\n", i+1, title, link, snippet)
	}
	if b.Len() == 0 { return TextResult("no results found for: " + query) }
	return TextResult(b.String())
}

// --- clarify ---

type ClarifyTool struct{}

func NewClarifyTool() *ClarifyTool { return &ClarifyTool{} }
func (t *ClarifyTool) Name() string { return "clarify" }
func (t *ClarifyTool) Description() string {
	return "Ask the user a clarifying question before proceeding. Use when the request is ambiguous or you need more information to give the best answer."
}
func (t *ClarifyTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"question": map[string]any{"type": "string", "description": "The clarifying question to ask the user"},
	}, "required": []string{"question"}}
}
func (t *ClarifyTool) Execute(ctx context.Context, args map[string]any) *Result {
	question, _ := args["question"].(string)
	if question == "" { return ErrorResult("question is required") }
	return &Result{ForLLM: "CLARIFICATION_REQUESTED: " + question, ForUser: "❓ " + question}
}


// searchSearXNG queries a self-hosted SearXNG instance.
