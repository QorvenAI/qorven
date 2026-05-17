// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

type Result struct {
	Title string `json:"title"`; URL string `json:"url"`; Snippet string `json:"snippet"`; Source string `json:"source"`
}

type Config struct {
	PerplexityKey, BraveKey, TavilyKey, SerperKey, SearXNGURL, WhoogleURL, QorCrawlKey string
}

type Pipeline struct {
	cfg Config; provider providers.Provider; client *http.Client
}

func NewPipeline(cfg Config, provider providers.Provider) *Pipeline {
	return &Pipeline{cfg: cfg, provider: provider, client: &http.Client{Timeout: 10 * time.Second}}
}

func (p *Pipeline) Search(ctx context.Context, query string, max int) ([]Result, string, error) {
	// Normalize query — fix typos before search
	query = normalizeSearchQuery(query)
	rewritten := p.RewriteQuery(ctx, query)
	slog.Info("search.rewrite", "original", query, "rewritten", rewritten)
	var mu sync.Mutex; all := []Result{}; var wg sync.WaitGroup
	for _, s := range p.sources() {
		wg.Add(1)
		go func(name string, fn func(context.Context, string, int) []Result) {
			defer wg.Done()
			r := fn(ctx, rewritten, max)
			mu.Lock(); all = append(all, r...); mu.Unlock()
		}(s.name, s.fn)
	}
	wg.Wait()
	seen := map[string]bool{}; deduped := []Result{}
	for _, r := range all {
		k := strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(r.URL, "https://"), "http://"), "/")
		if !seen[k] { seen[k] = true; deduped = append(deduped, r) }
	}
	if len(deduped) > max { deduped = deduped[:max] }
	return deduped, rewritten, nil
}

func (p *Pipeline) RewriteQuery(ctx context.Context, query string) string {
	if p.provider == nil { return query }
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second); defer cancel()
	resp, err := p.provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "Rewrite into optimal search query. Return ONLY the query:\n" + query}},
		Options: map[string]any{"temperature": 0, "max_tokens": 50},
	})
	if err != nil || strings.TrimSpace(resp.Content) == "" { return query }
	return strings.TrimSpace(resp.Content)
}

type src struct { name string; fn func(context.Context, string, int) []Result }

func (p *Pipeline) sources() []src {
	s := []src{}
	if p.cfg.PerplexityKey != "" { s = append(s, src{"perplexity", p.perplexity}) }
	if p.cfg.TavilyKey != "" { s = append(s, src{"tavily", p.tavily}) }
	if p.cfg.SerperKey != "" { s = append(s, src{"serper", p.serper}) }
	if p.cfg.BraveKey != "" { s = append(s, src{"brave", p.brave}) }
	if p.cfg.SearXNGURL != "" { s = append(s, src{"searxng", p.searxng}) }
	s = append(s, src{"duckduckgo", p.ddg})
	return s
}

func (p *Pipeline) perplexity(ctx context.Context, q string, max int) []Result {
	body, _ := json.Marshal(map[string]any{"model": "sonar", "messages": []map[string]string{{"role": "user", "content": q}}})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.perplexity.ai/chat/completions", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+p.cfg.PerplexityKey); req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req); if err != nil { return nil }; defer resp.Body.Close()
	var d struct { Citations []string }; json.NewDecoder(resp.Body).Decode(&d)
	var r []Result; for i, c := range d.Citations { if i >= max { break }; r = append(r, Result{Title: domainOf(c), URL: c, Source: "perplexity"}) }
	return r
}

func (p *Pipeline) tavily(ctx context.Context, q string, max int) []Result {
	body, _ := json.Marshal(map[string]any{"query": q, "max_results": max, "search_depth": "advanced"})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json"); req.Header.Set("Authorization", "Bearer "+p.cfg.TavilyKey)
	resp, err := p.client.Do(req); if err != nil { return nil }; defer resp.Body.Close()
	var d struct { Results []struct{ Title, URL, Content string } }; json.NewDecoder(resp.Body).Decode(&d)
	var r []Result; for _, x := range d.Results { r = append(r, Result{x.Title, x.URL, x.Content, "tavily"}) }
	return r
}

func (p *Pipeline) serper(ctx context.Context, q string, max int) []Result {
	body, _ := json.Marshal(map[string]any{"q": q, "num": max})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://google.serper.dev/search", strings.NewReader(string(body)))
	req.Header.Set("X-API-KEY", p.cfg.SerperKey); req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req); if err != nil { return nil }; defer resp.Body.Close()
	var d struct { Organic []struct{ Title, Link, Snippet string } }; json.NewDecoder(resp.Body).Decode(&d)
	var r []Result; for _, x := range d.Organic { r = append(r, Result{x.Title, x.Link, x.Snippet, "serper"}) }
	return r
}

func (p *Pipeline) brave(ctx context.Context, q string, max int) []Result {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d", url.QueryEscape(q), max), nil)
	req.Header.Set("Accept", "application/json"); req.Header.Set("X-Subscription-Token", p.cfg.BraveKey)
	resp, err := p.client.Do(req); if err != nil { return nil }; defer resp.Body.Close()
	var d struct { Web struct{ Results []struct{ Title, URL, Description string } } }; json.NewDecoder(resp.Body).Decode(&d)
	var r []Result; for _, x := range d.Web.Results { r = append(r, Result{x.Title, x.URL, x.Description, "brave"}) }
	return r
}

