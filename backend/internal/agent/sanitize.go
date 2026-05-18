// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"log/slog"
	"regexp"
	"strings"
)

// SanitizeResponse applies the full sanitization pipeline to assistant
// response text before saving to session and sending to user.
func SanitizeResponse(content string) string {
	if content == "" {
		return content
	}

	original := content

	// 1. Strip garbled tool-call XML (DeepSeek, GLM, Minimax)
	content = stripGarbledToolXML(content)
	if content == "" {
		return ""
	}

	// 2. Strip downgraded tool call text
	content = stripDowngradedToolCallText(content)

	// 3. Strip thinking/reasoning tags
	content = stripThinkingTags(content)

	// 4. Strip <final> tags (keep content inside)
	content = stripFinalTags(content)

	// 5. Strip echoed [System Message] blocks
	content = stripEchoedSystemMessages(content)

	// 6. Collapse consecutive duplicate blocks
	content = collapseConsecutiveDuplicateBlocks(content)

	// 7. Strip MEDIA: paths from LLM output
	content = stripMediaPaths(content)

	// 8. Strip leading blank lines
	content = stripLeadingBlankLines(content)

	// 9. Config leak detection
	content = StripConfigLeak(content, "")

	content = strings.TrimSpace(content)

	if content != original {
		slog.Debug("sanitized assistant content",
			"original_len", len(original),
			"cleaned_len", len(content),
		)
	}

	return content
}

// --- Garbled tool-call XML ---

var garbledToolXMLPattern = regexp.MustCompile(
	`(?s)</?(?:function_calls?|functioninvoke|invoke|invfunction_calls|tool_call|tool_use|parameter|minimax:tool_call|web_search|call_results?|result|object|value|item|key|type)[^>]*>`,
)

var garbledToolXMLIndicators = []string{
	"invfunction_calls", "functioninvoke", "<parameter name=",
	"</parameter", "<function_call", "<tool_call", "<tool_use", "<minimax:tool_call",
	"<web_search>", "<call_results>", "<result>", "</call_results>",
}

func stripGarbledToolXML(content string) string {
	hasIndicator := false
	lower := strings.ToLower(content)
	for _, ind := range garbledToolXMLIndicators {
		if strings.Contains(lower, strings.ToLower(ind)) {
			hasIndicator = true
			break
		}
	}
	if !hasIndicator {
		return content
	}

	cleaned := garbledToolXMLPattern.ReplaceAllString(content, "")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		slog.Warn("stripped entire response as garbled tool XML", "original_len", len(content))
		return ""
	}
	return cleaned
}

// --- Downgraded tool call text ---

