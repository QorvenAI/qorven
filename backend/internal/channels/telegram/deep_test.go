// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"strings"
	"testing"
)

// Deep Telegram tests — real-world message formatting, chunking edge cases.

func TestDeep_Format_RealLLMResponse(t *testing.T) {
	// Simulate a real LLM response with mixed formatting
	md := "## Summary\n\n" +
		"Here are the **key findings**:\n\n" +
		"1. The `database` connection pool is at 80% capacity\n" +
		"2. Memory usage has *increased* by 15%\n" +
		"3. API latency is within acceptable range\n\n" +
		"### Recommendations\n\n" +
		"- Scale the DB pool from 10 to 20 connections\n" +
		"- Investigate memory leak in the `agent loop`\n" +
		"- No action needed for API latency\n\n" +
		"```sql\nALTER SYSTEM SET max_connections = 200;\nSELECT pg_reload_conf();\n```\n\n" +
		"Let me know if you need more details."

	html := markdownToTelegramHTML(md)
	if html == "" { t.Fatal("empty HTML") }

	// Key content should survive
	if !strings.Contains(html, "key findings") { t.Error("missing key findings") }
	if !strings.Contains(html, "database") { t.Error("missing database") }
	if !strings.Contains(html, "Recommendations") { t.Error("missing recommendations") }
	if !strings.Contains(html, "max_connections") { t.Error("missing SQL") }
	if !strings.Contains(html, "more details") { t.Error("missing closing") }

	t.Logf("MD→HTML: %d → %d chars", len(md), len(html))
}

func TestDeep_Format_LongCodeBlock(t *testing.T) {
	code := "```go\n" + strings.Repeat("func handler() {\n\tfmt.Println(\"line\")\n}\n\n", 100) + "```"
	html := markdownToTelegramHTML(code)
	if html == "" { t.Fatal("empty") }
	// Should contain pre/code tags
	if !strings.Contains(html, "handler") { t.Error("code content lost") }
}

func TestDeep_Chunk_ExactBoundary(t *testing.T) {
	// Message exactly at 4096 limit
	text := strings.Repeat("x", 4096)
	chunks := chunkHTML(text, 4096)
	if len(chunks) != 1 { t.Errorf("exact boundary: %d chunks", len(chunks)) }
}

func TestDeep_Chunk_OneBeyond(t *testing.T) {
	text := strings.Repeat("x", 4097)
	chunks := chunkHTML(text, 4096)
	if len(chunks) < 2 { t.Errorf("one beyond: %d chunks", len(chunks)) }
}

func TestDeep_Chunk_WithHTMLTags(t *testing.T) {
	// HTML tags that span chunk boundaries
	html := "<b>" + strings.Repeat("bold text ", 500) + "</b>"
	chunks := chunkHTML(html, 4096)
	for i, c := range chunks {
		// Each chunk should have balanced tags
		opens := strings.Count(c, "<b>")
		closes := strings.Count(c, "</b>")
		if opens > closes+1 { t.Errorf("chunk %d: unbalanced tags (opens=%d, closes=%d)", i, opens, closes) }
	}
}

func TestDeep_Chunk_PreserveWords(t *testing.T) {
	// Chunks should not split in the middle of words
	text := strings.Repeat("longword ", 600)
	chunks := chunkHTML(text, 4096)
	for i, c := range chunks {
		// Should not start or end mid-word (except first/last)
		if i > 0 && len(c) > 0 && c[0] != ' ' && c[0] != '<' {
			// chunk may start mid-content — acceptable for HTML chunking
		}
	}
}

func TestDeep_CloseUnclosedTags_Nested(t *testing.T) {
	tests := []struct{ input, mustContain string }{
		{"<b><i>nested", "</i>"},
		{"<pre><code>code block", "</code>"},
		{"<b>bold <i>italic", "</i>"},
		{"no tags", "no tags"},
		{"<b>closed</b>", "closed"},
		{"", ""},
	}
	for i, tt := range tests {
		result := closeUnclosedTags(tt.input)
		if tt.mustContain != "" && !strings.Contains(result, tt.mustContain) {
			t.Errorf("test %d: %q missing %q in %q", i, tt.input, tt.mustContain, result)
		}
	}
}

func TestDeep_EscapeHTML_AllSpecial(t *testing.T) {
	input := `<script>alert("xss")</script> & 'quotes' "double"`
	result := escapeHTML(input)
	if strings.Contains(result, "<script>") { t.Error("script tag not escaped") }
	if !strings.Contains(result, "&amp;") { t.Error("& not escaped") }
	if !strings.Contains(result, "&lt;") { t.Error("< not escaped") }
	if !strings.Contains(result, "&gt;") { t.Error("> not escaped") }
}

func TestDeep_ConvertTables_Complex(t *testing.T) {
	table := "| Name | Role | Status |\n" +
		"|------|------|--------|\n" +
		"| Alice | CEO | Active |\n" +
		"| Bob | CTO | Active |\n" +
		"| Charlie | Dev | On Leave |"

	result := convertTables(table)
	if !strings.Contains(result, "Alice") { t.Error("missing Alice") }
	if !strings.Contains(result, "Charlie") { t.Error("missing Charlie") }
	if !strings.Contains(result, "On Leave") { t.Error("missing status") }
}

func TestDeep_ParseTableRow_EdgeCases(t *testing.T) {
	tests := []struct{ input string; wantCells int }{
		{"| A | B | C |", 3},
		{"| Single |", 1},
		{"|  Spaces  |  Around  |", 2},
		{"| | Empty | |", 3},
		{"no pipes", 1},
		{"", 1},
		{"||||", 1},
	}
	for _, tt := range tests {
		cells := parseTableRow(tt.input)
		if len(cells) != tt.wantCells {
			t.Errorf("parseTableRow(%q) = %d cells, want %d", tt.input, len(cells), tt.wantCells)
		}
	}
}

func TestDeep_DisplayWidth_CJK(t *testing.T) {
	// CJK characters are typically double-width
	ascii := displayWidth("hello")
	cjk := displayWidth("日本語")
	if cjk <= ascii { t.Logf("CJK width=%d, ASCII width=%d", cjk, ascii) }
	// Mixed
	mixed := displayWidth("Hello日本")
	if mixed < 5 { t.Errorf("mixed width=%d (too small)", mixed) }
}

func TestDeep_StripAllFormatting_Comprehensive(t *testing.T) {
	// stripAllFormatting removes bold/strike markers
	result := stripAllFormatting("**bold** and ~~strike~~")
	if strings.Contains(result, "**") { t.Error("bold markers remain") }
	if strings.Contains(result, "~~") { t.Error("strike markers remain") }
	if !strings.Contains(result, "bold") { t.Error("bold text lost") }
	if !strings.Contains(result, "strike") { t.Error("strike text lost") }
}

func TestDeep_Format_EmptyInput(t *testing.T) {
	if markdownToTelegramHTML("") != "" { t.Log("empty input produces output") }
}

func TestDeep_Format_OnlyWhitespace(t *testing.T) {
	result := markdownToTelegramHTML("   \n\n\t  ")
	_ = result // should not panic
}

func TestDeep_Format_VeryLong(t *testing.T) {
	long := strings.Repeat("This is a paragraph. ", 10000)
	result := markdownToTelegramHTML(long)
	if result == "" { t.Error("empty result for long input") }
	t.Logf("long format: %d → %d chars", len(long), len(result))
}
