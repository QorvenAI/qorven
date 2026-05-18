// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Full Telegram formatting engine.

const telegramHTMLMaxLen = 4000

// --- Markdown → Telegram HTML ---

var (
	codeBlockRe = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	inlineCodeRe = regexp.MustCompile("`([^`]+)`")
	tableRowRe  = regexp.MustCompile(`^\|(.+)\|$`)
	tableSepRe  = regexp.MustCompile(`^\|[-:|]+\|$`)
)

func markdownToTelegramHTML(md string) string {
	// Step 1: Extract and protect code blocks
	var codeBlocks []string
	protected := codeBlockRe.ReplaceAllStringFunc(md, func(match string) string {
		idx := len(codeBlocks)
		sub := codeBlockRe.FindStringSubmatch(match)
		lang, code := "", match
		if len(sub) >= 3 { lang = sub[1]; code = sub[2] }
		var block string
		if lang != "" {
			block = fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", lang, escapeHTML(code))
		} else {
			block = fmt.Sprintf("<pre><code>%s</code></pre>", escapeHTML(code))
		}
		codeBlocks = append(codeBlocks, block)
		return fmt.Sprintf("\x00CODEBLOCK_%d\x00", idx)
	})

	// Step 2: Extract and protect inline code
	var inlineCodes []string
	protected = inlineCodeRe.ReplaceAllStringFunc(protected, func(match string) string {
		idx := len(inlineCodes)
		sub := inlineCodeRe.FindStringSubmatch(match)
		if len(sub) >= 2 {
			inlineCodes = append(inlineCodes, "<code>"+escapeHTML(sub[1])+"</code>")
		} else {
			inlineCodes = append(inlineCodes, match)
		}
		return fmt.Sprintf("\x00INLINE_%d\x00", idx)
	})

	// Step 3: Escape HTML in remaining text
	protected = escapeHTML(protected)

	// Step 4: Convert markdown tables to ASCII code blocks
	protected = convertTables(protected)

	// Step 5: Convert markdown formatting
	// Bold: **text** → <b>text</b>
	protected = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(protected, "<b>$1</b>")
	// Italic: _text_ → <i>text</i> (but not inside words like some_var)
	protected = regexp.MustCompile(`(?:^|\s)_([^_]+)_(?:\s|$)`).ReplaceAllString(protected, " <i>$1</i> ")
	// Strikethrough: ~~text~~ → <s>text</s>
	protected = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(protected, "<s>$1</s>")
	// Links: [text](url) → <a href="url">text</a>
	protected = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllString(protected, `<a href="$2">$1</a>`)

	// Step 6: Restore code blocks and inline codes
	for i, block := range codeBlocks {
		protected = strings.Replace(protected, fmt.Sprintf("\x00CODEBLOCK_%d\x00", i), block, 1)
	}
	for i, code := range inlineCodes {
		protected = strings.Replace(protected, fmt.Sprintf("\x00INLINE_%d\x00", i), code, 1)
	}

	return protected
}

// --- Table Rendering (markdown tables → ASCII in <pre> blocks) ---

func convertTables(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		// Detect table start: | col1 | col2 | followed by |---|---|
		if i+1 < len(lines) && tableRowRe.MatchString(lines[i]) && tableSepRe.MatchString(lines[i+1]) {
			// Collect all table rows
			var tableLines []string
			tableLines = append(tableLines, lines[i])
			j := i + 2 // skip separator
			for j < len(lines) && tableRowRe.MatchString(lines[j]) {
				tableLines = append(tableLines, lines[j])
				j++
			}
			result = append(result, renderTableAsCode(tableLines))
			i = j
			continue
		}
		result = append(result, lines[i])
		i++
	}
	return strings.Join(result, "\n")
}

