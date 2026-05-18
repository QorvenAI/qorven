// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/llm"
)

type Mode string

const (
	ModeSpeed    Mode = "speed"    // 1 search, fast answer
	ModeBalanced Mode = "balanced" // 3 searches, synthesized
	ModeQuality  Mode = "quality"  // 5+ searches + scrape, deep
)

type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

type Report struct {
	Query   string   `json:"query"`
	Mode    Mode     `json:"mode"`
	Answer  string   `json:"answer"`
	Sources []Source `json:"sources"`
}

type Engine struct {
	searxngURL string
	llm        llm.Provider
	client     *http.Client
}

func NewEngine(searxngURL string, llmProvider llm.Provider) *Engine {
	return &Engine{searxngURL: searxngURL, llm: llmProvider, client: &http.Client{Timeout: 30 * time.Second}}
}

func (e *Engine) Research(ctx context.Context, query string, mode Mode) (*Report, error) {
	return e.ResearchWithProgress(ctx, query, mode, nil)
}

// ProgressEvent is sent during research for streaming UI.
type ProgressEvent struct {
	Step    string `json:"step"`    // "decompose", "search", "analyze", "synthesize"
	Detail  string `json:"detail"`
	Sources int    `json:"sources,omitempty"`
}

func (e *Engine) ResearchWithProgress(ctx context.Context, query string, mode Mode, onProgress func(ProgressEvent)) (*Report, error) {
	numSearches := 1
	switch mode {
	case ModeBalanced:
		numSearches = 3
	case ModeQuality:
		numSearches = 5
	}

	progress := func(ev ProgressEvent) { if onProgress != nil { onProgress(ev) } }

	// Step 1: Decompose query
	progress(ProgressEvent{Step: "decompose", Detail: fmt.Sprintf("Breaking down query into %d search angles", numSearches)})
	slog.Info("research started", "query", query, "mode", mode, "searches", numSearches)
	queries := e.expandQueries(query, numSearches)

	// Step 2: Search in PARALLEL
	type searchResult struct {
		sources []Source
		query   string
	}
	resultCh := make(chan searchResult, len(queries))
	for _, q := range queries {
		go func(query string) {
			sources, err := e.search(ctx, query)
			if err != nil {
				slog.Warn("search failed", "query", query, "error", err)
				resultCh <- searchResult{query: query}
				return
			}
			resultCh <- searchResult{sources: sources, query: query}
		}(q)
	}

	allSources := []Source{}
	for i := 0; i < len(queries); i++ {
		r := <-resultCh
		progress(ProgressEvent{Step: "search", Detail: fmt.Sprintf("Searched [%d/%d]: %s (%d results)", i+1, len(queries), r.query, len(r.sources))})
		allSources = append(allSources, r.sources...)
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := []Source{}
	for _, s := range allSources {
		if !seen[s.URL] { seen[s.URL] = true; unique = append(unique, s) }
	}
	if len(unique) > 10 { unique = unique[:10] }

	progress(ProgressEvent{Step: "analyze", Detail: fmt.Sprintf("Analyzing %d sources", len(unique)), Sources: len(unique)})

	// Step 3: Extract content from top sources (the Perplexity effect)
	// Search only returns titles/descriptions. We need to READ the pages.
	if mode != ModeSpeed && e.llm != nil {
		progress(ProgressEvent{Step: "extract", Detail: fmt.Sprintf("Reading %d pages", min(len(unique), 5))})
		unique = e.extractContent(ctx, unique, min(len(unique), 5))
	}

	// Step 4: Synthesize
	progress(ProgressEvent{Step: "synthesize", Detail: "Generating cited answer"})
	answer, err := e.synthesize(ctx, query, unique)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}

	return &Report{Query: query, Mode: mode, Answer: answer, Sources: unique}, nil
}

func (e *Engine) search(ctx context.Context, query string) ([]Source, error) {
	if e.searxngURL != "" {
		return e.searchSearxNG(ctx, query)
	}
	return e.searchDDG(ctx, query)
}

func (e *Engine) searchSearxNG(ctx context.Context, query string) ([]Source, error) {
	u := fmt.Sprintf("%s/search?q=%s&format=json&engines=google,bing,duckduckgo", e.searxngURL, query)
	resp, err := e.client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	sources := []Source{}
	for _, r := range result.Results {
		if len(sources) >= 5 {
			break
		}
		sources = append(sources, Source{Title: r.Title, URL: r.URL, Text: r.Content})
	}
	return sources, nil
}

func (e *Engine) searchDDG(ctx context.Context, query string) ([]Source, error) {
	u := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", query)
	resp, err := e.client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50000))
	// Basic extraction from DDG HTML
	text := string(body)
	sources := []Source{}
	for _, marker := range []string{"result__a", "result__snippet"} {
		idx := strings.Index(text, marker)
		if idx > 0 && len(sources) < 3 {
			sources = append(sources, Source{Title: "DuckDuckGo Result", URL: "https://duckduckgo.com/?q=" + query, Text: text[idx:min(idx+200, len(text))]})
		}
	}
	return sources, nil
}

