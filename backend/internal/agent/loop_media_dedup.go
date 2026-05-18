// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"path/filepath"
	"strings"
)

// DeduplicateMediaSuffix filters ContentSuffix lines, removing any whose file
// basename already appears in the main content. This prevents duplicate file
// references when the agent's own text already includes the same media.
//
// ContentSuffix format (from mediaToMarkdown):
//
//	\n\n![image](/v1/files/path/to/file.png)
//	[filename.md](/v1/files/path/to/filename.md)
func DeduplicateMediaSuffix(content, suffix string) string {
	lines := strings.Split(suffix, "\n")
	var kept []string
	for _, line := range lines {
		if line == "" {
			kept = append(kept, line)
			continue
		}
		base := extractLinkBasename(line)
		if base != "" && containsMediaRef(content, base) {
			continue // already referenced in content
		}
		kept = append(kept, line)
	}
	result := strings.Join(kept, "\n")
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return result
}

// containsMediaRef checks if content contains a URL reference to a file by basename.
func containsMediaRef(content, basename string) bool {
	return strings.Contains(content, "/"+basename+")") ||
		strings.Contains(content, "/"+basename+"?") ||
		strings.Contains(content, "/"+basename+"\n") ||
		strings.HasSuffix(content, "/"+basename)
}

// extractLinkBasename extracts the filename from a markdown link or image line.
func extractLinkBasename(line string) string {
	idx := strings.Index(line, "](")
	if idx < 0 {
		return ""
	}
	urlStart := idx + 2
	urlEnd := strings.LastIndex(line, ")")
	if urlEnd <= urlStart {
		return ""
	}
	url := line[urlStart:urlEnd]
	if qIdx := strings.Index(url, "?"); qIdx > 0 {
		url = url[:qIdx]
	}
	return filepath.Base(url)
}

// DeduplicateMediaResults removes duplicate media results by path.
func DeduplicateMediaResults(media []MediaResult) []MediaResult {
	if len(media) <= 1 {
		return media
	}
	seen := make(map[string]bool, len(media))
	result := make([]MediaResult, 0, len(media))
	for _, m := range media {
		if seen[m.Path] {
			continue
		}
		seen[m.Path] = true
		result = append(result, m)
	}
	return result
}
