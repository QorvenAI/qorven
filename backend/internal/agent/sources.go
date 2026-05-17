// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"regexp"
	"strings"
)

// Source represents a cited source in a response.
type Source struct {
	Index string `json:"index"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

var sourcePattern = regexp.MustCompile(`\[(\d+)\]\s*\[?([^\]\n]+)\]?\(?(https?://[^\s\)]+)\)?`)
var simpleSourcePattern = regexp.MustCompile(`\[(\d+)\]\s*(https?://[^\s]+)`)

// ExtractSources parses [N] URL patterns from response text.
func ExtractSources(content string) []Source {
	var sources []Source
	seen := make(map[string]bool)

	// Try markdown link format: [1] [Title](URL)
	for _, m := range sourcePattern.FindAllStringSubmatch(content, -1) {
		if len(m) >= 4 && !seen[m[3]] {
			seen[m[3]] = true
			title := strings.TrimSpace(m[2])
			title = strings.Trim(title, "[]")
			sources = append(sources, Source{Index: m[1], Title: title, URL: m[3]})
		}
	}

	// Try simple format: [1] https://...
	if len(sources) == 0 {
		for _, m := range simpleSourcePattern.FindAllStringSubmatch(content, -1) {
			if len(m) >= 3 && !seen[m[2]] {
				seen[m[2]] = true
				sources = append(sources, Source{Index: m[1], Title: domainFromURL(m[2]), URL: m[2]})
			}
		}
	}

	return sources
}

func domainFromURL(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "www.")
	if i := strings.Index(u, "/"); i > 0 { u = u[:i] }
	return u
}
