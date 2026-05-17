// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"strings"
)


func normalizeLineEndings(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func extractFrontmatter(content string) string {
	match := frontmatterRe.FindStringSubmatch(normalizeLineEndings(content))
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func stripFrontmatter(content string) string {
	return frontmatterRe.ReplaceAllString(normalizeLineEndings(content), "")
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// parseSimpleYAML parses basic YAML: key: value pairs and block scalars.
func parseSimpleYAML(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")
	var currentKey string
	var blockLines []string
	var inBlock bool

	flushBlock := func() {
		if currentKey != "" && len(blockLines) > 0 {
			result[currentKey] = strings.Join(blockLines, " ")
		}
		currentKey = ""
		blockLines = nil
		inBlock = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if inBlock {
			if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
				blockLines = append(blockLines, strings.TrimSpace(line))
				continue
			}
			flushBlock()
		}
		if idx := strings.Index(trimmed, ":"); idx > 0 {
			key := strings.TrimSpace(trimmed[:idx])
			val := strings.TrimSpace(trimmed[idx+1:])
			if val == "|" || val == ">" || val == "" {
				currentKey = key
				inBlock = true
				continue
			}
			// Strip quotes
			val = strings.Trim(val, "\"'")
			// Handle list items
			if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
				val = strings.Trim(val, "[]")
			}
			result[key] = val
		}
	}
	flushBlock()
	return result
}
