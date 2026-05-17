// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scraper

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Hard scraper tests — real HTML processing, proxy rotation, stealth verification.

func TestHard_HTMLToMarkdown_ComplexPage(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head><title>Qorven Documentation</title></head>
<body>
<nav><a href="/">Home</a> | <a href="/docs">Docs</a></nav>
<main>
<h1>Getting Started with Qorven</h1>
<p>Qorven is a <strong>multi-agent AI workspace</strong> that runs as a single binary.</p>
<h2>Installation</h2>
<pre><code>curl -fsSL https://get.qorven.io | sh
qorven start</code></pre>
<h2>Features</h2>
<ul>
<li>52+ tools for agent automation</li>
<li>8 messaging channels (Telegram, Discord, Slack...)</li>
<li>6 LLM providers with failover</li>
<li>Knowledge graph with BFS traversal</li>
</ul>
<h2>Configuration</h2>
<table>
<tr><th>Setting</th><th>Default</th><th>Description</th></tr>
<tr><td>port</td><td>4200</td><td>Gateway port</td></tr>
<tr><td>model</td><td>gpt-4o-mini</td><td>Default LLM model</td></tr>
</table>
<footer><p>&copy; 2026 Qorven</p></footer>
</main>
<script>console.log('tracking');</script>
<style>.hidden{display:none}</style>
</body>
</html>`

	md := HTMLToMarkdown(html)
	if md == "" { t.Fatal("empty markdown") }

	// Content preserved
	if !strings.Contains(md, "Getting Started") { t.Error("missing title") }
	if !strings.Contains(md, "multi-agent") { t.Error("missing description") }
	if !strings.Contains(md, "52+") { t.Error("missing features") }
	if !strings.Contains(md, "qorven start") { t.Error("missing code") }

	// Script/style removed
	if strings.Contains(md, "tracking") { t.Error("script not removed") }
	if strings.Contains(md, "display:none") { t.Error("style not removed") }

	t.Logf("complex page: %d HTML → %d MD chars", len(html), len(md))
}

func TestHard_HTMLToMarkdown_MalformedHTML(t *testing.T) {
	malformed := []string{
		"<p>Unclosed paragraph",
		"<div><span>Nested unclosed",
		"<b>Bold <i>italic</b> broken nesting</i>",
		"<script>alert('xss')</script><p>After script</p>",
		"<table><tr><td>No closing tags",
		"&amp; &lt; &gt; &quot; entities",
		"<p style='color:red' onclick='alert(1)'>Styled</p>",
	}

	for i, html := range malformed {
		md := HTMLToMarkdown(html)
		if md == "" && len(html) > 10 { t.Logf("malformed %d: empty output", i) }
		// Should never panic
	}
	t.Log("malformed HTML: all handled without panic ✓")
}

func TestHard_Scraper_ProxyRotation_Fairness(t *testing.T) {
	proxies := []string{
		"http://proxy1:8080", "http://proxy2:8080",
		"http://proxy3:8080", "http://proxy4:8080",
	}
	pr := NewProxyRotator(proxies)

	counts := map[string]int{}
	for i := 0; i < 100; i++ {
		p := pr.Next()
		if p != nil { counts[p.Host]++ }
	}

	// Each proxy should be used roughly equally (25 ± 10)
	for host, count := range counts {
		if count < 15 || count > 35 { t.Errorf("proxy %s used %d times (expected ~25)", host, count) }
	}
	t.Logf("proxy fairness: %v", counts)
}

func TestHard_Scraper_RealFetch_WithStealth(t *testing.T) {
	if testing.Short() { t.Skip("skip real HTTP") }

	s := New()
	s.SetStealth()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	page, err := s.Fetch(ctx, "https://httpbin.org/headers")
	if err != nil { t.Skipf("httpbin: %v", err) }

	text := page.Text()
	// Stealth mode should set a browser-like User-Agent
	if strings.Contains(text, "Go-http-client") { t.Error("stealth failed: default Go UA detected") }
	t.Logf("stealth fetch: %d chars, UA hidden ✓", len(text))
}

func TestHard_Scraper_CSSSelector(t *testing.T) {
	if testing.Short() { t.Skip("skip real HTTP") }

	s := New()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	page, err := s.Fetch(ctx, "https://httpbin.org/html")
	if err != nil { t.Skipf("httpbin: %v", err) }

	// Test CSS selectors
	title := page.Title()
	if title == "" { t.Log("no title") }

	links := page.Links()
	t.Logf("page: title=%q, links=%d", title, len(links))

	// Select specific elements
	elements := page.Select("p")
	if len(elements) == 0 { t.Log("no <p> elements") }
	for i, el := range elements {
		if i >= 3 { break }
		t.Logf("  p[%d]: %q...", i, el.Text()[:min9(len(el.Text()), 50)])
	}
}

func min9(a, b int) int { if a < b { return a }; return b }
