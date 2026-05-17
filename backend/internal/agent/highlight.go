// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import "strings"

// ExtractHighlight pulls the most interesting 1-line hook from a response.
// Used for toast notifications — the preview should have signal, not just "task done".
func ExtractHighlight(content string, maxLen int) string {
	if content == "" { return "" }
	if maxLen == 0 { maxLen = 120 }

	// Skip markdown headers, blank lines, greetings
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" { continue }
		if strings.HasPrefix(trimmed, "#") { continue }
		if strings.HasPrefix(trimmed, "---") { continue }
		if strings.HasPrefix(trimmed, "```") { continue }
		if strings.HasPrefix(trimmed, "- **") {
			// Bullet with bold — often the best highlight
			trimmed = strings.TrimPrefix(trimmed, "- ")
			trimmed = strings.ReplaceAll(trimmed, "**", "")
			if len(trimmed) > maxLen { trimmed = trimmed[:maxLen] + "…" }
			return trimmed
		}
		// Skip generic openers
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "here") || strings.HasPrefix(lower, "sure") ||
			strings.HasPrefix(lower, "got it") || strings.HasPrefix(lower, "certainly") ||
			strings.HasPrefix(lower, "i'll") || strings.HasPrefix(lower, "let me") {
			continue
		}
		// Use first substantive line
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		if len(trimmed) > maxLen { trimmed = trimmed[:maxLen] + "…" }
		return trimmed
	}
	// Fallback: first 120 chars
	clean := strings.ReplaceAll(content, "\n", " ")
	clean = strings.ReplaceAll(clean, "**", "")
	if len(clean) > maxLen { clean = clean[:maxLen] + "…" }
	return strings.TrimSpace(clean)
}
