// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package slack

import (
	"regexp"
	"strings"
)

// Full Slack mrkdwn formatting engine.

var (
	slackCodeBlockRe = regexp.MustCompile("(?s)```(\\w*)\\n?(.*?)```")
	slackInlineCodeRe = regexp.MustCompile("`([^`]+)`")
	slackTableRowRe  = regexp.MustCompile(`^\|(.+)\|$`)
	slackTableSepRe  = regexp.MustCompile(`^\|[-:|]+\|$`)
)

// markdownToSlackMrkdwn converts standard markdown to Slack's mrkdwn format.
// Slack uses: *bold*, _italic_, ~strike~, `code`, ```code block```
// Key differences from standard markdown:
//   - Bold: ** → * (single asterisk)
//   - Links: [text](url) → <url|text>
//   - No HTML tags
//   - Tables must be rendered as code blocks
func markdownToSlackMrkdwn(md string) string {
	// Step 1: Extract and protect code blocks
	var codeBlocks []string
	protected := slackCodeBlockRe.ReplaceAllStringFunc(md, func(match string) string {
		idx := len(codeBlocks)
		codeBlocks = append(codeBlocks, match) // keep as-is (Slack supports ```)
		return placeholder("CB", idx)
	})

	// Step 2: Extract and protect inline code
	var inlineCodes []string
	protected = slackInlineCodeRe.ReplaceAllStringFunc(protected, func(match string) string {
		idx := len(inlineCodes)
		inlineCodes = append(inlineCodes, match) // keep as-is
		return placeholder("IC", idx)
	})

	// Step 3: Convert tables to code blocks
	protected = convertSlackTables(protected)

	// Step 4: Convert formatting
	// Bold: **text** → *text*
	protected = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(protected, "*$1*")
	// Strikethrough: ~~text~~ → ~text~
	protected = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(protected, "~$1~")
	// Links: [text](url) → <url|text>
	protected = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(protected, "<$2|$1>")
	// Headers: # text → *text* (bold, since Slack has no headers)
	protected = regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`).ReplaceAllString(protected, "*$1*")

	// Step 5: Escape HTML entities (Slack renders them literally)
	protected = escapeSlackEntities(protected)

	// Step 6: Restore code blocks and inline codes
	for i, block := range codeBlocks {
		protected = strings.Replace(protected, placeholder("CB", i), block, 1)
	}
	for i, code := range inlineCodes {
		protected = strings.Replace(protected, placeholder("IC", i), code, 1)
	}

	return protected
}

func convertSlackTables(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) && slackTableRowRe.MatchString(lines[i]) && slackTableSepRe.MatchString(lines[i+1]) {
			var tableLines []string
			tableLines = append(tableLines, lines[i])
			j := i + 2
			for j < len(lines) && slackTableRowRe.MatchString(lines[j]) {
				tableLines = append(tableLines, lines[j])
				j++
			}
			result = append(result, "```")
			result = append(result, tableLines...)
			result = append(result, "```")
			i = j
			continue
		}
		result = append(result, lines[i])
		i++
	}
	return strings.Join(result, "\n")
}

func escapeSlackEntities(text string) string {
	// Slack auto-links URLs and renders &, <, > — escape them outside code
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	// But restore our Slack link format <url|text>
	text = strings.ReplaceAll(text, "&lt;http", "<http")
	text = regexp.MustCompile(`&gt;(\|[^>]+)?`).ReplaceAllStringFunc(text, func(m string) string {
		return strings.Replace(m, "&gt;", ">", 1)
	})
	return text
}

func placeholder(prefix string, idx int) string {
	return "\x00" + prefix + "_" + string(rune('0'+idx)) + "\x00"
}

// stripSlackFormatting removes all Slack mrkdwn formatting
func stripSlackFormatting(text string) string {
	text = strings.ReplaceAll(text, "```", "")
	text = strings.ReplaceAll(text, "`", "")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`~([^~]+)~`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`_([^_]+)_`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`<([^|>]+)\|([^>]+)>`).ReplaceAllString(text, "$2")
	return text
}

// htmlTagsToMarkdown converts HTML tags back to markdown (for content from web sources)
func htmlTagsToMarkdown(text string) string {
	text = regexp.MustCompile(`<b>(.*?)</b>`).ReplaceAllString(text, "**$1**")
	text = regexp.MustCompile(`<strong>(.*?)</strong>`).ReplaceAllString(text, "**$1**")
	text = regexp.MustCompile(`<i>(.*?)</i>`).ReplaceAllString(text, "_$1_")
	text = regexp.MustCompile(`<em>(.*?)</em>`).ReplaceAllString(text, "_$1_")
	text = regexp.MustCompile(`<code>(.*?)</code>`).ReplaceAllString(text, "`$1`")
	text = regexp.MustCompile(`<a href="([^"]+)">(.*?)</a>`).ReplaceAllString(text, "[$2]($1)")
	text = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "") // strip remaining tags
	return text
}

// extractSlackTokens extracts Slack-specific tokens (<@U123>, <#C123|channel>, <https://url>)
func extractSlackTokens(text string) ([]string, string) {
	re := regexp.MustCompile(`<([^>]+)>`)
	var tokens []string
	cleaned := re.ReplaceAllStringFunc(text, func(match string) string {
		inner := match[1 : len(match)-1]
		tokens = append(tokens, inner)
		// Convert Slack tokens to readable text
		if strings.HasPrefix(inner, "@U") { return "@user" }
		if strings.HasPrefix(inner, "#C") {
			parts := strings.SplitN(inner, "|", 2)
			if len(parts) == 2 { return "#" + parts[1] }
			return "#channel"
		}
		if strings.Contains(inner, "|") {
			parts := strings.SplitN(inner, "|", 2)
			return parts[1] // display text
		}
		return inner // URL
	})
	return tokens, cleaned
}
