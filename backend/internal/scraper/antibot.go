// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scraper

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	"github.com/imroc/req/v3"
)

// AntiBot is a 5-layer anti-bot scraper that defeats modern WAFs.
//
// Layer 1: TLS/JA3 — req library spoofs Chrome/Firefox TLS fingerprint
// Layer 2: HTTP/2 — req handles SETTINGS frames matching real browsers
// Layer 3: Headers — req preserves header order, generates browser-consistent headers
// Layer 4: Browser — go-rod with stealth patches for JS-rendered pages
// Layer 5: IP/Rate — proxy rotation + random jitter between requests

type AntiBot struct {
	client     *req.Client
	proxies    []string
	proxyIdx   int
	minDelay   time.Duration
	maxDelay   time.Duration
	browserPath string
}

// NewAntiBot creates a full anti-bot scraper.
func NewAntiBot() *AntiBot {
	client := req.C().
		SetTLSFingerprintChrome(). // Layer 1: Chrome JA3/JA4
		SetTimeout(30 * time.Second).
		SetRedirectPolicy(req.MaxRedirectPolicy(10)).
		ImpersonateChrome().
		EnableAutoDecompress() // Layer 2+3: HTTP/2 frames + header order

	return &AntiBot{
		client:   client,
		minDelay: 500 * time.Millisecond,
		maxDelay: 3 * time.Second,
	}
}

// SetProxies configures proxy rotation (Layer 5).
func (ab *AntiBot) SetProxies(proxies []string) {
	ab.proxies = proxies
}

// SetBrowserPath sets the Chrome/Chromium path for JS rendering.
func (ab *AntiBot) SetBrowserPath(path string) {
	ab.browserPath = path
}

// Fetch scrapes a URL using the full anti-bot stack.
// Tries HTTP first, falls back to headless browser for JS-rendered pages.
func (ab *AntiBot) Fetch(ctx context.Context, rawURL string) (*ScrapeResult, error) {
	// Layer 5: Random jitter
	ab.jitter()

	// Layer 5: Rotate proxy
	if len(ab.proxies) > 0 {
		proxy := ab.proxies[ab.proxyIdx%len(ab.proxies)]
		ab.proxyIdx++
		ab.client.SetProxyURL(proxy)
	}

	// Layer 1+2+3: HTTP fetch with spoofed TLS + H2 + headers
	profile := GenerateProfile()

	resp, err := ab.client.R().
		SetContext(ctx).
		SetHeaders(profile.Headers).
		SetHeader("User-Agent", profile.UserAgent).
		Get(rawURL)

	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	result := &ScrapeResult{
		URL:        rawURL,
		StatusCode: resp.StatusCode,
		Engine:     "http",
	}

	if resp.StatusCode == 200 {
		result.HTML = resp.String()
		result.Text = htmlToText(result.HTML)
		slog.Info("antibot.fetch.success", "url", rawURL, "engine", "http", "bytes", len(result.HTML))
		return result, nil
	}

	// HTTP failed (403/429) — try headless browser with stealth (Layer 4)
	if resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode == 503 {
		slog.Info("antibot.fetch.blocked", "url", rawURL, "status", resp.StatusCode, "trying", "browser")
		browserResult, err := ab.fetchWithBrowser(ctx, rawURL)
		if err == nil {
			return browserResult, nil
		}
		slog.Warn("antibot.browser.failed", "url", rawURL, "error", err)
	}

	// Return whatever we got
	result.HTML = resp.String()
	result.Text = htmlToText(result.HTML)
	return result, nil
}

// fetchWithBrowser uses go-rod with stealth patches for JS-rendered pages.
func (ab *AntiBot) fetchWithBrowser(ctx context.Context, rawURL string) (*ScrapeResult, error) {
	// Find browser
	path := ab.browserPath
	if path == "" {
		var found bool
		path, found = launcher.LookPath()
		if !found {
			return nil, fmt.Errorf("no browser found — set CHROME_PATH or install chromium")
		}
	}

	// Launch with stealth
	u := launcher.New().
		Bin(path).
		Headless(true).
		NoSandbox(true).
		Set("disable-blink-features", "AutomationControlled").
		MustLaunch()

	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	// Create stealth page (patches navigator.webdriver, plugins, Canvas, etc.)
	page := stealth.MustPage(browser)
	defer page.MustClose()

	// Navigate with timeout
	err := rod.Try(func() {
		page.Timeout(20 * time.Second).MustNavigate(rawURL).MustWaitStable()
	})
	if err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	// Extract content
	html := page.MustHTML()
	title := page.MustEval(`() => document.title`).String()

	result := &ScrapeResult{
		URL:        rawURL,
		StatusCode: 200,
		HTML:       html,
		Text:       htmlToText(html),
		Title:      title,
		Engine:     "browser-stealth",
	}

	slog.Info("antibot.browser.success", "url", rawURL, "bytes", len(html), "title", title)
	return result, nil
}

func (ab *AntiBot) jitter() {
	if ab.maxDelay <= ab.minDelay { return }
	delay := ab.minDelay + time.Duration(rand.Int63n(int64(ab.maxDelay-ab.minDelay)))
	time.Sleep(delay)
}

// ScrapeResult holds the result of a scrape.
type ScrapeResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	HTML       string `json:"html"`
	Text       string `json:"text"`
	Title      string `json:"title"`
	Engine     string `json:"engine"` // "http", "browser-stealth"
}

// --- Convenience functions ---

// QuickFetch is a one-liner to scrape any URL with full anti-bot.
func QuickFetch(ctx context.Context, rawURL string) (string, error) {
	ab := NewAntiBot()
	result, err := ab.Fetch(ctx, rawURL)
	if err != nil { return "", err }
	if result.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", result.StatusCode)
	}
	return result.Text, nil
}

// QuickFetchHTML returns raw HTML.
func QuickFetchHTML(ctx context.Context, rawURL string) (string, int, error) {
	ab := NewAntiBot()
	result, err := ab.Fetch(ctx, rawURL)
	if err != nil { return "", 0, err }
	return result.HTML, result.StatusCode, nil
}

// CrawlPages scrapes multiple URLs with jitter between requests.
func CrawlPages(ctx context.Context, urls []string) []ScrapeResult {
	ab := NewAntiBot()
	results := []ScrapeResult{}
	for _, u := range urls {
		result, err := ab.Fetch(ctx, u)
		if err != nil {
			results = append(results, ScrapeResult{URL: u, StatusCode: 0, Text: err.Error()})
			continue
		}
		results = append(results, *result)
	}
	return results
}

// ExtractLinks returns all links from a page.
func ExtractLinks(html, baseURL string) []string {
	links := []string{}
	base, _ := url.Parse(baseURL)
	// Simple link extraction
	for _, marker := range []string{`href="`, `href='`} {
		rest := html
		for {
			idx := strings.Index(rest, marker)
			if idx < 0 { break }
			rest = rest[idx+len(marker):]
			end := strings.IndexAny(rest, `"'`)
			if end < 0 { break }
			link := rest[:end]
			if strings.HasPrefix(link, "/") && base != nil {
				link = base.Scheme + "://" + base.Host + link
			}
			if strings.HasPrefix(link, "http") {
				links = append(links, link)
			}
			rest = rest[end:]
		}
	}
	return links
}

// Needed for io import
var _ = io.EOF
