// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"strings"
	"testing"
)

// Hard voice tests — TTS text cleaning, format stripping, audio format detection.

func TestCleanTextForTTS_Plain(t *testing.T) {
	result := CleanTextForTTS("Hello world")
	if result != "Hello world" { t.Errorf("plain text changed: %q", result) }
}

func TestCleanTextForTTS_CodeBlocks(t *testing.T) {
	input := "Here's code:\n```go\nfunc main() {}\n```\nDone."
	result := CleanTextForTTS(input)
	if strings.Contains(result, "```") { t.Error("code fences should be stripped") }
	if strings.Contains(result, "func main") { t.Error("code content should be stripped") }
	if !strings.Contains(result, "Done") { t.Error("text after code should remain") }
}

func TestCleanTextForTTS_InlineCode(t *testing.T) {
	result := CleanTextForTTS("Use the `fmt.Println` function")
	if strings.Contains(result, "`") { t.Error("backticks should be stripped") }
	if !strings.Contains(result, "fmt.Println") { t.Error("inline code text should remain") }
}

func TestCleanTextForTTS_Bold(t *testing.T) {
	result := CleanTextForTTS("This is **bold** text")
	if strings.Contains(result, "**") { t.Error("bold markers should be stripped") }
	if !strings.Contains(result, "bold") { t.Error("bold text should remain") }
}

func TestCleanTextForTTS_Italic(t *testing.T) {
	result := CleanTextForTTS("This is *italic* text")
	if strings.Contains(result, "*italic*") { t.Error("italic markers should be stripped") }
}

func TestCleanTextForTTS_URLs(t *testing.T) {
	result := CleanTextForTTS("Visit https://example.com/path?q=1 for more")
	if strings.Contains(result, "https://") { t.Error("URLs should be stripped") }
}

func TestCleanTextForTTS_HTMLTags(t *testing.T) {
	result := CleanTextForTTS("Hello <b>bold</b> and <a href='url'>link</a>")
	if strings.Contains(result, "<b>") { t.Error("HTML tags should be stripped") }
	if !strings.Contains(result, "bold") { t.Error("tag content should remain") }
}

func TestCleanTextForTTS_Tables(t *testing.T) {
	input := "| Name | Age |\n|------|-----|\n| Alice | 30 |"
	result := CleanTextForTTS(input)
	if strings.Contains(result, "|") { t.Error("table pipes should be stripped") }
}

func TestCleanTextForTTS_Empty(t *testing.T) {
	result := CleanTextForTTS("")
	if result != "" { t.Errorf("empty should stay empty: %q", result) }
}

func TestCleanTextForTTS_Whitespace(t *testing.T) {
	result := CleanTextForTTS("  lots   of   spaces  \n\n\n  and newlines  ")
	if strings.Contains(result, "   ") { t.Error("excessive whitespace should be cleaned") }
}

func TestCleanTextForTTS_Complex(t *testing.T) {
	input := `# Title
Here's a **bold** statement with `+"`code`"+` and a [link](https://example.com).

`+"```python"+`
print("hello")
`+"```"+`

| Col1 | Col2 |
|------|------|
| A    | B    |

The end.`
	result := CleanTextForTTS(input)
	if strings.Contains(result, "```") { t.Error("code fences remain") }
	if strings.Contains(result, "https://") { t.Error("URL remains") }
	if !strings.Contains(result, "end") { t.Error("final text lost") }
}

func TestStripCodeBlocks(t *testing.T) {
	input := "before\n```\ncode\n```\nafter"
	result := stripCodeBlocks(input)
	if strings.Contains(result, "```") { t.Error("code fences should be stripped") }
	if !strings.Contains(result, "before") { t.Error("text before should remain") }
	if !strings.Contains(result, "after") { t.Error("text after should remain") }
}

func TestStripInlineCode(t *testing.T) {
	result := stripInlineCode("Use `fmt.Println` here")
	if strings.Contains(result, "`") { t.Error("backticks should be stripped") }
}

func TestStripMarkdownFormatting(t *testing.T) {
	result := stripMarkdownFormatting("**bold** and *italic* and ~~strike~~")
	if strings.Contains(result, "**") { t.Error("bold markers remain") }
	if strings.Contains(result, "~~") { t.Error("strike markers remain") }
}

func TestStripURLs(t *testing.T) {
	result := stripURLs("Visit https://example.com and http://test.org/path")
	if strings.Contains(result, "https://") { t.Error("https URL remains") }
	if strings.Contains(result, "http://") { t.Error("http URL remains") }
}

