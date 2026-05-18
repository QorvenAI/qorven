// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Engine types for URL-based routing .
type Engine string

const (
	EngineHTTP       Engine = "http"       // Plain HTTP fetch
	EngineBrowser    Engine = "browser"    // Headless Chrome via chromedp
	EnginePDF        Engine = "pdf"        // PDF text extraction
	EngineDocument   Engine = "document"   // DOCX/XLSX/PPTX extraction
)

// EngineRouter decides which scraping engine to use for a given URL.
// Priority: explicit domain mapping > file extension > content-type > default HTTP.
type EngineRouter struct {
	mu             sync.RWMutex
	domainMappings map[string]Engine // domain → engine
	extensionMap   map[string]Engine // .pdf → pdf engine
}

// NewEngineRouter creates a router with default mappings.
func NewEngineRouter() *EngineRouter {
	r := &EngineRouter{
		domainMappings: make(map[string]Engine),
		extensionMap: map[string]Engine{
			".pdf":  EnginePDF,
			".docx": EngineDocument,
			".xlsx": EngineDocument,
			".pptx": EngineDocument,
			".doc":  EngineDocument,
			".xls":  EngineDocument,
		},
	}

	// Default browser-required domains (from blocklist + our experience)
	browserDomains := []string{
		"espncricinfo.com", "cricbuzz.com", "bloomberg.com",
		"twitter.com", "x.com", "instagram.com",
		"linkedin.com", "facebook.com", "tiktok.com",
		"reddit.com", "medium.com", "substack.com",
		"notion.so", "figma.com", "canva.com",
		"nytimes.com", "wsj.com", "ft.com",
		"amazon.com", "ebay.com", "walmart.com",
	}
	for _, d := range browserDomains {
		r.domainMappings[d] = EngineBrowser
	}

	// Load custom mappings from env (qorven pattern)
	r.loadFromEnv()

	// Load from config file if exists
	r.loadFromFile()

	return r
}

// Route returns the best engine for a URL.
func (r *EngineRouter) Route(rawURL string) Engine {
	u, err := url.Parse(rawURL)
	if err != nil {
		return EngineHTTP
	}

	// 1. Check file extension first
	ext := strings.ToLower(filepath.Ext(u.Path))
	if engine, ok := r.extensionMap[ext]; ok {
		return engine
	}

	// 2. Check domain mapping
	host := strings.TrimPrefix(u.Hostname(), "www.")
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Exact match
	if engine, ok := r.domainMappings[host]; ok {
		return engine
	}

	// Wildcard match (*.example.com)
	for pattern, engine := range r.domainMappings {
		if strings.HasPrefix(pattern, "*.") {
			base := strings.TrimPrefix(pattern, "*.")
			if host == base || strings.HasSuffix(host, "."+base) {
				return engine
			}
		}
		// Suffix match (sub.example.com matches example.com)
		if strings.HasSuffix(host, "."+pattern) {
			return engine
		}
	}

	return EngineHTTP
}

// AddDomain adds or updates a domain→engine mapping at runtime.
func (r *EngineRouter) AddDomain(domain string, engine Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domainMappings[domain] = engine
}

// MarkAs403 records that a domain returned 403, auto-routes to browser next time.
func (r *EngineRouter) MarkAs403(rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	r.AddDomain(host, EngineBrowser)
}

// loadFromEnv loads domain mappings from FORCED_ENGINE_DOMAINS env var.
// Format: JSON {"domain": "engine", "*.pattern": "engine"}
func (r *EngineRouter) loadFromEnv() {
	envVar := os.Getenv("FORCED_ENGINE_DOMAINS")
	if envVar == "" {
		return
	}
	var mappings map[string]string
	if json.Unmarshal([]byte(envVar), &mappings) == nil {
		for domain, engine := range mappings {
			r.domainMappings[domain] = Engine(engine)
		}
	}
}

// loadFromFile loads domain mappings from ~/.qorven/engine-domains.json
func (r *EngineRouter) loadFromFile() {
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".qorven", "engine-domains.json"))
	if err != nil {
		return
	}
	var mappings map[string]string
	if json.Unmarshal(data, &mappings) == nil {
		for domain, engine := range mappings {
			r.domainMappings[domain] = Engine(engine)
		}
	}
}
