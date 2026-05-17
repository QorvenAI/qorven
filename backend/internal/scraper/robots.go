// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scraper

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RobotsChecker fetches and caches robots.txt rules per domain.
// Respects Crawl-delay and User-agent directives.
// This closes the ethical gap — Qorven scraper now respects site policies.
type RobotsChecker struct {
	mu     sync.RWMutex
	cache  map[string]*robotsRules
	client *http.Client
	ttl    time.Duration
}

type robotsRules struct {
	disallow   []string
	crawlDelay time.Duration
	fetchedAt  time.Time
}

// NewRobotsChecker creates a checker with a 1-hour TTL.
func NewRobotsChecker(client *http.Client) *RobotsChecker {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &RobotsChecker{
		cache:  make(map[string]*robotsRules),
		client: client,
		ttl:    1 * time.Hour,
	}
}

// IsAllowed checks if a URL is allowed by the site's robots.txt.
// Returns (allowed, crawlDelay).
func (rc *RobotsChecker) IsAllowed(ctx context.Context, rawURL string) (bool, time.Duration) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true, 0
	}
	domain := u.Host
	rules := rc.getRules(ctx, domain, u.Scheme)
	if rules == nil {
		return true, 0
	}

	path := u.Path
	if path == "" {
		path = "/"
	}

	for _, disallowed := range rules.disallow {
		if strings.HasPrefix(path, disallowed) {
			slog.Debug("robots.disallowed", "url", rawURL, "rule", disallowed)
			return false, rules.crawlDelay
		}
	}
	return true, rules.crawlDelay
}

func (rc *RobotsChecker) getRules(ctx context.Context, domain, scheme string) *robotsRules {
	rc.mu.RLock()
	if rules, ok := rc.cache[domain]; ok && time.Since(rules.fetchedAt) < rc.ttl {
		rc.mu.RUnlock()
		return rules
	}
	rc.mu.RUnlock()

	rules := rc.fetchRobots(ctx, domain, scheme)
	rc.mu.Lock()
	rc.cache[domain] = rules
	rc.mu.Unlock()
	return rules
}

func (rc *RobotsChecker) fetchRobots(ctx context.Context, domain, scheme string) *robotsRules {
	if scheme == "" {
		scheme = "https"
	}
	robotsURL := scheme + "://" + domain + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Qorven/1.0")

	resp, err := rc.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil
	}

	return parseRobots(string(body))
}

func parseRobots(content string) *robotsRules {
	rules := &robotsRules{fetchedAt: time.Now()}
	inOurSection := false

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "user-agent":
			inOurSection = value == "*" || strings.Contains(strings.ToLower(value), "qorven")
		case "disallow":
			if inOurSection && value != "" {
				rules.disallow = append(rules.disallow, value)
			}
		case "crawl-delay":
			if inOurSection {
				if d, err := time.ParseDuration(value + "s"); err == nil {
					rules.crawlDelay = d
				}
			}
		}
	}
	return rules
}
