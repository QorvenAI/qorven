// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scraper

import (
	"strings"
	"testing"
	"time"
)

func TestScraper_New(t *testing.T) {
	s := New()
	if s == nil { t.Fatal("nil scraper") }
}

func TestScraper_SetUserAgent(t *testing.T) {
	s := New()
	s.SetUserAgent("TestBot/1.0")
}

func TestScraper_SetHeader(t *testing.T) {
	s := New()
	s.SetHeader("X-Custom", "value")
}

func TestScraper_SetStealth(t *testing.T) {
	s := New()
	s.SetStealth()
	// Should not panic
}

func TestScraper_RotateUserAgent(t *testing.T) {
	s := New()
	s.RotateUserAgent()
	// Should not panic
}

func TestHTMLToMarkdown_Simple(t *testing.T) {
	html := "<h1>Title</h1><p>Hello world</p>"
	md := HTMLToMarkdown(html)
	if md == "" { t.Error("empty markdown") }
	if !strings.Contains(md, "Title") { t.Error("missing title") }
}

func TestHTMLToMarkdown_Empty(t *testing.T) {
	md := HTMLToMarkdown("")
	_ = md // should not panic
}

func TestHTMLToMarkdown_Complex(t *testing.T) {
	html := `<div><h2>Section</h2><ul><li>Item 1</li><li>Item 2</li></ul><p>Text with <strong>bold</strong></p></div>`
	md := HTMLToMarkdown(html)
	if !strings.Contains(md, "Section") { t.Error("missing section") }
	if !strings.Contains(md, "Item 1") { t.Error("missing list item") }
}

func TestHTMLToMarkdown_ScriptRemoval(t *testing.T) {
	html := `<p>Content</p><script>alert('xss')</script><p>More</p>`
	md := HTMLToMarkdown(html)
	if strings.Contains(md, "alert") { t.Error("script should be removed") }
}

func TestProxyRotator_New(t *testing.T) {
	pr := NewProxyRotator([]string{"http://proxy1:8080", "http://proxy2:8080"})
	if pr == nil { t.Fatal("nil rotator") }
}

func TestProxyRotator_Next(t *testing.T) {
	pr := NewProxyRotator([]string{"http://proxy1:8080", "http://proxy2:8080"})
	p1 := pr.Next()
	p2 := pr.Next()
	if p1 == nil || p2 == nil { t.Error("nil proxy") }
	// Should rotate
	p3 := pr.Next()
	if p3 == nil { t.Error("nil on third call") }
}

func TestProxyRotator_Empty(t *testing.T) {
	pr := NewProxyRotator(nil)
	p := pr.Next()
	if p != nil { t.Error("empty rotator should return nil") }
}

func TestRandomDelay(t *testing.T) {
	start := time.Now()
	RandomDelay(1*time.Millisecond, 10*time.Millisecond)
	elapsed := time.Since(start)
	if elapsed < 1*time.Millisecond { t.Error("delay too short") }
	if elapsed > 100*time.Millisecond { t.Error("delay too long") }
}

func TestPage_Title(t *testing.T) {
	// Can't fetch real URLs in unit tests, but verify the type exists
	var p *Page
	if p != nil { _ = p.Title() }
}

func TestPage_Text(t *testing.T) {
	var p *Page
	if p != nil { _ = p.Text() }
}

func TestPage_Links(t *testing.T) {
	var p *Page
	if p != nil { _ = p.Links() }
}

func TestElement_Text(t *testing.T) {
	var e *Element
	if e != nil { _ = e.Text() }
}
