// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// importantTailRe matches keywords in the tail of output that indicate
// the tail contains important information (errors, summaries, results).
var importantTailRe = regexp.MustCompile(`(?i)(error|exception|failed|fatal|traceback|panic|stack trace|exit code|total|summary|result|complete|finished|done)\b`)

// HasImportantTail checks if the tail of content contains important keywords.
func HasImportantTail(content string, tailSize int) bool {
	if len(content) <= tailSize {
		return importantTailRe.MatchString(content)
	}
	tail := content[len(content)-tailSize:]
	return importantTailRe.MatchString(tail)
}

// TruncateToolResult truncates a tool result to fit within maxChars.
// Preserves head and tail if the tail contains important keywords.
func TruncateToolResult(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}

	// Check if tail has important info
	tailSize := min(2000, maxChars/4)
	if HasImportantTail(content, tailSize) {
		// Keep head + tail
		headSize := maxChars - tailSize - 50 // 50 for truncation marker
		if headSize < 100 {
			headSize = 100
		}
		head := content[:headSize]
		tail := content[len(content)-tailSize:]
		return head + "\n\n[... truncated " + formatBytes(len(content)-headSize-tailSize) + " ...]\n\n" + tail
	}

	// Just truncate from end
	return content[:maxChars-30] + "\n\n[... truncated ...]"
}

// TruncateWithContext truncates content while preserving context around a pattern.
func TruncateWithContext(content string, maxChars int, pattern string) string {
	if len(content) <= maxChars {
		return content
	}

	// Find pattern location
	idx := strings.Index(content, pattern)
	if idx < 0 {
		return TruncateToolResult(content, maxChars)
	}

	// Center around pattern
	contextSize := maxChars / 2
	start := idx - contextSize
	if start < 0 {
		start = 0
	}
	end := idx + len(pattern) + contextSize
	if end > len(content) {
		end = len(content)
	}

	var result strings.Builder
	if start > 0 {
		result.WriteString("[... truncated ...]\n\n")
	}
	result.WriteString(content[start:end])
	if end < len(content) {
		result.WriteString("\n\n[... truncated ...]")
	}
	return result.String()
}

func formatBytes(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d bytes", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%d KB", n/1024)
	}
	return fmt.Sprintf("%d MB", n/(1024*1024))
}