func TestStripHTMLTags(t *testing.T) {
	result := stripHTMLTags("<p>Hello <b>world</b></p>")
	if strings.Contains(result, "<") { t.Error("HTML tags remain") }
	if !strings.Contains(result, "Hello") { t.Error("text lost") }
	if !strings.Contains(result, "world") { t.Error("text lost") }
}

func TestStripTables(t *testing.T) {
	input := "text\n| A | B |\n|---|---|\n| 1 | 2 |\nmore text"
	result := stripTables(input)
	if strings.Contains(result, "|") { t.Error("table pipes remain") }
}

func TestCleanWhitespace(t *testing.T) {
	result := cleanWhitespace("  hello   world  \n\n\n  test  ")
	if strings.Contains(result, "   ") { t.Error("triple spaces remain") }
	if strings.Contains(result, "\n\n\n") { t.Error("triple newlines remain") }
}

func TestPlatformAudioFormat(t *testing.T) {
	tests := []struct{ channel, want string }{
		{"telegram", "ogg"},
		{"discord", "ogg"},
		{"whatsapp", "ogg"},
		{"web", "mp3"},
		{"unknown", "mp3"},
	}
	for _, tt := range tests {
		got := PlatformAudioFormat(tt.channel)
		if got != tt.want { t.Errorf("PlatformAudioFormat(%q)=%q, want %q", tt.channel, got, tt.want) }
	}
}

func TestBuildVoiceTranscriptTag(t *testing.T) {
	tag := BuildVoiceTranscriptTag("audio.ogg", "audio/ogg", "Hello world")
	if tag == "" { t.Error("empty tag") }
	if !strings.Contains(tag, "Hello world") { t.Error("missing transcript") }
}

func TestBuildVoiceTranscriptTag_Empty(t *testing.T) {
	tag := BuildVoiceTranscriptTag("", "", "")
	_ = tag // should not panic
}

func TestManager_New(t *testing.T) {
	m := NewManager()
	if m == nil { t.Fatal("nil manager") }
}

func TestManager_HasTTS_Empty(t *testing.T) {
	m := NewManager()
	if m.HasTTS() { t.Error("should not have TTS without providers") }
}

func TestManager_HasSTT_Empty(t *testing.T) {
	m := NewManager()
	if m.HasSTT() { t.Error("should not have STT without providers") }
}

// === TABLE-DRIVEN VOICE TESTS ===

func TestCleanTextForTTS_AllPatterns(t *testing.T) {
	tests := []struct{ input string; mustNotContain string; mustContain string }{
		{"plain text", "", "plain text"},
		{"**bold**", "**", "bold"},
		{"*italic*", "", "italic"},
		{"`code`", "`", "code"},
		{"```go\nfunc main(){}\n```", "```", ""},
		{"[link](https://x.com)", "https://", "link"},
		{"https://example.com/path", "https://", ""},
		{"<b>html</b>", "<b>", "html"},
		{"| A | B |\n|---|---|\n| 1 | 2 |", "|", ""},
		{"  lots   of   spaces  ", "   ", ""},
		{"\n\n\n\nmany newlines\n\n\n", "\n\n\n", "newlines"},
		{"~~strikethrough~~", "~~", ""},
		{"# Header", "#", "Header"},
		{"emoji 🎉 test", "", "🎉"},
		{"", "", ""},
		{strings.Repeat("word ", 10000), "", "word"},
	}
	for i, tt := range tests {
		result := CleanTextForTTS(tt.input)
		if tt.mustNotContain != "" && strings.Contains(result, tt.mustNotContain) {
			t.Errorf("test %d: %q still in result", i, tt.mustNotContain)
		}
		if tt.mustContain != "" && !strings.Contains(result, tt.mustContain) {
			t.Errorf("test %d: %q missing from result", i, tt.mustContain)
		}
	}
}

func TestPlatformAudioFormat_AllChannels(t *testing.T) {
	channels := []struct{ ch, want string }{
		{"telegram", "ogg"}, {"discord", "ogg"}, {"whatsapp", "ogg"},
		{"slack", "mp3"}, {"web", "mp3"}, {"webchat", "mp3"},
		{"email", "mp3"}, {"sms", "mp3"}, {"unknown", "mp3"},
		{"", "mp3"},
	}
	for _, tt := range channels {
		got := PlatformAudioFormat(tt.ch)
		if got != tt.want { t.Errorf("PlatformAudioFormat(%q)=%q, want %q", tt.ch, got, tt.want) }
	}
}