func stripDowngradedToolCallText(content string) string {
	if !strings.Contains(content, "[Tool Call:") &&
		!strings.Contains(content, "[Tool Result") &&
		!strings.Contains(content, "[Historical context:") {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	skipping := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[Tool Call:") ||
			strings.HasPrefix(trimmed, "[Tool Result") ||
			strings.HasPrefix(trimmed, "[Historical context:") {
			skipping = true
			continue
		}

		if skipping {
			if trimmed == "" || strings.HasPrefix(trimmed, "Arguments:") ||
				strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "}") {
				continue
			}
			skipping = false
		}

		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// --- Thinking/reasoning tags ---

var thinkingTagPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<think>.*?</think>`),
	regexp.MustCompile(`(?is)<thinking>.*?</thinking>`),
	regexp.MustCompile(`(?is)<thought>.*?</thought>`),
	regexp.MustCompile(`(?is)<antThinking>.*?</antThinking>`),
	regexp.MustCompile(`(?is)<antthinking>.*?</antthinking>`),
}

func stripThinkingTags(content string) string {
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "<think") && !strings.Contains(lower, "<thought") &&
		!strings.Contains(lower, "<antthinking") {
		return content
	}
	result := content
	for _, pat := range thinkingTagPatterns {
		result = pat.ReplaceAllString(result, "")
	}
	return strings.TrimSpace(result)
}

// --- <final> tags ---

var finalTagPattern = regexp.MustCompile(`(?i)<\s*/?\s*final\s*>`)

func stripFinalTags(content string) string {
	if !strings.Contains(strings.ToLower(content), "final") {
		return content
	}
	return finalTagPattern.ReplaceAllString(content, "")
}

// --- Echoed [System Message] ---

func stripEchoedSystemMessages(content string) string {
	if !strings.Contains(content, "[System Message]") {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	skipping := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[System Message]") {
			skipping = true
			continue
		}
		if skipping {
			if strings.TrimSpace(line) == "" {
				skipping = false
				continue
			}
			continue
		}
		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// --- Collapse consecutive duplicate blocks ---

func collapseConsecutiveDuplicateBlocks(content string) string {
	blocks := strings.Split(content, "\n\n")
	if len(blocks) <= 1 {
		return content
	}

	var result []string
	for i, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			continue
		}
		if i > 0 && len(result) > 0 && trimmed == strings.TrimSpace(result[len(result)-1]) {
			continue
		}
		result = append(result, block)
	}

	return strings.Join(result, "\n\n")
}

// --- Strip MEDIA: paths ---

var mediaPathPattern = regexp.MustCompile(`MEDIA:\S+`)

func stripMediaPaths(content string) string {
	if !strings.Contains(content, "MEDIA:") {
		return content
	}
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[[audio_as_voice]]") {
			continue
		}
		if mediaPathPattern.MatchString(trimmed) {
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// --- Strip leading blank lines ---

var leadingBlankLinesPattern = regexp.MustCompile(`^(?:[ \t]*\r?\n)+`)

func stripLeadingBlankLines(content string) string {
	return leadingBlankLinesPattern.ReplaceAllString(content, "")
}

// --- Config leak detection ---

var configLeakFileNames = []string{
	"SOUL.md", "IDENTITY.md", "AGENTS.md", "BOOTSTRAP.md",
	"internal_config", "system prompt",
}

var fencedCodeBlockPattern = regexp.MustCompile("(?s)```[^`]*```")
var inlineCodePattern = regexp.MustCompile("`[^`\n]+`")

func stripMarkdownCode(s string) string {
	s = fencedCodeBlockPattern.ReplaceAllString(s, "")
	s = inlineCodePattern.ReplaceAllString(s, "")
	return s
}

// StripConfigLeak detects when an agent dumps its internal configuration.
func StripConfigLeak(content, agentType string) string {
	if agentType != "predefined" || content == "" {
		return content
	}

	plain := stripMarkdownCode(content)

	hits := 0
	for _, name := range configLeakFileNames {
		if strings.Contains(plain, name) {
			hits++
		}
	}
	if hits < 3 {
		return content
	}

	slog.Warn("security.config_leak_stripped", "file_hits", hits, "original_len", len(content))
	return "🔒 Security check not passed."
}

// --- NO_REPLY detection ---

// IsSilentReplyText checks if the text is a NO_REPLY token.
func IsSilentReplyText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	const token = "NO_REPLY"
	if trimmed == token {
		return true
	}
	if strings.HasPrefix(trimmed, token) {
		rest := trimmed[len(token):]
		if rest == "" || !isWordCharSanitize(rune(rest[0])) {
			return true
		}
	}
	if strings.HasSuffix(trimmed, token) {
		before := trimmed[:len(trimmed)-len(token)]
		if before == "" || !isWordCharSanitize(rune(before[len(before)-1])) {
			return true
		}
	}
	return false
}

func isWordCharSanitize(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// --- Message Directives ---

var messageDirectivePatternSanitize = regexp.MustCompile(`\[\[\w+(?::[^\]\n]+)?\]\]`)

// StripDirectives removes internal [[...]] routing tags from user-facing text.
func StripDirectives(content string) string {
	if !strings.Contains(content, "[[") {
		return content
	}
	result := messageDirectivePatternSanitize.ReplaceAllStringFunc(content, func(match string) string {
		inner := match[2 : len(match)-2]
		if strings.HasPrefix(inner, "tts") {
			return match // preserve for TTS
		}
		return ""
	})
	return strings.TrimSpace(result)
}

// --- 10. Strip fabricated sources ---

// StripFabricatedSources removes "Sources:", "References:" sections when
// no web search tools were actually used. DeepSeek halluccinates these.
func StripFabricatedSources(content string, toolsUsed []string) string {
	if content == "" {
		return content
	}
	// If web_search or web_fetch was used, keep sources
	for _, t := range toolsUsed {
		if t == "web_search" || t == "web_fetch" || t == "browser" || t == "deep_research" {
			return content
		}
	}
	// Strip Sources/References sections at the end
	lines := strings.Split(content, "\n")
	cutIdx := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		// Check for source section headers
		if strings.HasPrefix(trimmed, "**Sources") || strings.HasPrefix(trimmed, "Sources:") ||
			strings.HasPrefix(trimmed, "**References") || strings.HasPrefix(trimmed, "References:") ||
			strings.HasPrefix(trimmed, "---") {
			cutIdx = i
			continue
		}
		// Check for citation lines like [1] Wikipedia - ...
		if len(trimmed) > 3 && trimmed[0] == '[' && (trimmed[1] >= '0' && trimmed[1] <= '9') {
			cutIdx = i
			continue
		}
		// Check for - [1] or * [1] style
		if (strings.HasPrefix(trimmed, "- [") || strings.HasPrefix(trimmed, "* [")) &&
			len(trimmed) > 4 && trimmed[3] >= '0' && trimmed[3] <= '9' {
			cutIdx = i
			continue
		}
		break
	}
	if cutIdx < len(lines) {
		result := strings.TrimSpace(strings.Join(lines[:cutIdx], "\n"))
		if result != content {
			slog.Debug("stripped fabricated sources", "original_len", len(content), "cleaned_len", len(result))
		}
		return result
	}
	return content
}
