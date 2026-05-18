// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Patterns from QorCrawl: sitemap crawling, HTML→Markdown, link extraction

// SitemapURL represents a URL entry in a sitemap.
type SitemapURL struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	ChangeFreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

// Sitemap represents a parsed sitemap.xml.
type Sitemap struct {
	URLs     []SitemapURL `xml:"url"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// CrawlSitemap fetches and parses a sitemap, returning all URLs.
// Handles sitemap index files (recursive), gzipped sitemaps, and limits.
func (s *Scraper) CrawlSitemap(ctx context.Context, sitemapURL string, maxURLs int) ([]string, error) {
	if maxURLs <= 0 {
		maxURLs = 100
	}

	body, err := s.fetchRaw(ctx, sitemapURL)
	if err != nil {
		// Try common sitemap locations
		for _, path := range []string{"/sitemap.xml", "/sitemap_index.xml", "/sitemap.xml.gz"} {
			base := strings.TrimRight(sitemapURL, "/")
			if !strings.Contains(sitemapURL, "sitemap") {
				body, err = s.fetchRaw(ctx, base+path)
				if err == nil {
					break
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("sitemap fetch failed: %w", err)
		}
	}

	var sitemap Sitemap
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		return nil, fmt.Errorf("sitemap parse: %w", err)
	}

	urls := []string{}

	// If it's a sitemap index, recursively fetch each sub-sitemap
	if len(sitemap.Sitemaps) > 0 {
		for _, sub := range sitemap.Sitemaps {
			if len(urls) >= maxURLs {
				break
			}
			subURLs, err := s.CrawlSitemap(ctx, sub.Loc, maxURLs-len(urls))
			if err != nil {
				slog.Warn("sub-sitemap failed", "url", sub.Loc, "error", err)
				continue
			}
			urls = append(urls, subURLs...)
		}
	}

	// Collect URLs from this sitemap
	for _, u := range sitemap.URLs {
		if len(urls) >= maxURLs {
			break
		}
		if u.Loc != "" {
			urls = append(urls, u.Loc)
		}
	}

	return urls, nil
}

func (s *Scraper) fetchRaw(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.HasSuffix(url, ".gz") || resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
	}

	return io.ReadAll(io.LimitReader(reader, 10*1024*1024)) // 10MB limit
}

// HTMLToMarkdown converts HTML to clean Markdown text.
// QorCrawl pattern: strip tags, preserve structure, clean whitespace.
func HTMLToMarkdown(html string) string {
	// Remove script/style
	html = regexp.MustCompile(`(?is)<script.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<style.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<!--.*?-->`).ReplaceAllString(html, "")

	// Convert headers
	for i := 6; i >= 1; i-- {
		prefix := strings.Repeat("#", i)
		html = regexp.MustCompile(fmt.Sprintf(`(?is)<h%d[^>]*>(.*?)</h%d>`, i, i)).ReplaceAllString(html, "\n"+prefix+" $1\n")
	}

	// Convert common elements
	html = regexp.MustCompile(`(?is)<br\s*/?\s*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`).ReplaceAllString(html, "\n$1\n")
	html = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`).ReplaceAllString(html, "- $1\n")
	html = regexp.MustCompile(`(?is)<strong[^>]*>(.*?)</strong>`).ReplaceAllString(html, "**$1**")
	html = regexp.MustCompile(`(?is)<b[^>]*>(.*?)</b>`).ReplaceAllString(html, "**$1**")
	html = regexp.MustCompile(`(?is)<em[^>]*>(.*?)</em>`).ReplaceAllString(html, "*$1*")
	html = regexp.MustCompile(`(?is)<i[^>]*>(.*?)</i>`).ReplaceAllString(html, "*$1*")
	html = regexp.MustCompile(`(?is)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(html, "[$2]($1)")
	html = regexp.MustCompile(`(?is)<code[^>]*>(.*?)</code>`).ReplaceAllString(html, "`$1`")
	html = regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`).ReplaceAllString(html, "\n```\n$1\n```\n")
	html = regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`).ReplaceAllString(html, "> $1\n")

	// Strip remaining tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")

	// Decode entities
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// Clean whitespace
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")
	return strings.TrimSpace(html)
}

// ExtractMetadata extracts page metadata (title, description, og tags).
func (p *Page) ExtractMetadata() map[string]string {
	meta := map[string]string{"title": p.Title()}

	p.Doc.Find("meta").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		property, _ := s.Attr("property")
		content, _ := s.Attr("content")
		key := name
		if key == "" {
			key = property
		}
		if key != "" && content != "" {
			meta[key] = content
		}
	})

	// Canonical URL
	p.Doc.Find("link[rel=canonical]").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			meta["canonical"] = href
		}
	})

	return meta
}
