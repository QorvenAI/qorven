// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Scraper is a web scraping engine with CSS/XPath selectors, anti-detection, and caching.
type Scraper struct {
	client    *http.Client
	userAgent string
	headers   map[string]string
	timeout   time.Duration
	stealth       bool
	proxyRotator  *ProxyRotator
	retries   int
}

// New creates a scraper with sensible defaults.
func New() *Scraper {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		MaxResponseHeaderBytes: 1 << 20, // 1MB header limit
	}
	return &Scraper{
		client: &http.Client{Timeout: 30 * time.Second, Transport: transport},
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		headers:   map[string]string{"Accept": "text/html,application/xhtml+xml", "Accept-Language": "en-US,en;q=0.9"},
		timeout:   30 * time.Second,
		retries:   2,
	}
}

// SetUserAgent overrides the default user agent.
func (s *Scraper) SetUserAgent(ua string) { s.userAgent = ua }

// SetHeader adds a custom header.
func (s *Scraper) SetHeader(key, value string) { s.headers[key] = value }

// Page represents a fetched and parsed web page.
type Page struct {
	URL        string
	StatusCode int
	HTML       string
	Doc        *goquery.Document
	FetchedAt  time.Time
	Elapsed    time.Duration
}

// Fetch downloads and parses a URL.
func (s *Scraper) Fetch(ctx context.Context, url string) (*Page, error) {
	start := time.Now()
	var lastErr error

	for attempt := 0; attempt <= s.retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("scraper: build request: %w", err)
		}
		req.Header.Set("User-Agent", s.userAgent)
		for k, v := range s.headers {
			req.Header.Set(k, v)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
		if err != nil {
			lastErr = err
			continue
		}

		elapsed := time.Since(start)
		slog.Debug("scraper.fetch", "url", url, "status", resp.StatusCode, "bytes", len(body), "elapsed", elapsed)

		return &Page{
			URL:        url,
			StatusCode: resp.StatusCode,
			HTML:       string(body),
			Doc:        doc,
			FetchedAt:  time.Now(),
			Elapsed:    elapsed,
		}, nil
	}

	return nil, fmt.Errorf("scraper: fetch %s failed after %d retries: %w", url, s.retries, lastErr)
}

// Select returns elements matching a CSS selector.
func (p *Page) Select(selector string) []Element {
	elements := []Element{}
	p.Doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		elements = append(elements, Element{sel: s})
	})
	return elements
}

// SelectFirst returns the first element matching a CSS selector.
func (p *Page) SelectFirst(selector string) *Element {
	s := p.Doc.Find(selector).First()
	if s.Length() == 0 {
		return nil
	}
	return &Element{sel: s}
}

// Text extracts all visible text from the page.
func (p *Page) Text() string {
	return strings.TrimSpace(p.Doc.Text())
}

// Title returns the page title.
func (p *Page) Title() string {
	return p.Doc.Find("title").Text()
}

// Links returns all href links on the page.
func (p *Page) Links() []string {
	links := []string{}
	p.Doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			links = append(links, href)
		}
	})
	return links
}

// Element wraps a goquery selection with convenience methods.
type Element struct {
	sel *goquery.Selection
}

func (e *Element) Text() string                    { return strings.TrimSpace(e.sel.Text()) }
func (e *Element) HTML() string                    { h, _ := e.sel.Html(); return h }
func (e *Element) Attr(name string) string          { v, _ := e.sel.Attr(name); return v }
func (e *Element) HasAttr(name string) bool         { _, exists := e.sel.Attr(name); return exists }
func (e *Element) HasClass(class string) bool       { return e.sel.HasClass(class) }
func (e *Element) Children(selector string) []Element {
	children := []Element{}
	e.sel.Find(selector).Each(func(i int, s *goquery.Selection) {
		children = append(children, Element{sel: s})
	})
	return children
}

// Table extracts a table as rows of cells.
func (p *Page) Table(selector string) [][]string {
	rows := [][]string{}
	p.Doc.Find(selector + " tr").Each(func(i int, tr *goquery.Selection) {
		cells := []string{}
		tr.Find("td, th").Each(func(j int, td *goquery.Selection) {
			cells = append(cells, strings.TrimSpace(td.Text()))
		})
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	})
	return rows
}

// JSON fetches a URL and returns the raw JSON body.
func (s *Scraper) JSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", "application/json")
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
