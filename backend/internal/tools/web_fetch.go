// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"github.com/qorvenai/qorven/internal/scraper"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultFetchMaxChars    = 60000
	defaultFetchMaxRedirect = 3
	defaultErrorMaxChars    = 4000
	fetchTimeoutSeconds     = 30
	fetchUserAgent          = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

// WebFetchTool fetches URLs and extracts content.
type WebFetchTool struct {
	maxChars       int
	cache          *webFetchCache
	policy         string   // "allow_all" (default), "allowlist"
	allowedDomains []string
	blockedDomains []string
	browserFetch   func(ctx context.Context, url string) (string, error)
	engineRouter   *scraper.EngineRouter // headless browser fallback
	mu             sync.RWMutex
}

// SetBrowserFallback sets a function that fetches pages via headless browser.
// Called when plain HTTP gets 403/bot-protection.
func (t *WebFetchTool) SetBrowserFallback(fn func(ctx context.Context, url string) (string, error)) {
	t.browserFetch = fn
}

func (t *WebFetchTool) SetEngineRouter(r *scraper.EngineRouter) { t.engineRouter = r }

// WebFetchConfig holds configuration for the web fetch tool.
type WebFetchConfig struct {
	MaxChars       int
	CacheTTL       time.Duration
	Policy         string
	AllowedDomains []string
	BlockedDomains []string
}

func NewWebFetchToolWithConfig(cfg WebFetchConfig) *WebFetchTool {
	maxChars := cfg.MaxChars
	if maxChars <= 0 {
		maxChars = defaultFetchMaxChars
	}
	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	policy := cfg.Policy
	if policy == "" {
		policy = "allow_all"
	}
	return &WebFetchTool{
		maxChars:       maxChars,
		cache:          newWebFetchCache(100, ttl),
		policy:         policy,
		allowedDomains: cfg.AllowedDomains,
		blockedDomains: cfg.BlockedDomains,
	}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return "Fetch a URL and extract its content. Supports HTML (converted to markdown/text), JSON, and plain text. Includes SSRF protection."
}

func (t *WebFetchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "HTTP or HTTPS URL to fetch.",
			},
			"extractMode": map[string]any{
				"type":        "string",
				"description": `Extraction mode ("markdown" or "text"). Default: "markdown".`,
				"enum":        []string{"markdown", "text"},
			},
			"maxChars": map[string]any{
				"type":        "number",
				"description": "Maximum characters to return. Default: 60000.",
			},
		},
		"required": []string{"url"},
	}
}

func (t *WebFetchTool) Execute(ctx context.Context, args map[string]any) *Result {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return ErrorResult("url is required")
	}

	// Validate URL scheme
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid URL: %v", err))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrorResult("only http and https URLs are supported")
	}
	if parsed.Host == "" {
		return ErrorResult("missing hostname in URL")
	}

	// SSRF protection
	if err := ValidateURL(rawURL); err != nil {
		return ErrorResult(fmt.Sprintf("SSRF protection: %v", err))
	}

	hostname := parsed.Hostname()

	// Domain blocklist check
	if t.isDomainBlocked(hostname) {
		return ErrorResult(fmt.Sprintf("domain %q is blocked by policy", hostname))
	}

	// Domain allowlist check
	t.mu.RLock()
	policy := t.policy
	t.mu.RUnlock()
	if policy == "allowlist" && !t.isDomainAllowed(hostname) {
		return ErrorResult(fmt.Sprintf("domain %q is not in the allowed domains list", hostname))
	}

	extractMode := "markdown"
	if em, ok := args["extractMode"].(string); ok && (em == "markdown" || em == "text") {
		extractMode = em
	}

	maxChars := t.maxChars
	if mc, ok := args["maxChars"].(float64); ok && int(mc) >= 100 {
		maxChars = int(mc)
	}

	// Check cache
	cacheKey := fmt.Sprintf("fetch:%s:%s:%d", rawURL, extractMode, maxChars)
	if cached, ok := t.cache.get(cacheKey); ok {
		slog.Debug("web_fetch cache hit", "url", rawURL)
		return NewResult(cached)
	}

	// Fetch
	result, err := t.doFetch(ctx, rawURL, extractMode, maxChars, policy)
	if err != nil {
		errMsg := truncateStr(err.Error(), defaultErrorMaxChars)
		return ErrorResult(fmt.Sprintf("fetch failed: %s", errMsg))
	}

	wrapped := wrapExternalContent(result, "Web Fetch", true)
	t.cache.set(cacheKey, wrapped)
	return NewResult(wrapped)
}


