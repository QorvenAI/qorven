// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/scraper"
)

// CrawlTool crawls a website's sitemap and extracts content from pages.
type CrawlTool struct {
	scraper *scraper.Scraper
}

func NewCrawlTool() *CrawlTool {
	return &CrawlTool{scraper: scraper.New()}
}

func (t *CrawlTool) Name() string { return "crawl" }
func (t *CrawlTool) Description() string {
	return "Crawl a website by discovering pages from its sitemap, then extract content. Use for comprehensive site analysis."
}
func (t *CrawlTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":       map[string]any{"type": "string", "description": "Website URL to crawl"},
			"max_pages": map[string]any{"type": "integer", "description": "Max pages to crawl (default 10)"},
			"extract":   map[string]any{"type": "string", "enum": []string{"text", "markdown", "links"}, "description": "What to extract from each page"},
		},
		"required": []string{"url"},
	}
}

func (t *CrawlTool) Execute(ctx context.Context, args map[string]any) *Result {
	url, _ := args["url"].(string)
	maxPages := 10
	if mp, ok := args["max_pages"].(float64); ok && mp > 0 {
		maxPages = int(mp)
	}
	extract, _ := args["extract"].(string)
	if extract == "" {
		extract = "markdown"
	}
	if url == "" {
		return ErrorResult("url is required")
	}

	// Discover pages via sitemap
	urls, err := t.scraper.CrawlSitemap(ctx, url, maxPages)
	if err != nil || len(urls) == 0 {
		// Fallback: just scrape the given URL
		urls = []string{url}
	}

	var report strings.Builder
	report.WriteString(fmt.Sprintf("# Crawl: %s\nPages found: %d\n\n", url, len(urls)))

	for i, pageURL := range urls {
		if i >= maxPages {
			break
		}
		page, err := t.scraper.Fetch(ctx, pageURL)
		if err != nil {
			report.WriteString(fmt.Sprintf("## %d. %s\nError: %v\n\n", i+1, pageURL, err))
			continue
		}

		switch extract {
		case "markdown":
			md := scraper.HTMLToMarkdown(page.HTML)
			if len(md) > 2000 {
				md = md[:2000] + "..."
			}
			report.WriteString(fmt.Sprintf("## %d. %s\n%s\n\n", i+1, page.Title(), md))
		case "links":
			links := page.Links()
			if len(links) > 20 {
				links = links[:20]
			}
			report.WriteString(fmt.Sprintf("## %d. %s (%d links)\n%s\n\n", i+1, pageURL, len(links), strings.Join(links, "\n")))
		default:
			text := page.Text()
			if len(text) > 2000 {
				text = text[:2000] + "..."
			}
			report.WriteString(fmt.Sprintf("## %d. %s\n%s\n\n", i+1, page.Title(), text))
		}
	}

	result := report.String()
	if len(result) > MaxToolOutput {
		result = result[:MaxToolOutput]
	}
	return TextResult(result)
}
