// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webintel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WebIntel is the Perplexity-grade web intelligence pipeline.
// It searches, scrapes, extracts clean content, and feeds it to the LLM.
type WebIntel struct {
	searxngURL string
	client     *http.Client
}

func New(searxngURL string) *WebIntel {
	return &WebIntel{
		searxngURL: searxngURL,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// SearchResult is a clean search result.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Content string `json:"content,omitempty"` // full extracted text (after scrape)
}

// Search runs parallel queries via SearXNG and returns clean results.
func (w *WebIntel) Search(ctx context.Context, queries []string, maxPerQuery int) []SearchResult {
	if maxPerQuery <= 0 { maxPerQuery = 5 }
	var mu sync.Mutex
	all := []SearchResult{}
	var wg sync.WaitGroup

	for _, q := range queries {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			results := w.searchSearXNG(ctx, query, maxPerQuery)
			mu.Lock()
			all = append(all, results...)
			mu.Unlock()
		}(q)
	}
	wg.Wait()

	// Deduplicate by URL
	seen := make(map[string]bool)
	unique := []SearchResult{}
	for _, r := range all {
		if !seen[r.URL] {
			seen[r.URL] = true
			unique = append(unique, r)
		}
	}
	return unique
}

// SearchAndScrape searches then scrapes top results for full content.
func (w *WebIntel) SearchAndScrape(ctx context.Context, queries []string, scrapeTop int) []SearchResult {
	results := w.Search(ctx, queries, 5)
	if scrapeTop <= 0 { scrapeTop = 3 }
	if scrapeTop > len(results) { scrapeTop = len(results) }

	// Parallel scrape top results
	var wg sync.WaitGroup
	for i := 0; i < scrapeTop; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			content := w.ScrapeURL(ctx, results[idx].URL)
			if content != "" {
				results[idx].Content = content
			}
		}(i)
	}
	wg.Wait()
	return results
}

// ScrapeURL fetches a URL and extracts clean markdown-like text.
func (w *WebIntel) ScrapeURL(ctx context.Context, rawURL string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil { return "" }
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Qorven/2.0; +https://qorven.ai)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := w.client.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()
	if resp.StatusCode >= 400 { return "" }

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500000)) // 500KB max
	html := string(body)

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		return truncate(html, 5000)
	}
	return HTMLToMarkdown(html)
}

// RewriteQueries converts a user question into SEO-friendly search queries.
func RewriteQueries(question string) []string {
	q := strings.TrimSpace(question)
	// Don't strip words — just create variations of the original query
	queries := []string{q}
	if !strings.Contains(strings.ToLower(q), "latest") && !strings.Contains(strings.ToLower(q), "2026") {
		queries = append(queries, q+" latest 2026")
	}
	if len(queries) < 3 {
		queries = append(queries, q+" details")
	}
	return queries[:min(len(queries), 3)]
}

