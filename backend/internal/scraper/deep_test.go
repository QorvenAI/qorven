// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeep_HTMLToMarkdown_RealPage(t *testing.T) {
	html := `<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body>
<h1>Welcome to Qorven</h1>
<p>Qorven is a <strong>multi-agent AI workspace</strong> platform.</p>
<h2>Features</h2>
<ul>
<li>52+ tools</li>
<li>8 messaging channels</li>
<li>6 LLM providers</li>
</ul>
<h2>Getting Started</h2>
<pre><code>qorven start
qorven agent chat --agent prime</code></pre>
<p>Visit <a href="https://qorven.io">our website</a> for more.</p>
<script>alert('xss')</script>
<style>.hidden{display:none}</style>
</body></html>`

	md := HTMLToMarkdown(html)
	if md == "" { t.Fatal("empty markdown") }

	// Content should be preserved
	if !strings.Contains(md, "Qorven") { t.Error("missing Qorven") }
	if !strings.Contains(md, "52+") { t.Error("missing features") }
	if !strings.Contains(md, "Getting Started") { t.Error("missing section") }

	// Script/style should be removed
	if strings.Contains(md, "alert") { t.Error("script not removed") }
	if strings.Contains(md, "display:none") { t.Error("style not removed") }

	t.Logf("HTML→MD: %d chars → %d chars", len(html), len(md))
}

func TestDeep_HTMLToMarkdown_EmptyAndEdge(t *testing.T) {
	cases := []string{"", "<html></html>", "<p></p>", "plain text", "<br/>"}
	for _, html := range cases {
		md := HTMLToMarkdown(html)
		_ = md // should not panic
	}
}

func TestDeep_Scraper_StealthMode(t *testing.T) {
	s := New()
	s.SetStealth()
	s.RotateUserAgent()
	// Should not panic, should configure anti-detection
}

func TestDeep_Scraper_ProxyRotation(t *testing.T) {
	proxies := []string{
		"http://proxy1.example.com:8080",
		"http://proxy2.example.com:8080",
		"http://proxy3.example.com:8080",
	}
	pr := NewProxyRotator(proxies)
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		p := pr.Next()
		if p != nil { seen[p.String()] = true }
	}
	if len(seen) < 2 { t.Errorf("should rotate: saw %d unique proxies", len(seen)) }
	t.Logf("proxy rotation: %d unique in 10 calls", len(seen))
}

func TestDeep_Scraper_RealFetch(t *testing.T) {
	if testing.Short() { t.Skip("skip real HTTP") }
	s := New()
	s.SetStealth()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	page, err := s.Fetch(ctx, "https://httpbin.org/html")
	if err != nil { t.Skipf("httpbin unavailable: %v", err) }
	if page == nil { t.Fatal("nil page") }

	title := page.Title()
	text := page.Text()
	links := page.Links()

	t.Logf("fetched: title=%q, text=%d chars, links=%d", title, len(text), len(links))
	if text == "" { t.Error("empty text") }
}
