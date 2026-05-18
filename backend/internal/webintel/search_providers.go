// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package webintel

import (
	"context"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SearchProvider is a configured search service with key rotation.
type SearchProvider struct {
	ID      string
	Name    string
	Keys    []string // multiple keys for rotation
	BaseURL string
	current atomic.Int64
}

// NextKey returns the next key in rotation.
func (p *SearchProvider) NextKey() string {
	if len(p.Keys) == 0 { return "" }
	idx := p.current.Add(1) % int64(len(p.Keys))
	return p.Keys[idx]
}

// SearchRegistry manages available search providers with key rotation.
type SearchRegistry struct {
	mu        sync.RWMutex
	providers map[string]*SearchProvider // brave-search, tavily, exa, searxng
}

func NewSearchRegistry() *SearchRegistry {
	return &SearchRegistry{providers: make(map[string]*SearchProvider)}
}

// Register adds a search provider with one or more keys.
func (r *SearchRegistry) Register(id, name, baseURL string, keys []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.providers[id]; ok {
		// Append keys to existing provider
		existing.Keys = append(existing.Keys, keys...)
		return
	}
	r.providers[id] = &SearchProvider{ID: id, Name: name, Keys: keys, BaseURL: baseURL}
	slog.Info("search.provider.registered", "id", id, "keys", len(keys))
}

// Get returns a provider by ID.
func (r *SearchRegistry) Get(id string) *SearchProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.providers[id]
}

// BestAvailable returns the best search provider in priority order.
// Priority: SearXNG > Brave > Tavily > Exa > DuckDuckGo (always available)
func (r *SearchRegistry) BestAvailable() (string, *SearchProvider) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range []string{"searxng", "perplexity-search", "brave-search", "tavily", "exa"} {
		if p, ok := r.providers[id]; ok && len(p.Keys) > 0 {
			return id, p
		}
		if p, ok := r.providers[id]; ok && p.BaseURL != "" && id == "searxng" {
			return id, p // SearXNG doesn't need keys
		}
	}
	return "ddg", nil // DuckDuckGo fallback
}

// List returns all registered providers.
func (r *SearchRegistry) List() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []map[string]any{}
	for _, p := range r.providers {
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "keys": len(p.Keys), "base_url": p.BaseURL,
		})
	}
	return out
}

// SearchWithBest uses the best available provider to search.
func (w *WebIntel) SearchWithRegistry(ctx context.Context, registry *SearchRegistry, queries []string, maxPerQuery int) []SearchResult {
	if registry == nil {
		return w.Search(ctx, queries, maxPerQuery)
	}

	bestID, provider := registry.BestAvailable()
	switch bestID {
	case "perplexity-search":
		return w.searchPerplexityParallel(ctx, provider, queries, maxPerQuery)
	case "brave-search":
		return w.searchBraveParallel(ctx, provider, queries, maxPerQuery)
	case "tavily":
		return w.searchTavilyParallel(ctx, provider, queries, maxPerQuery)
	case "searxng":
		w.searxngURL = provider.BaseURL
		return w.Search(ctx, queries, maxPerQuery)
	default:
		return w.Search(ctx, queries, maxPerQuery) // DDG fallback
	}
}

// --- Brave Search ---
func (w *WebIntel) searchBraveParallel(ctx context.Context, p *SearchProvider, queries []string, max int) []SearchResult {
	var mu sync.Mutex
	all := []SearchResult{}
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			key := p.NextKey()
			results := searchBrave(ctx, key, query, max)
			mu.Lock()
			all = append(all, results...)
			mu.Unlock()
		}(q)
	}
	wg.Wait()
	return dedup(all)
}

func searchBrave(ctx context.Context, apiKey, query string, max int) []SearchResult {
	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d", url.QueryEscape(query), max)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil { return nil }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil }
	var data struct {
		Web struct {
			Results []struct {
				Title string `json:"title"`
				URL   string `json:"url"`
				Desc  string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	results := []SearchResult{}
	for _, r := range data.Web.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Desc})
	}
	slog.Info("search.brave", "query", query, "results", len(results))
	return results
}

// --- Tavily Search ---
func (w *WebIntel) searchTavilyParallel(ctx context.Context, p *SearchProvider, queries []string, max int) []SearchResult {
	var mu sync.Mutex
	all := []SearchResult{}
	var wg sync.WaitGroup
	for _, q := range queries {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			key := p.NextKey()
			results := searchTavily(ctx, key, query, max)
			mu.Lock()
			all = append(all, results...)
			mu.Unlock()
		}(q)
	}
	wg.Wait()
	return dedup(all)
}

func searchTavily(ctx context.Context, apiKey, query string, max int) []SearchResult {
	body := fmt.Sprintf(`{"api_key":"%s","query":"%s","max_results":%d,"include_answer":false}`,
		apiKey, strings.ReplaceAll(query, `"`, `\"`), max)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil { return nil }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	json.Unmarshal(b, &data)
	results := []SearchResult{}
	for _, r := range data.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	slog.Info("search.tavily", "query", query, "results", len(results))
	return results
}

func dedup(results []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	out := []SearchResult{}
	for _, r := range results {
		if !seen[r.URL] { seen[r.URL] = true; out = append(out, r) }
	}
	return out
}

// --- Perplexity Search API ---
func (w *WebIntel) searchPerplexityParallel(ctx context.Context, p *SearchProvider, queries []string, max int) []SearchResult {
	// Perplexity supports multi-query in a single request (up to 5)
	key := p.NextKey()
	return searchPerplexity(ctx, key, queries, max)
}

func searchPerplexity(ctx context.Context, apiKey string, queries []string, max int) []SearchResult {
	// Perplexity Search API supports multi-query natively
	var queryParam any
	if len(queries) == 1 {
		queryParam = queries[0]
	} else {
		queryParam = queries
	}
	body, _ := json.Marshal(map[string]any{
		"query":               queryParam,
		"max_results":         max,
		"max_tokens_per_page": 2048,
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.perplexity.ai/search", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil { return nil }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
			Date    string `json:"date"`
		} `json:"results"`
	}
	json.Unmarshal(b, &data)
	results := []SearchResult{}
	for _, r := range data.Results {
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
	}
	slog.Info("search.perplexity", "queries", len(queries), "results", len(results))
	return results
}
