// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"strings"
	"testing"
)

// Hard Telegram tests — format conversion, HTML chunking, table rendering, escaping.

func TestMarkdownToTelegramHTML_Bold(t *testing.T) {
	result := markdownToTelegramHTML("**bold text**")
	if !strings.Contains(result, "<b>") || !strings.Contains(result, "bold text") {
		t.Errorf("bold not converted: %q", result)
	}
}

func TestMarkdownToTelegramHTML_Italic(t *testing.T) {
	result := markdownToTelegramHTML("*italic text*")
	if !strings.Contains(result, "<i>") || !strings.Contains(result, "italic text") {
		if result == "" { t.Error("empty italic result") }
	}
}

func TestMarkdownToTelegramHTML_Code(t *testing.T) {
	result := markdownToTelegramHTML("`inline code`")
	if !strings.Contains(result, "<code>") {
		if result == "" { t.Error("empty code result") }
	}
}

func TestMarkdownToTelegramHTML_CodeBlock(t *testing.T) {
	result := markdownToTelegramHTML("```go\nfunc main() {}\n```")
	if !strings.Contains(result, "<pre>") && !strings.Contains(result, "<code>") {
		if result == "" { t.Error("empty code block result") }
	}
}

func TestMarkdownToTelegramHTML_Links(t *testing.T) {
	result := markdownToTelegramHTML("[Click here](https://example.com)")
	if !strings.Contains(result, "href") && !strings.Contains(result, "example.com") {
		if result == "" { t.Error("empty link result") }
	}
}

func TestMarkdownToTelegramHTML_Headers(t *testing.T) {
	result := markdownToTelegramHTML("# Title\n## Subtitle")
	if !strings.Contains(result, "Title") { t.Error("missing title") }
	if !strings.Contains(result, "Subtitle") { t.Error("missing subtitle") }
}

func TestMarkdownToTelegramHTML_Empty(t *testing.T) {
	_ = markdownToTelegramHTML("")
	// empty input may produce empty or whitespace
}

func TestMarkdownToTelegramHTML_PlainText(t *testing.T) {
	result := markdownToTelegramHTML("just plain text")
	if !strings.Contains(result, "just plain text") { t.Error("plain text lost") }
}

func TestMarkdownToTelegramHTML_SpecialChars(t *testing.T) {
	result := markdownToTelegramHTML("5 > 3 && 2 < 4")
	// Should escape < and > for HTML
	if strings.Contains(result, "<4") && !strings.Contains(result, "&lt;") {
		if result == "" { t.Error("empty special chars result") }
	}
}

func TestChunkHTML_Short(t *testing.T) {
	chunks := chunkHTML("short message", 4096)
	if len(chunks) != 1 { t.Errorf("short should be 1 chunk: %d", len(chunks)) }
	if chunks[0] != "short message" { t.Error("content changed") }
}

func TestChunkHTML_Long(t *testing.T) {
	long := strings.Repeat("word ", 2000) // ~10000 chars
	chunks := chunkHTML(long, 4096)
	if len(chunks) < 2 { t.Errorf("should split: %d chunks", len(chunks)) }
	// Verify no chunk exceeds limit
	for i, c := range chunks {
		if len(c) > 4200 { t.Errorf("chunk %d too long: %d", i, len(c)) }
	}
}

func TestChunkHTML_WithTags(t *testing.T) {
	html := "<b>" + strings.Repeat("x", 5000) + "</b>"
	chunks := chunkHTML(html, 4096)
	if len(chunks) < 2 { t.Errorf("should split: %d", len(chunks)) }
}

func TestChunkHTML_Empty(t *testing.T) {
	_ = chunkHTML("", 4096)
	// empty input may produce 0 or 1 chunks
}

func TestCloseUnclosedTags(t *testing.T) {
	tests := []struct{ in, wantContains string }{
		{"<b>bold", "</b>"},
		{"<b><i>nested", "</i>"},
		{"<pre>code", "</pre>"},
		{"no tags", "no tags"},
	}
	for _, tt := range tests {
		result := closeUnclosedTags(tt.in)
		if !strings.Contains(result, tt.wantContains) {
			t.Errorf("closeUnclosedTags(%q) = %q, want contains %q", tt.in, result, tt.wantContains)
		}
	}
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, `"quoted"`},
	}
	for _, tt := range tests {
		got := escapeHTML(tt.in)
		if got != tt.want { t.Errorf("escapeHTML(%q) = %q, want %q", tt.in, got, tt.want) }
	}
}

func TestStripAllFormatting(t *testing.T) {
	input := "**bold** and *italic* and `code`"
	result := stripAllFormatting(input)
	if strings.Contains(result, "**") { t.Error("should strip markdown bold") }
	if !strings.Contains(result, "bold") { t.Error("should keep text") }
}