func (p *Pipeline) searxng(ctx context.Context, q string, max int) []Result {
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/search?q=%s&format=json&categories=general", strings.TrimRight(p.cfg.SearXNGURL, "/"), url.QueryEscape(q)), nil)
	resp, err := p.client.Do(req); if err != nil { return nil }; defer resp.Body.Close()
	var d struct { Results []struct{ Title, URL, Content string } }; json.NewDecoder(resp.Body).Decode(&d)
	var r []Result; for i, x := range d.Results { if i >= max { break }; r = append(r, Result{x.Title, x.URL, x.Content, "searxng"}) }
	return r
}

func (p *Pipeline) ddg(ctx context.Context, q string, max int) []Result {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://html.duckduckgo.com/html/?q="+url.QueryEscape(q), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := p.client.Do(req); if err != nil { return nil }; defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body); return parseDDG(string(body), max)
}

func parseDDG(html string, max int) []Result {
	r := []Result{}
	for _, part := range strings.Split(html, "result__a")[1:] {
		if len(r) >= max { break }
		hi := strings.Index(part, "href=\""); if hi < 0 { continue }
		he := strings.Index(part[hi+6:], "\""); if he < 0 { continue }
		u := part[hi+6 : hi+6+he]
		if strings.Contains(u, "uddg=") { if d, e := url.QueryUnescape(u[strings.Index(u, "uddg=")+5:]); e == nil { u = d } }
		if strings.HasPrefix(u, "http") { r = append(r, Result{URL: u, Source: "duckduckgo"}) }
	}
	return r
}

func domainOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	u = strings.TrimPrefix(u, "www.")
	if i := strings.Index(u, "/"); i > 0 { return u[:i] }
	return u
}

// FetchAndExtract fetches URLs and extracts clean text content.
// Tries multiple methods: direct fetch → readability extraction.
func (p *Pipeline) FetchAndExtract(ctx context.Context, results []Result, maxFetch int) []Result {
	if maxFetch > len(results) { maxFetch = len(results) }
	var wg sync.WaitGroup
	for i := 0; i < maxFetch; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			content := p.fetchURL(ctx, results[idx].URL)
			if content != "" {
				results[idx].Snippet = content
			}
		}(i)
	}
	wg.Wait()
	return results[:maxFetch]
}

func (p *Pipeline) fetchURL(ctx context.Context, rawURL string) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := p.client.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()

	// On bot-block, fall back to Jina Reader (free, no API key needed)
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		return p.jinaFetch(ctx, rawURL)
	}
	if resp.StatusCode != 200 { return "" }

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500_000)) // 500KB max
	return ExtractToMarkdown(ctx, rawURL, string(body), p.cfg.QorCrawlKey, p.client)
}

// jinaFetch uses r.jina.ai as a free reader proxy for bot-blocked pages.
func (p *Pipeline) jinaFetch(ctx context.Context, rawURL string) string {
	jinaURL := "https://r.jina.ai/" + rawURL
	req, _ := http.NewRequestWithContext(ctx, "GET", jinaURL, nil)
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("X-Return-Format", "text")
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != 200 { return "" }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200_000))
	text := strings.TrimSpace(string(body))
	if len(text) > 4000 { text = text[:4000] + "\n\n..." }
	return text
}

// extractText strips HTML and returns clean readable text.

// SearchAndFetch does the full pipeline: rewrite → search → fetch top 3.
func (p *Pipeline) SearchAndFetch(ctx context.Context, query string, max int) ([]Result, string, error) {
	results, rewritten, err := p.Search(ctx, query, max)
	if err != nil { return nil, rewritten, err }
	// Fetch content from top 3 results
	fetched := p.FetchAndExtract(ctx, results, min(3, len(results)))
	return fetched, rewritten, nil
}

func min(a, b int) int { if a < b { return a }; return b }

// normalizeSearchQuery fixes common typos and removes filler words for better search results.
func normalizeSearchQuery(q string) string {
	if q == "" { return q }
	// Fix doubled spaces
	for strings.Contains(q, "  ") { q = strings.ReplaceAll(q, "  ", " ") }
	// Common typos
	fixes := map[string]string{
		"teh ": "the ", "taht ": "that ", "adn ": "and ", "hte ": "the ",
		"wiht ": "with ", "waht ": "what ", "hwo ": "how ", "fo ": "of ",
		"doesnt ": "doesn't ", "dont ": "don't ", "cant ": "can't ",
		"whats ": "what's ", "hows ": "how's ", "thats ": "that's ",
	}
	for typo, fix := range fixes { q = strings.ReplaceAll(q, typo, fix) }
	// Remove filler prefixes
	lower := strings.ToLower(q)
	for _, f := range []string{"please ", "can you ", "could you ", "help me ", "show me ", "tell me ", "just "} {
		if strings.HasPrefix(lower, f) { q = q[len(f):]; lower = strings.ToLower(q) }
	}
	return strings.TrimSpace(q)
}