func (t *WebFetchTool) doFetch(ctx context.Context, rawURL, extractMode string, maxChars int, policy string) (string, error) {
	// Smart engine selection: use browser directly for known JS-heavy sites
	if t.engineRouter != nil && t.engineRouter.Route(rawURL) == scraper.EngineBrowser && t.browserFetch != nil {
		slog.Info("web_fetch.engine_select", "url", rawURL, "engine", "browser")
		result, err := t.browserFetch(ctx, rawURL)
		if err == nil && result != "" { return result, nil }
		// Fall through to HTTP if browser fails
	}
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	// 5-layer anti-bot: TLS fingerprint + HTTP/2 + header order + browser stealth + jitter
	// Uses req library (ImpersonateChrome) for layers 1-3, go-rod/stealth for layer 4
	// 5-layer anti-bot fetch
	ab := scraper.NewAntiBot()
	abResult, abErr := ab.Fetch(ctx, rawURL)
	if abErr == nil && abResult.StatusCode == 200 && len(abResult.HTML) > 0 {
		// Convert HTML to readable text using web_fetch's own converter
		text := htmlToTextDOM(abResult.HTML)
		if text == "" {
			text = htmlToMarkdownDOM(abResult.HTML)
		}
		if text != "" {
			return text, nil
		}
		// If converter fails, return cleaned text from AntiBot
		if abResult.Text != "" {
			return abResult.Text, nil
		}
	}
	// Fallback to standard fetch
	profile := scraper.GenerateProfile()
	req.Header.Set("User-Agent", profile.UserAgent)
	for k, v := range profile.Headers {
		req.Header.Set(k, v)
	}
	client := scraper.NewStealthClient(scraper.ProfileRandom, time.Duration(fetchTimeoutSeconds)*time.Second)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Auto-fallback chain on 403/429/bot-protection
	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		slog.Info("web_fetch.blocked", "url", rawURL, "status", resp.StatusCode)

		// Try 1: Headless browser
		if t.browserFetch != nil {
			if result, err := t.browserFetch(ctx, rawURL); err == nil && result != "" {
				return result, nil
			}
		}

		// Try 2: Google cache
		cacheURL := "https://webcache.googleusercontent.com/search?q=cache:" + rawURL
		cacheReq, _ := http.NewRequestWithContext(ctx, "GET", cacheURL, nil)
		cacheReq.Header.Set("User-Agent", fetchUserAgent)
		if cacheResp, err := client.Do(cacheReq); err == nil {
			defer cacheResp.Body.Close()
			if cacheResp.StatusCode == 200 {
				cacheBody, _ := io.ReadAll(io.LimitReader(cacheResp.Body, 512*1024))
				text := htmlToTextDOM(string(cacheBody))
				if text != "" {
					slog.Info("web_fetch.google_cache_success", "url", rawURL)
					return "[Cached version]\n" + text, nil
				}
			}
		}

		if t.engineRouter != nil { t.engineRouter.MarkAs403(rawURL) }
		return "", fmt.Errorf("HTTP %d: site blocks automated access (tried direct, browser, cache)", resp.StatusCode)
	}

	readLimit := int64(max(maxChars*10, 512*1024))
	body, err := io.ReadAll(io.LimitReader(resp.Body, readLimit))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	finalURL := resp.Request.URL.String()

	var text, extractor string

	switch {
	case strings.Contains(contentType, "application/json"):
		text, extractor = extractJSON(body)

	case strings.Contains(contentType, "text/markdown"):
		text = string(body)
		extractor = "markdown"
		if extractMode == "text" {
			text = markdownToPlainText(text)
		}

	case strings.Contains(contentType, "text/html"),
		strings.Contains(contentType, "application/xhtml"):
		if extractMode == "markdown" {
			text = htmlToMarkdownDOM(string(body))
			extractor = "html-to-markdown"
		} else {
			text = htmlToTextDOM(string(body))
			extractor = "html-to-text"
		}
		if text == "" && len(body) > 0 {
			text = "[No content extracted. The page may require JavaScript to render, " +
				"or returned a bot-protection challenge. Try using browser automation instead.]"
		}

	default:
		text = string(body)
		extractor = "raw"
	}

	// Build result
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("URL: %s\n", finalURL))
	if finalURL != rawURL {
		sb.WriteString(fmt.Sprintf("Redirected from: %s\n", rawURL))
	}
	sb.WriteString(fmt.Sprintf("Status: %d\n", resp.StatusCode))
	sb.WriteString(fmt.Sprintf("Extractor: %s\n", extractor))

	if len(text) > maxChars {
		workspace := WorkspaceFromCtx(ctx)
		tmpPath, writeErr := writeWebFetchTempFile(workspace, text, rawURL)
		if writeErr != nil {
			text = text[:maxChars]
			sb.WriteString(fmt.Sprintf("Truncated: true (limit: %d chars)\n", maxChars))
		} else {
			sb.WriteString(fmt.Sprintf("Content-Length: %d chars (exceeds %d char limit)\n", len(text), maxChars))
			sb.WriteString(fmt.Sprintf("Full-Content-File: %s\n", tmpPath))
			text = text[:maxChars] + fmt.Sprintf("\n\n[Content truncated. Full content (%d chars) saved to: %s]", len(text), tmpPath)
		}
	}

	sb.WriteString(fmt.Sprintf("Length: %d\n\n", len(text)))
	sb.WriteString(text)

	return sb.String(), nil
}

