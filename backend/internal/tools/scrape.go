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

// ScrapeTool allows agents to scrape web pages with CSS selectors.
type ScrapeTool struct {
	scraper *scraper.Scraper
}

func NewScrapeTool() *ScrapeTool {
	return &ScrapeTool{scraper: scraper.New()}
}

func (t *ScrapeTool) Name() string { return "scrape" }

func (t *ScrapeTool) Description() string {
	return "Scrape a web page and extract data using CSS selectors. Returns text content, links, tables, or specific elements."
}

func (t *ScrapeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "URL to scrape",
			},
			"selector": map[string]any{
				"type":        "string",
				"description": "CSS selector to extract specific elements (optional, returns full text if empty)",
			},
			"extract": map[string]any{
				"type":        "string",
				"enum":        []string{"text", "html", "links", "table", "attr"},
				"description": "What to extract: text (default), html, links, table, or attr",
			},
			"attr": map[string]any{
				"type":        "string",
				"description": "Attribute name to extract (when extract=attr)",
			},
		},
		"required": []string{"url"},
	}
}

func (t *ScrapeTool) Execute(ctx context.Context, args map[string]any) *Result {
	url, _ := args["url"].(string)
	selector, _ := args["selector"].(string)
	extract, _ := args["extract"].(string)
	attr, _ := args["attr"].(string)

	if url == "" {
		return ErrorResult("url is required")
	}
	if err := ValidateURL(url); err != nil {
		return ErrorResult(fmt.Sprintf("SSRF protection: %v", err))
	}
	if extract == "" {
		extract = "text"
	}

	page, err := t.scraper.Fetch(ctx, url)
	if err != nil {
		return ErrorResult(fmt.Sprintf("scrape failed: %v", err))
	}

	if page.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("HTTP %d for %s", page.StatusCode, url))
	}

	switch extract {
	case "links":
		links := page.Links()
		if len(links) > 50 {
			links = links[:50]
		}
		return TextResult(strings.Join(links, "\n"))

	case "table":
		sel := "table"
		if selector != "" {
			sel = selector
		}
		rows := page.Table(sel)
		var sb strings.Builder
		for _, row := range rows {
			sb.WriteString(strings.Join(row, " | "))
			sb.WriteString("\n")
		}
		return TextResult(sb.String())

	case "html":
		if selector != "" {
			el := page.SelectFirst(selector)
			if el == nil {
				return ErrorResult(fmt.Sprintf("no element matches %q", selector))
			}
			return TextResult(el.HTML())
		}
		html := page.HTML
		if len(html) > MaxToolOutput {
			html = html[:MaxToolOutput]
		}
		return TextResult(html)

	case "attr":
		if selector == "" || attr == "" {
			return ErrorResult("selector and attr required for extract=attr")
		}
		elements := page.Select(selector)
		var values []string
		for _, el := range elements {
			if v := el.Attr(attr); v != "" {
				values = append(values, v)
			}
		}
		return TextResult(strings.Join(values, "\n"))

	default: // text
		if selector != "" {
			elements := page.Select(selector)
			var texts []string
			for _, el := range elements {
				if t := el.Text(); t != "" {
					texts = append(texts, t)
				}
			}
			result := strings.Join(texts, "\n")
			if len(result) > MaxToolOutput {
				result = result[:MaxToolOutput]
			}
			return TextResult(result)
		}
		text := page.Text()
		if len(text) > MaxToolOutput {
			text = text[:MaxToolOutput]
		}
		return TextResult(fmt.Sprintf("Title: %s\n\n%s", page.Title(), text))
	}
}
