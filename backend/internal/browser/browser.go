// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"os"
	"os/exec"
	"fmt"
	"log/slog"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser is a stealth headless Chrome browser for web scraping.
// Anti-bot techniques: random UA, disable automation flags, realistic timing.
type Browser struct {
	mu       sync.Mutex
	allocCtx context.Context
	cancel   context.CancelFunc
	ready    bool
}

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
}

// New creates a stealth browser instance.
func New() *Browser {
	return &Browser{}
}

// Init starts the browser process with stealth flags.
func (b *Browser) Init() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ready { return nil }

	// Find Chrome binary — check env, common paths
	chromePath := findChrome()
	if chromePath == "" {
		return fmt.Errorf("chromium not found — run install.sh or set CHROME_PATH")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		// Stealth flags — disable automation detection
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("disable-features", "site-per-process"),
		chromedp.Flag("disable-infobars", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("headless", "new"), // new headless mode (less detectable)
		chromedp.UserAgent(userAgents[rand.Intn(len(userAgents))]),
		chromedp.WindowSize(1920, 1080),
	)

	b.allocCtx, b.cancel = chromedp.NewExecAllocator(context.Background(), opts...)
	b.ready = true
	slog.Info("browser.initialized", "mode", "stealth-headless")
	return nil
}

// FetchResult is the result of a page fetch.
type FetchResult struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	HTML     string `json:"-"`
	Markdown string `json:"markdown"`
	Error    string `json:"error,omitempty"`
}

// Fetch loads a URL with full JavaScript rendering and returns clean markdown.
func (b *Browser) Fetch(ctx context.Context, url string) *FetchResult {
	if !b.ready {
		if err := b.Init(); err != nil {
			return &FetchResult{URL: url, Error: "browser not available: " + err.Error()}
		}
	}

	tabCtx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	// Timeout per page
	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 30*time.Second)
	defer timeoutCancel()

	var title, html string
	err := chromedp.Run(tabCtx,
		// Navigate with random delay (anti-bot)
		chromedp.Navigate(url),
		chromedp.Sleep(time.Duration(500+rand.Intn(1500))*time.Millisecond), // 0.5-2s random wait
		chromedp.WaitReady("body"),
		chromedp.Title(&title),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		slog.Warn("browser.fetch_failed", "url", url, "error", err)
		return &FetchResult{URL: url, Error: err.Error()}
	}

	md := HTMLToMarkdown(html)
	slog.Info("browser.fetched", "url", url, "title", title, "md_len", len(md))
	return &FetchResult{URL: url, Title: title, HTML: html, Markdown: md}
}

// FetchMultiple fetches multiple URLs in parallel.
func (b *Browser) FetchMultiple(ctx context.Context, urls []string, maxConcurrent int) []*FetchResult {
	if maxConcurrent <= 0 { maxConcurrent = 3 }
	results := make([]*FetchResult, len(urls))
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = b.Fetch(ctx, url)
		}(i, u)
	}
	wg.Wait()
	return results
}

// Close shuts down the browser.
func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil { b.cancel() }
	b.ready = false
}

// HTMLToMarkdown converts full rendered HTML to clean structured markdown.
func HTMLToMarkdown(html string) string {
	// 1. Remove unwanted elements
	for _, tag := range []string{"script", "style", "nav", "header", "footer", "noscript", "svg",
		"iframe", "form", "button", "input", "select", "textarea", "meta", "link",
		"aside", "dialog", "template"} {
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<%s[^>]*>.*?</%s>`, tag, tag))
		html = re.ReplaceAllString(html, "")
	}

	// 2. Remove data URIs, source maps, base64
	html = regexp.MustCompile(`//# sourceMappingURL=\S+`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`data:[^"'\s]{50,}`).ReplaceAllString(html, "")

	// 3. Convert structural HTML to markdown
	// Headings
	for i := 6; i >= 1; i-- {
		re := regexp.MustCompile(fmt.Sprintf(`(?is)<h%d[^>]*>(.*?)</h%d>`, i, i))
		html = re.ReplaceAllString(html, "\n"+strings.Repeat("#", i)+" $1\n")
	}
	// Paragraphs and divs with content
	html = regexp.MustCompile(`(?is)<p[^>]*>(.*?)</p>`).ReplaceAllString(html, "\n$1\n")
	html = regexp.MustCompile(`(?is)<br\s*/?\s*>`).ReplaceAllString(html, "\n")
	// Lists
	html = regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`).ReplaceAllString(html, "- $1\n")
	// Bold/italic
	html = regexp.MustCompile(`(?is)<(b|strong)[^>]*>(.*?)</(b|strong)>`).ReplaceAllString(html, "**$2**")
	html = regexp.MustCompile(`(?is)<(i|em)[^>]*>(.*?)</(i|em)>`).ReplaceAllString(html, "*$2*")
	// Links
	html = regexp.MustCompile(`(?is)<a[^>]*href="([^"]*)"[^>]*>(.*?)</a>`).ReplaceAllString(html, "[$2]($1)")
	// Tables
	html = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`).ReplaceAllString(html, "$1|\n")
	html = regexp.MustCompile(`(?is)<t[hd][^>]*>(.*?)</t[hd]>`).ReplaceAllString(html, "| $1 ")
	// Blockquote
	html = regexp.MustCompile(`(?is)<blockquote[^>]*>(.*?)</blockquote>`).ReplaceAllString(html, "> $1\n")

	// 4. Strip remaining tags
	html = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, " ")

	// 5. Decode entities
	r := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"",
		"&#39;", "'", "&nbsp;", " ", "&#x27;", "'", "&apos;", "'",
		"&mdash;", "—", "&ndash;", "–", "&hellip;", "…",
	)
	html = r.Replace(html)

	// 6. Clean whitespace
	html = regexp.MustCompile(`[ \t]+`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")

	// 7. Filter noise lines
	clean := []string{}
	for _, line := range strings.Split(html, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || len(line) < 3 { continue }
		// Skip lines that are mostly non-alpha (JS/CSS remnants)
		alpha := 0
		for _, c := range line {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == ' ' { alpha++ }
		}
		if len(line) > 20 && float64(alpha)/float64(len(line)) < 0.3 { continue }
		clean = append(clean, line)
	}

	result := strings.Join(clean, "\n")
	if len(result) > 10000 { result = result[:10000] + "\n[truncated]" }
	return result
}

// findChrome locates the Chrome/Chromium binary.
func findChrome() string {
	// 1. Environment variable
	if p := os.Getenv("CHROME_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil { return p }
	}
	// 2. Common paths
	paths := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/snap/bin/chromium",
		os.Getenv("HOME") + "/.cache/ms-playwright/chromium-1208/chrome-linux/chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil { return p }
	}
	// 3. Search in PATH
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome"} {
		if p, err := exec.LookPath(name); err == nil { return p }
	}
	return ""
}