// FormatForLLM formats search results as clean context for the LLM.
func FormatForLLM(results []SearchResult) string {
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s\nURL: %s\n", i+1, r.Title, r.URL))
		if r.Content != "" {
			sb.WriteString(truncate(r.Content, 2000))
		} else if r.Snippet != "" {
			sb.WriteString(r.Snippet)
		}
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// --- SearXNG ---

func (w *WebIntel) searchSearXNG(ctx context.Context, query string, max int) []SearchResult {
	if w.searxngURL == "" {
		return w.searchDDGAPI(ctx, query, max)
	}
	u := fmt.Sprintf("%s/search?q=%s&format=json&engines=google,bing,duckduckgo&pageno=1",
		w.searxngURL, url.QueryEscape(query))

	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := w.client.Do(req)
	if err != nil {
		slog.Warn("searxng.failed", "error", err)
		return w.searchDDGAPI(ctx, query, max)
	}
	defer resp.Body.Close()

	var data struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	results := []SearchResult{}
	for _, r := range data.Results {
		if len(results) >= max { break }
		if r.URL == "" { continue }
		results = append(results, SearchResult{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	return results
}

// DuckDuckGo HTML search fallback (no API key needed)
func (w *WebIntel) searchDDGAPI(ctx context.Context, query string, max int) []SearchResult {
	u := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Qorven/2.0)")
	resp, err := w.client.Do(req)
	if err != nil { return nil }
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200000))
	html := string(body)

	// Extract results from DDG HTML
	results := []SearchResult{}
	// Pattern: <a class="result__a" href="...">title</a> ... <a class="result__snippet">snippet</a>
	linkRe := regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>(.*?)</a>`)

	links := linkRe.FindAllStringSubmatch(html, max*2)
	snippets := snippetRe.FindAllStringSubmatch(html, max*2)

	for i, link := range links {
		if len(results) >= max { break }
		if len(link) < 3 { continue }
		rawURL := link[1]
		// DDG wraps URLs in redirect — extract actual URL
		if idx := strings.Index(rawURL, "uddg="); idx >= 0 {
			decoded, err := url.QueryUnescape(rawURL[idx+5:])
			if err == nil { rawURL = decoded }
			// Trim trailing &
			if ampIdx := strings.Index(rawURL, "&"); ampIdx > 0 { rawURL = rawURL[:ampIdx] }
		}
		title := stripTags(link[2])
		snippet := ""
		if i < len(snippets) && len(snippets[i]) >= 2 {
			snippet = stripTags(snippets[i][1])
		}
		if rawURL != "" && title != "" {
			results = append(results, SearchResult{Title: title, URL: rawURL, Snippet: snippet})
		}
	}
	slog.Info("webintel.ddg_search", "query", query, "results", len(results))
	return results
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	return strings.TrimSpace(re.ReplaceAllString(s, ""))
}

// --- HTML to Markdown ---

// HTMLToMarkdown converts HTML to clean readable text, stripping all junk.
func HTMLToMarkdown(html string) string {
	// 1. Remove everything we don't want
	for _, tag := range []string{"script", "style", "nav", "header", "footer", "noscript", "svg", "iframe", "form", "button", "input", "select", "textarea", "meta", "link"} {
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<%s[^>]*>.*?</%s>`, tag, tag))
		html = re.ReplaceAllString(html, "")
	}
	// Remove self-closing tags
	html = regexp.MustCompile(`(?i)<(meta|link|br|hr|img|input)[^>]*/?\s*>`).ReplaceAllString(html, "\n")

	// 2. Remove sourceMappingURL and base64 data
	html = regexp.MustCompile(`//# sourceMappingURL=\S+`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`data:[^"'\s]+`).ReplaceAllString(html, "")

	// 3. Convert structural HTML to markdown
	// Headings
	for i := 6; i >= 1; i-- {
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<h%d[^>]*>(.*?)</h%d>`, i, i))
		prefix := strings.Repeat("#", i)
		html = re.ReplaceAllString(html, "\n"+prefix+" $1\n")
	}
	// Paragraphs
	html = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`).ReplaceAllString(html, "\n$1\n")
	// List items
	html = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`).ReplaceAllString(html, "- $1\n")
	// Bold
	html = regexp.MustCompile(`(?is)<(b|strong)[^>]*>(.*?)</(b|strong)>`).ReplaceAllString(html, "**$2**")
	// Italic
	html = regexp.MustCompile(`(?is)<(i|em)[^>]*>(.*?)</(i|em)>`).ReplaceAllString(html, "*$2*")
	// Links — keep text and URL
	html = regexp.MustCompile(`(?is)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(html, "$2 ($1)")
	// Blockquote
	html = regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`).ReplaceAllString(html, "> $1\n")

	// 4. Strip remaining HTML tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")

	// 5. Decode entities
	replacer := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"",
		"&#39;", "'", "&nbsp;", " ", "&#x27;", "'", "&#x2F;", "/",
		"&apos;", "'", "&mdash;", "—", "&ndash;", "–", "&hellip;", "…",
	)
	html = replacer.Replace(html)

	// 6. Clean up whitespace
	html = regexp.MustCompile(`[ \t]+`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")

	// 7. Remove lines that are just noise (very short, just symbols, etc.)
	clean := []string{}
	for _, line := range strings.Split(html, "\n") {
		line = strings.TrimSpace(line)
		if line == "" { continue }
		if len(line) < 3 { continue }
		// Skip lines that are mostly non-alpha (JS remnants)
		alpha := 0
		for _, c := range line {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == ' ' { alpha++ }
		}
		if len(line) > 20 && float64(alpha)/float64(len(line)) < 0.3 { continue }
		clean = append(clean, line)
	}

	result := strings.Join(clean, "\n")
	return truncate(result, 8000)
}

func truncate(s string, max int) string {
	if len(s) <= max { return s }
	return s[:max] + "\n[truncated]"
}