func renderTableAsCode(rows []string) string {
	if len(rows) == 0 { return "" }

	// Parse all rows into cells
	var parsed [][]string
	for _, row := range rows {
		parsed = append(parsed, parseTableRow(row))
	}

	// Calculate column widths
	maxCols := 0
	for _, cells := range parsed {
		if len(cells) > maxCols { maxCols = len(cells) }
	}
	colWidths := make([]int, maxCols)
	for _, cells := range parsed {
		for j, cell := range cells {
			w := displayWidth(cell)
			if w > colWidths[j] { colWidths[j] = w }
		}
	}

	// Render
	var sb strings.Builder
	sb.WriteString("<pre>")
	for i, cells := range parsed {
		sb.WriteString(renderRow(cells, colWidths))
		sb.WriteString("\n")
		if i == 0 {
			// Separator after header
			for j, w := range colWidths {
				if j > 0 { sb.WriteString("─┼─") }
				sb.WriteString(strings.Repeat("─", w))
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("</pre>")
	return sb.String()
}

func parseTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts { cells[i] = strings.TrimSpace(p) }
	return cells
}

func renderRow(cells []string, colWidths []int) string {
	var parts []string
	for i, cell := range cells {
		w := colWidths[i]
		pad := w - displayWidth(cell)
		if pad < 0 { pad = 0 }
		parts = append(parts, cell+strings.Repeat(" ", pad))
	}
	// Pad missing columns
	for i := len(cells); i < len(colWidths); i++ {
		parts = append(parts, strings.Repeat(" ", colWidths[i]))
	}
	return strings.Join(parts, " │ ")
}

// displayWidth returns the visual width of a string (handles CJK double-width)
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
			(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
			(r >= 0xac00 && r <= 0xd7a3) ||
			(r >= 0xf900 && r <= 0xfaff) ||
			(r >= 0xfe10 && r <= 0xfe19) ||
			(r >= 0xfe30 && r <= 0xfe6f) ||
			(r >= 0xff00 && r <= 0xff60) ||
			(r >= 0xffe0 && r <= 0xffe6)) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// --- HTML-Aware Chunking (split AFTER conversion, not before) ---

func chunkHTML(html string, maxLen int) []string {
	if len(html) <= maxLen { return []string{html} }

	var chunks []string
	remaining := html
	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			chunks = append(chunks, remaining)
			break
		}
		// Find safe split point — don't break inside HTML tags
		cut := maxLen
		// Search backwards for a safe split point
		for cut > maxLen/2 {
			// Don't split inside a tag
			if remaining[cut] == '<' { cut--; continue }
			// Check if we're inside a tag
			lastOpen := strings.LastIndex(remaining[:cut], "<")
			lastClose := strings.LastIndex(remaining[:cut], ">")
			if lastOpen > lastClose {
				// We're inside a tag — move before it
				cut = lastOpen
				continue
			}
			// Prefer splitting at newline
			if idx := strings.LastIndex(remaining[:cut], "\n"); idx > maxLen/2 {
				cut = idx + 1
				break
			}
			// Prefer splitting at space
			if idx := strings.LastIndex(remaining[:cut], " "); idx > maxLen/2 {
				cut = idx + 1
				break
			}
			break
		}
		chunk := remaining[:cut]
		remaining = remaining[cut:]

		// Close any unclosed tags in this chunk
		chunk = closeUnclosedTags(chunk)
		chunks = append(chunks, chunk)
	}
	return chunks
}

func closeUnclosedTags(html string) string {
	// Simple tag balancer for common Telegram HTML tags
	tags := []string{"b", "i", "s", "u", "code", "pre", "a"}
	for _, tag := range tags {
		opens := strings.Count(html, "<"+tag+">") + strings.Count(html, "<"+tag+" ")
		closes := strings.Count(html, "</"+tag+">")
		for opens > closes {
			html += "</" + tag + ">"
			closes++
		}
	}
	return html
}

// --- Helpers ---

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func stripAllFormatting(text string) string {
	text = codeBlockRe.ReplaceAllString(text, "$2")
	text = inlineCodeRe.ReplaceAllString(text, "$1")
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "__", "")
	text = strings.ReplaceAll(text, "~~", "")
	return text
}

// htmlToPlain strips HTML tags for plain text fallback
func htmlToPlain(html string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	plain := re.ReplaceAllString(html, "")
	plain = strings.ReplaceAll(plain, "&amp;", "&")
	plain = strings.ReplaceAll(plain, "&lt;", "<")
	plain = strings.ReplaceAll(plain, "&gt;", ">")
	return plain
}

var _ = utf8.RuneLen // ensure import used
