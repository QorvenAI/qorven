// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// ExtractToMarkdown converts HTML to clean markdown for LLM consumption.
// Tries QorCrawl first (if key), falls back to built-in converter.
func ExtractToMarkdown(ctx context.Context, rawURL, html, qor_crawlKey string, client *http.Client) string {
	// Try QorCrawl API first
	if qor_crawlKey != "" {
		if md := qor_crawlExtract(ctx, rawURL, qor_crawlKey, client); md != "" {
			return md
		}
	}
	// Built-in HTML → Markdown
	return htmlToMarkdown(html)
}

// qor_crawlExtract uses QorCrawl API to get clean markdown.
func qor_crawlExtract(ctx context.Context, rawURL, apiKey string, client *http.Client) string {
	body, _ := json.Marshal(map[string]any{"url": rawURL, "formats": []string{"markdown"}})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.qor_crawl.dev/v1/scrape", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 { return "" }
	defer resp.Body.Close()
	var data struct {
		Data struct {
			Markdown string `json:"markdown"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	md := data.Data.Markdown
	if len(md) > 4000 { md = md[:4000] + "\n\n..." }
	return md
}

// htmlToMarkdown converts HTML to markdown — free, no API needed.
func htmlToMarkdown(html string) string {
	s := html

	// Remove unwanted tags entirely
	for _, tag := range []string{"script", "style", "nav", "header", "footer", "aside", "noscript", "svg", "iframe"} {
		re := regexp.MustCompile("(?is)<" + tag + "[^>]*>.*?</" + tag + ">")
		s = re.ReplaceAllString(s, "")
	}

	// Convert headings
	for i := 6; i >= 1; i-- {
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<h%d[^>]*>(.*?)</h%d>`, i, i))
		prefix := strings.Repeat("#", i)
		s = re.ReplaceAllString(s, "\n"+prefix+" $1\n")
	}

	// Convert paragraphs
	s = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`).ReplaceAllString(s, "\n$1\n")

	// Convert links
	s = regexp.MustCompile(`(?is)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(s, "[$2]($1)")

	// Convert bold/strong
	s = regexp.MustCompile(`(?is)<(strong|b)[^>]*>(.*?)</(strong|b)>`).ReplaceAllString(s, "**$2**")

	// Convert italic/em
	s = regexp.MustCompile(`(?is)<(em|i)[^>]*>(.*?)</(em|i)>`).ReplaceAllString(s, "*$2*")

	// Convert list items
	s = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`).ReplaceAllString(s, "- $1")

	// Convert code blocks
	s = regexp.MustCompile(`(?is)<pre[^>]*><code[^>]*>(.*?)</code></pre>`).ReplaceAllString(s, "\n```\n$1\n```\n")
	s = regexp.MustCompile(`(?is)<code[^>]*>(.*?)</code>`).ReplaceAllString(s, "`$1`")

	// Convert blockquotes
	s = regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`).ReplaceAllString(s, "\n> $1\n")

	// Convert images (keep alt text)
	s = regexp.MustCompile(`(?is)<img[^>]*alt="([^"]*)"[^>]*>`).ReplaceAllString(s, "![$1]")

	// Convert tables (basic)
	s = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`).ReplaceAllString(s, "$1|\n")
	s = regexp.MustCompile(`(?is)<t[hd][^>]*>(.*?)</t[hd]>`).ReplaceAllString(s, "| $1 ")

	// Convert br to newline
	s = regexp.MustCompile(`(?i)<br\s*/?\s*>`).ReplaceAllString(s, "\n")

	// Strip remaining HTML tags
	s = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")

	// Decode HTML entities
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")

	// Clean up whitespace
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	s = regexp.MustCompile(`[ \t]+`).ReplaceAllString(s, " ")

	// Trim and limit
	s = strings.TrimSpace(s)
	if len(s) > 4000 { s = s[:4000] + "\n\n..." }
	return s
}