func (e *Engine) synthesize(ctx context.Context, query string, sources []Source) (string, error) {
	context_parts := []string{}
	for i, s := range sources {
		context_parts = append(context_parts, fmt.Sprintf("[%d] %s\n%s\nURL: %s", i+1, s.Title, s.Text, s.URL))
	}
	sourceContext := strings.Join(context_parts, "\n\n")

	prompt := fmt.Sprintf(`You are a research assistant. Answer the following question using ONLY the provided sources. Cite sources using [1], [2], etc. inline.

Question: %s

Sources:
%s

Provide a comprehensive, well-cited answer. If sources are insufficient, say so.`, query, sourceContext)

	// Nil-guard: research was previously constructed with a nil llm
	// provider in gateway.go, and any request that reached synthesis
	// panicked here. Degrade to a plain source listing so the user
	// still gets their research even without an LLM attached.
	if e.llm == nil {
		var sb strings.Builder
		fmt.Fprintf(&sb, "# Research: %s\n\n(LLM synthesis unavailable — showing raw sources.)\n\n", query)
		for i, s := range sources {
			fmt.Fprintf(&sb, "## %d. %s\n%s\n<%s>\n\n", i+1, s.Title, s.Text, s.URL)
		}
		return sb.String(), nil
	}

	resp, err := e.llm.Chat(ctx, llm.ChatRequest{
		Model:    "balanced",
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (e *Engine) expandQueries(query string, n int) []string {
	if n <= 1 {
		return []string{query}
	}

	// Use LLM to decompose query into sub-queries
	if e.llm != nil {
		prompt := fmt.Sprintf(`Break this research question into %d specific sub-queries that together would fully answer it.
Each sub-query should search for a different angle or aspect.

Question: %s

Return ONLY a JSON array of strings, nothing else:
["sub-query 1", "sub-query 2", ...]`, n, query)

		resp, err := e.llm.Chat(context.Background(), llm.ChatRequest{
			Model:    "balanced",
			Messages: []llm.Message{{Role: "user", Content: prompt}},
		})
		if err == nil {
			content := strings.TrimSpace(resp.Content)
			content = strings.TrimPrefix(content, "```json")
			content = strings.TrimPrefix(content, "```")
			content = strings.TrimSuffix(content, "```")
			content = strings.TrimSpace(content)

			subQueries := []string{}
			if json.Unmarshal([]byte(content), &subQueries) == nil && len(subQueries) > 0 {
				slog.Info("research.decomposed", "original", query, "sub_queries", len(subQueries))
				return subQueries
			}
		}
	}

	// Fallback: simple expansion
	queries := []string{query}
	if n >= 2 { queries = append(queries, query+" explained") }
	if n >= 3 { queries = append(queries, query+" latest 2026") }
	if n >= 4 { queries = append(queries, query+" comparison") }
	if n >= 5 { queries = append(queries, query+" pros cons") }
	return queries[:min(len(queries), n)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractContent fetches and LLM-summarizes the top N source pages.
// This is the key difference between "search results" and "research" —
// we actually READ the pages, not just return titles.
func (e *Engine) extractContent(ctx context.Context, sources []Source, maxExtract int) []Source {
	client := &http.Client{Timeout: 15 * time.Second}

	for i := 0; i < maxExtract && i < len(sources); i++ {
		s := &sources[i]
		if s.Text != "" && len(s.Text) > 200 { continue } // already has content

		// Fetch page
		req, err := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
		if err != nil { continue }
		req.Header.Set("User-Agent", "Qorven/1.0 Research Agent")

		resp, err := client.Do(req)
		if err != nil { slog.Debug("extract.fetch_failed", "url", s.URL, "error", err); continue }

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 100000))
		resp.Body.Close()

		if resp.StatusCode >= 400 { continue }

		content := string(body)
		if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			content = simpleHTMLToText(content)
		}

		if len(content) < 100 { continue }

		// LLM summarize if content is large
		if len(content) > 5000 && e.llm != nil {
			summary, err := e.llm.Chat(ctx, llm.ChatRequest{
				Model: "balanced",
				Messages: []llm.Message{
					{Role: "system", Content: "Extract the key facts, data points, and findings from this web page. Output clean markdown. Be thorough but concise. Do NOT add commentary."},
					{Role: "user", Content: content[:min(len(content), 15000)]},
				},
				MaxTokens: 1000,
			})
			
			
			if err == nil { content = summary.Content }
		} else if len(content) > 3000 {
			content = content[:3000] + "..."
		}

		s.Text = content
		slog.Debug("extract.done", "url", s.URL, "chars", len(content))
	}
	return sources
}

// simpleHTMLToText strips HTML tags for research extraction.
func simpleHTMLToText(html string) string {
	// Remove script/style blocks
	for _, tag := range []string{"script", "style", "nav", "footer", "header"} {
		for {
			start := strings.Index(strings.ToLower(html), "<"+tag)
			if start == -1 { break }
			end := strings.Index(strings.ToLower(html[start:]), "</"+tag+">")
			if end == -1 { break }
			html = html[:start] + html[start+end+len("</"+tag+">"):]
		}
	}
	// Strip remaining tags
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' { inTag = true; continue }
		if r == '>' { inTag = false; result.WriteRune(' '); continue }
		if !inTag { result.WriteRune(r) }
	}
	// Collapse whitespace
	text := result.String()
	for strings.Contains(text, "  ") { text = strings.ReplaceAll(text, "  ", " ") }
	for strings.Contains(text, "\n\n\n") { text = strings.ReplaceAll(text, "\n\n\n", "\n\n") }
	return strings.TrimSpace(text)
}
