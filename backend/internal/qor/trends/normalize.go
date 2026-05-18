// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"regexp"
	"strings"
	"time"
)

// normalize.go — Cross-platform normalization and date filtering.
// Rewritten from last30days normalize.py (442 lines).

// FilterByDateRange keeps only items within the requested time window.
func FilterByDateRange(items []SourceItem, from, to time.Time, requireDate bool) []SourceItem {
	var filtered []SourceItem
	for _, item := range items {
		if item.PublishedAt == nil {
			if !requireDate { filtered = append(filtered, item) }
			continue
		}
		t := *item.PublishedAt
		if (from.IsZero() || !t.Before(from)) && (to.IsZero() || !t.After(to)) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// NormalizeItems applies cross-platform normalization to all items.
func NormalizeItems(items []SourceItem) []SourceItem {
	for i := range items {
		items[i].Title = normalizeTitle(items[i].Title)
		items[i].Body = normalizeBody(items[i].Body)
		items[i].URL = normalizeItemURL(items[i].URL)
		if items[i].DateConfidence == "" { items[i].DateConfidence = "low" }
	}
	return items
}

var (
	htmlTagRe     = regexp.MustCompile(`<[^>]+>`)
	multiSpaceRe  = regexp.MustCompile(`\s{2,}`)
	emojiPrefixRe = regexp.MustCompile(`^[\x{1F300}-\x{1F9FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}\s]+`)
)

func normalizeTitle(title string) string {
	title = htmlTagRe.ReplaceAllString(title, "")
	title = strings.ReplaceAll(title, "&amp;", "&")
	title = strings.ReplaceAll(title, "&lt;", "<")
	title = strings.ReplaceAll(title, "&gt;", ">")
	title = strings.ReplaceAll(title, "&#39;", "'")
	title = strings.ReplaceAll(title, "&quot;", "\"")
	title = multiSpaceRe.ReplaceAllString(title, " ")
	return strings.TrimSpace(title)
}

func normalizeBody(body string) string {
	body = htmlTagRe.ReplaceAllString(body, "")
	body = strings.ReplaceAll(body, "&amp;", "&")
	body = strings.ReplaceAll(body, "&lt;", "<")
	body = strings.ReplaceAll(body, "&gt;", ">")
	body = multiSpaceRe.ReplaceAllString(body, " ")
	return strings.TrimSpace(body)
}

func normalizeItemURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "/")
	// Remove tracking parameters
	if idx := strings.Index(u, "?utm_"); idx > 0 { u = u[:idx] }
	return u
}

// NormalizeRedditContainer standardizes subreddit names.
func NormalizeRedditContainer(container string) string {
	container = strings.TrimSpace(container)
	if !strings.HasPrefix(container, "r/") && !strings.HasPrefix(container, "/r/") {
		container = "r/" + container
	}
	return strings.TrimPrefix(container, "/")
}

// ExtractDateFromText tries to parse a date from text content.
func ExtractDateFromText(text string) (*time.Time, string) {
	patterns := []struct {
		re         string
		layout     string
		confidence string
	}{
		{`\b(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)\b`, time.RFC3339, "high"},
		{`\b(\d{4}-\d{2}-\d{2})\b`, "2006-01-02", "med"},
		{`\b(\w+ \d{1,2}, \d{4})\b`, "January 2, 2006", "med"},
		{`\b(\d{1,2} \w+ \d{4})\b`, "2 January 2006", "med"},
	}
	for _, p := range patterns {
		re := regexp.MustCompile(p.re)
		if m := re.FindStringSubmatch(text); len(m) > 1 {
			if t, err := time.Parse(p.layout, m[1]); err == nil {
				return &t, p.confidence
			}
		}
	}
	return nil, "low"
}

// MergeEngagement combines engagement maps, summing overlapping keys.
func MergeEngagement(a, b map[string]float64) map[string]float64 {
	merged := make(map[string]float64)
	for k, v := range a { merged[k] = v }
	for k, v := range b { merged[k] += v }
	return merged
}
