// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"os"
	"time"
)

// QorCrawlTool crawls entire websites and extracts structured markdown.
// Use for: deep site crawling, documentation extraction, multi-page scraping.
// NOT for: single URL fetch (use web_fetch), search (use web_search).
type QorCrawlTool struct {
	apiToken string
	baseURL  string
	client   *http.Client
}

func NewQorCrawlTool(apiToken string) *QorCrawlTool {
	base := "https://api.qorven.ai/crawl/v1"
	// Support self-hosted: QOR_CRAWL_URL=http://localhost:3002/v1
	if env := os.Getenv("QOR_CRAWL_URL"); env != "" {
		base = env
	}
	return &QorCrawlTool{apiToken: apiToken, baseURL: base, client: &http.Client{Timeout: 60 * time.Second}}
}



func (t *QorCrawlTool) Name() string { return "qor_crawl" }
func (t *QorCrawlTool) Description() string {
	return `Deep web crawl tool. Crawls entire websites and extracts clean markdown content.
Use this when you need to:
- Crawl an entire site or multiple pages (not just one URL)
- Extract documentation from a website
- Get structured content from complex pages
- Scrape sites that require JavaScript rendering

For a SINGLE URL, use web_fetch instead (faster, free).
For SEARCHING the web, use web_search instead.`
}

func (t *QorCrawlTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":        map[string]any{"type": "string", "description": "URL to crawl (starting point)"},
			"mode":       map[string]any{"type": "string", "description": "scrape (single page) or crawl (follow links). Default: scrape", "enum": []string{"scrape", "crawl"}},
			"max_pages":  map[string]any{"type": "integer", "description": "Max pages to crawl (crawl mode only, default 5)"},
			"selectors":  map[string]any{"type": "string", "description": "CSS selectors to extract (optional, comma-separated)"},
		},
		"required": []string{"url"},
	}
}

func (t *QorCrawlTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.apiToken == "" {
		return ErrorResult("QorCrawl not configured — set CRAWL4AI_API_TOKEN")
	}

	url, _ := args["url"].(string)
	if url == "" {
		return ErrorResult("url is required")
	}

	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "scrape"
	}

	if mode == "scrape" {
		return t.scrape(ctx, url)
	}
	maxPages := 5
	if mp, ok := args["max_pages"].(float64); ok && mp > 0 {
		maxPages = int(mp)
	}
	return t.crawl(ctx, url, maxPages)
}

func (t *QorCrawlTool) scrape(ctx context.Context, url string) *Result {
	body, _ := json.Marshal(map[string]any{
		"url":     url,
		"formats": []string{"markdown"},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", t.baseURL+"/scrape", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("QorCrawl scrape error: %v", err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("QorCrawl %d: %s", resp.StatusCode, string(data[:min(len(data), 200)])))
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string            `json:"markdown"`
			Metadata map[string]string `json:"metadata"`
		} `json:"data"`
	}
	json.Unmarshal(data, &result)

	if !result.Success || result.Data.Markdown == "" {
		return ErrorResult("QorCrawl returned no content")
	}

	content := result.Data.Markdown
	if len(content) > 15000 {
		content = content[:15000] + "\n\n... (truncated, full page was " + fmt.Sprintf("%d", len(result.Data.Markdown)) + " chars)"
	}

	title := result.Data.Metadata["title"]
	slog.Info("qor_crawl.scrape", "url", url, "chars", len(result.Data.Markdown), "title", title)
	return TextResult(fmt.Sprintf("📄 QorCrawl scraped: %s\nTitle: %s\nContent (%d chars):\n\n%s", url, title, len(result.Data.Markdown), content))
}

func (t *QorCrawlTool) crawl(ctx context.Context, url string, maxPages int) *Result {
	body, _ := json.Marshal(map[string]any{
		"url":   url,
		"limit": maxPages,
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", t.baseURL+"/crawl", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return ErrorResult(fmt.Sprintf("QorCrawl crawl error: %v", err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var crawlResp struct {
		Success bool   `json:"success"`
		ID      string `json:"id"`
	}
	json.Unmarshal(data, &crawlResp)

	if !crawlResp.Success || crawlResp.ID == "" {
		return ErrorResult(fmt.Sprintf("QorCrawl crawl failed: %s", string(data[:min(len(data), 200)])))
	}

	// Poll for results (crawl is async). Use a background context so we
	// keep polling even if the request context is cancelled by a timeout.
	pollCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	for i := 0; i < 45; i++ {
		time.Sleep(2 * time.Second)
		statusReq, _ := http.NewRequestWithContext(pollCtx, "GET", t.baseURL+"/crawl/"+crawlResp.ID, nil)
		statusReq.Header.Set("Authorization", "Bearer "+t.apiToken)
		statusResp, err := t.client.Do(statusReq)
		if err != nil {
			if pollCtx.Err() != nil { break }
			continue
		}
		statusData, _ := io.ReadAll(statusResp.Body)
		statusResp.Body.Close()

		var status struct {
			Status string `json:"status"`
			Data   []struct {
				Markdown string            `json:"markdown"`
				Metadata map[string]string `json:"metadata"`
			} `json:"data"`
		}
		json.Unmarshal(statusData, &status)

		if status.Status == "completed" {
			var pages []string
			for i, d := range status.Data {
				if i >= maxPages { break }
				title := d.Metadata["sourceURL"]
				content := d.Markdown
				if len(content) > 3000 { content = content[:3000] + "..." }
				pages = append(pages, fmt.Sprintf("--- Page %d: %s ---\n%s", i+1, title, content))
			}
			slog.Info("qor_crawl.crawl.complete", "url", url, "pages", len(status.Data))
			return TextResult(fmt.Sprintf("🕷️ QorCrawl crawled %d pages from %s:\n\n%s", len(status.Data), url, strings.Join(pages, "\n\n")))
		}
	}

	// Timed out — fall back to scraping just the root URL so the agent gets something
	slog.Warn("qor_crawl.crawl.timeout", "url", url, "job_id", crawlResp.ID)
	slog.Info("qor_crawl.crawl.fallback_scrape", "url", url)
	return t.scrape(ctx, url)
}

func qorCrawlMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