// Domain helpers

func matchDomainList(hostname string, patterns []string) bool {
	hostname = strings.ToLower(hostname)
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == hostname {
			return true
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[1:]
			if strings.HasSuffix(hostname, suffix) && hostname != suffix[1:] {
				return true
			}
		}
	}
	return false
}

func (t *WebFetchTool) isDomainAllowed(hostname string) bool {
	t.mu.RLock()
	domains := t.allowedDomains
	t.mu.RUnlock()
	return matchDomainList(hostname, domains)
}

func (t *WebFetchTool) isDomainBlocked(hostname string) bool {
	t.mu.RLock()
	domains := t.blockedDomains
	t.mu.RUnlock()
	return matchDomainList(hostname, domains)
}

// JSON extraction

func extractJSON(body []byte) (string, string) {
	var data any
	if err := json.Unmarshal(body, &data); err == nil {
		formatted, _ := json.MarshalIndent(data, "", "  ")
		return string(formatted), "json"
	}
	return string(body), "raw"
}

// Temp file for large content

func writeWebFetchTempFile(workspace, content, sourceURL string) (string, error) {
	var randBytes [8]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return "", fmt.Errorf("generate random name: %w", err)
	}
	filename := fmt.Sprintf("web-fetch-%s.txt", hex.EncodeToString(randBytes[:]))

	dir := os.TempDir()
	if workspace != "" {
		dir = filepath.Join(workspace, "web-fetch")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create web-fetch dir: %w", err)
	}
	outPath := filepath.Join(dir, filename)

	sanitized := sanitizeMarkers(content)
	if err := os.WriteFile(outPath, []byte(sanitized), 0600); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	slog.Info("web_fetch: content saved to file", "path", outPath, "chars", len(sanitized))
	return outPath, nil
}

// Cache

type webFetchCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	value   string
	expires time.Time
}

func newWebFetchCache(maxSize int, ttl time.Duration) *webFetchCache {
	return &webFetchCache{
		entries: make(map[string]cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *webFetchCache) get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		return "", false
	}
	return e.value, true
}

func (c *webFetchCache) set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		// Evict oldest
		var oldest string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldest == "" || v.expires.Before(oldestTime) {
				oldest = k
				oldestTime = v.expires
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[key] = cacheEntry{value: value, expires: time.Now().Add(c.ttl)}
}

// Helpers

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func sanitizeMarkers(content string) string {
	// Strip potential prompt injection markers
	content = strings.ReplaceAll(content, "<|im_start|>", "")
	content = strings.ReplaceAll(content, "<|im_end|>", "")
	content = strings.ReplaceAll(content, "<|endoftext|>", "")
	return content
}

func wrapExternalContent(content, source string, _ bool) string {
	return fmt.Sprintf("<external_content source=%q>\n%s\n</external_content>", source, content)
}

func NewResult(content string) *Result {
	return &Result{ForLLM: content, ForUser: content}
}