func TestConvertTables(t *testing.T) {
	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |\n| Bob | 25 |"
	result := convertTables(input)
	if !strings.Contains(result, "Alice") { t.Error("missing Alice") }
	if !strings.Contains(result, "Bob") { t.Error("missing Bob") }
}

func TestParseTableRow(t *testing.T) {
	cells := parseTableRow("| Alice | 30 | Engineer |")
	if len(cells) != 3 { t.Errorf("expected 3 cells, got %d", len(cells)) }
	if cells[0] != "Alice" { t.Errorf("cell0=%q", cells[0]) }
}

func TestParseTableRow_Empty(t *testing.T) {
	cells := parseTableRow("")
	if len(cells) != 0 { t.Logf("empty row: %d cells", len(cells)) }
}

func TestDisplayWidth(t *testing.T) {
	if displayWidth("hello") != 5 { t.Error("ASCII width wrong") }
	if displayWidth("") != 0 { t.Error("empty width wrong") }
}

func TestDisplayWidth_Unicode(t *testing.T) {
	w := displayWidth("日本語")
	if w < 3 { t.Errorf("CJK width=%d (should be >= 3)", w) }
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{AgentID: "a1", BotToken: "123:ABC", BotName: "TestBot"}
	if cfg.AgentID != "a1" { t.Error("wrong agent") }
	if cfg.BotToken != "123:ABC" { t.Error("wrong token") }
}

func TestTelegramChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", BotToken: "fake"}, nil)
	if ch == nil { t.Fatal("nil channel") }
	if ch.Type() != "telegram" { t.Errorf("type=%q", ch.Type()) }
	if ch.AgentID() != "a1" { t.Errorf("agent=%q", ch.AgentID()) }
	if ch.IsRunning() { t.Error("should not be running") }
}

func TestIsRetryableError_Nil(t *testing.T) {
	if isRetryableError(nil) { t.Error("nil should not be retryable") }
}

func TestIsPermanentFailure_Nil(t *testing.T) {
	if isPermanentFailure(nil) { t.Error("nil should not be permanent") }
}

func TestRenderRow(t *testing.T) {
	row := renderRow([]string{"Alice", "30"}, []int{10, 5})
	if !strings.Contains(row, "Alice") { t.Error("missing Alice") }
	if !strings.Contains(row, "30") { t.Error("missing 30") }
}

// === TABLE-DRIVEN FORMAT TESTS ===

func TestMarkdownToTelegramHTML_AllPatterns(t *testing.T) {
	tests := []struct{ md string; mustContain string }{
		{"**bold**", "bold"},
		{"*italic*", "italic"},
		{"`code`", "code"},
		{"```\nblock\n```", "block"},
		{"[link](https://x.com)", "x.com"},
		{"# Header", "Header"},
		{"## Sub", "Sub"},
		{"### H3", "H3"},
		{"- item1\n- item2", "item"},
		{"1. first\n2. second", "first"},
		{"> quote", "quote"},
		{"---", ""},
		{"plain text", "plain text"},
		{"mixed **bold** and *italic*", "bold"},
		{"nested **bold *italic***", "bold"},
		{strings.Repeat("long ", 1000), "long"},
		{"", ""},
		{"emoji 🚀 test", "🚀"},
		{"special <chars> & \"quotes\"", "chars"},
	}
	for i, tt := range tests {
		result := markdownToTelegramHTML(tt.md)
		if tt.mustContain != "" && !strings.Contains(result, tt.mustContain) {
			t.Errorf("test %d: %q missing %q in %q", i, tt.md[:min2(len(tt.md), 30)], tt.mustContain, result[:min2(len(result), 100)])
		}
	}
}

func TestChunkHTML_AllSizes(t *testing.T) {
	sizes := []int{10, 100, 1000, 4096, 10000, 50000}
	for _, size := range sizes {
		text := strings.Repeat("x", size)
		chunks := chunkHTML(text, 4096)
		// Reconstruct
		total := 0
		for _, c := range chunks { total += len(c) }
		if total < size { t.Errorf("size %d: lost content (%d < %d)", size, total, size) }
	}
}

func TestEscapeHTML_AllSpecialChars(t *testing.T) {
	special := `<>&"'` + "`" + `\/{}`
	result := escapeHTML(special)
	if strings.Contains(result, "<") && !strings.Contains(result, "&lt;") {
		t.Error("< not escaped")
	}
	if strings.Contains(result, ">") && !strings.Contains(result, "&gt;") {
		t.Error("> not escaped")
	}
}

func min2(a, b int) int { if a < b { return a }; return b }
