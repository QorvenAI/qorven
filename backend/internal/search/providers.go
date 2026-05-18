// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// providers.go — Additional search providers: Brave, Exa, Jina, Kagi.
// These complement the existing search pipeline with specialized sources.

// BraveSearch queries the Brave Search API.
func BraveSearch(ctx context.Context, apiKey, query string, count int) ([]Result, error) {
	if count <= 0 { count = 10 }
	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d", url.QueryEscape(query), count)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	json.Unmarshal(body, &result)

	results := []Result{}
	for _, r := range result.Web.Results {
		results = append(results, Result{Title: r.Title, URL: r.URL, Snippet: r.Description, Source: "brave"})
	}
	return results, nil
}

// ExaSearch queries the Exa.ai neural search API.
func ExaSearch(ctx context.Context, apiKey, query string, count int) ([]Result, error) {
	if count <= 0 { count = 10 }
	payload, _ := json.Marshal(map[string]any{
		"query":      query,
		"numResults": count,
		"type":       "neural",
		"contents":   map[string]any{"text": true},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.exa.ai/search", strings.NewReader(string(payload)))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
			Text  string `json:"text"`
			Score float64 `json:"score"`
		} `json:"results"`
	}
	json.Unmarshal(body, &result)

	results := []Result{}
	for _, r := range result.Results {
		snippet := r.Text
		if len(snippet) > 300 { snippet = snippet[:300] }
		results = append(results, Result{Title: r.Title, URL: r.URL, Snippet: snippet, Source: "exa"})
	}
	return results, nil
}

// JinaSearch queries the Jina AI reader/search API.
func JinaSearch(ctx context.Context, apiKey, query string, count int) ([]Result, error) {
	u := "https://s.jina.ai/" + url.QueryEscape(query)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Return-Format", "json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result struct {
		Data []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Content     string `json:"content"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	results := []Result{}
	for i, r := range result.Data {
		if i >= count { break }
		snippet := r.Description
		if snippet == "" && len(r.Content) > 300 { snippet = r.Content[:300] }
		results = append(results, Result{Title: r.Title, URL: r.URL, Snippet: snippet, Source: "jina"})
	}
	return results, nil
}

// KagiFastGPT queries Kagi's FastGPT for instant answers.
func KagiFastGPT(ctx context.Context, apiKey, query string) (string, error) {
	u := fmt.Sprintf("https://kagi.com/api/v0/fastgpt?query=%s", url.QueryEscape(query))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Authorization", "Bot "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result struct {
		Data struct {
			Output     string `json:"output"`
			References []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
				URL     string `json:"url"`
			} `json:"references"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)
	return result.Data.Output, nil
}

// BraveAnswers queries Brave's AI-grounded answer API.
// Returns a direct answer with citations, like Perplexity.
func BraveAnswers(ctx context.Context, apiKey, query string) (string, []Result, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		"https://api.search.brave.com/res/v1/web/search?q="+url.QueryEscape(query)+"&summary=1", nil)
	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "", nil, err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var data struct {
		Summarizer struct {
			Key     string `json:"key"`
			Summary string `json:"summary"`
		} `json:"summarizer"`
		Web struct {
			Results []struct {
				Title string `json:"title"`
				URL   string `json:"url"`
				Desc  string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	json.Unmarshal(body, &data)

	results := []Result{}
	for _, r := range data.Web.Results {
		results = append(results, Result{Title: r.Title, URL: r.URL, Snippet: r.Desc})
	}
	return data.Summarizer.Summary, results, nil
}
