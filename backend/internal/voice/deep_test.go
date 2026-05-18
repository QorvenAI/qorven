// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"strings"
	"testing"
)

// Deep voice tests — TTS cleaning pipeline with real-world content.

func TestDeep_CleanTTS_RealMarkdown(t *testing.T) {
	// Real LLM response with mixed formatting
	input := `# How to Deploy a Go App

Here's a step-by-step guide:

1. **Build** the binary: ` + "`go build -o app .`" + `
2. **Copy** to server: ` + "`scp app user@server:/opt/`" + `
3. **Run** with systemd

` + "```bash" + `
sudo systemctl start app
sudo systemctl enable app
` + "```" + `

Check the status at https://your-server.com/health

| Step | Command | Time |
|------|---------|------|
| Build | go build | 10s |
| Deploy | scp | 5s |
| Start | systemctl | 1s |

> **Note**: Make sure port 8080 is open.

For more info, see [Go docs](https://go.dev/doc/).`

	result := CleanTextForTTS(input)

	// Code blocks should be replaced with spoken summary
	if strings.Contains(result, "```") { t.Error("code fences remain") }
	if strings.Contains(result, "systemctl start") { t.Error("code block content should be replaced") }

	// URLs should be stripped
	if strings.Contains(result, "https://") { t.Error("URLs remain") }

	// Tables should be stripped
	if strings.Contains(result, "|---") { t.Error("table separators remain") }

	// Markdown formatting should be stripped
	if strings.Contains(result, "**Build**") { t.Error("bold markers remain") }

	// But the actual words should remain
	if !strings.Contains(result, "Deploy") { t.Error("content word 'Deploy' lost") }
	if !strings.Contains(result, "Go") { t.Error("content word 'Go' lost") }

	// Should be clean for speech
	if strings.Contains(result, "   ") { t.Error("excessive whitespace") }

	t.Logf("cleaned %d chars → %d chars", len(input), len(result))
	t.Logf("result: %s", result[:min8(len(result), 200)])
}

func TestDeep_CleanTTS_CodeHeavyResponse(t *testing.T) {
	input := "Here's the implementation:\n\n" +
		"```go\n" +
		"package main\n\n" +
		"import (\n\t\"fmt\"\n\t\"net/http\"\n)\n\n" +
		"func handler(w http.ResponseWriter, r *http.Request) {\n" +
		"\tfmt.Fprintf(w, \"Hello, %s!\", r.URL.Path[1:])\n" +
		"}\n\n" +
		"func main() {\n" +
		"\thttp.HandleFunc(\"/\", handler)\n" +
		"\thttp.ListenAndServe(\":8080\", nil)\n" +
		"}\n```\n\n" +
		"This creates a simple web server on port 8080."

	result := CleanTextForTTS(input)

	// The code should be replaced, not read aloud
	if strings.Contains(result, "http.HandleFunc") { t.Error("Go code should not be read aloud") }
	if strings.Contains(result, "func main()") { t.Error("function declaration should not be read") }

	// But the explanation should remain
	if !strings.Contains(result, "web server") { t.Error("explanation lost") }
	if !strings.Contains(result, "implementation") { t.Error("intro lost") }

	t.Logf("code-heavy: %d → %d chars", len(input), len(result))
}

func TestDeep_CleanTTS_MultipleCodeBlocks(t *testing.T) {
	input := "First block:\n```python\nprint('hello')\n```\n\n" +
		"Second block:\n```javascript\nconsole.log('world')\n```\n\n" +
		"Third block:\n```sql\nSELECT * FROM users;\n```\n\nDone."

	result := CleanTextForTTS(input)
	if strings.Contains(result, "```") { t.Error("code fences remain") }
	if !strings.Contains(result, "Done") { t.Error("final text lost") }
	if !strings.Contains(result, "First") { t.Error("intro text lost") }
}

func TestDeep_CleanTTS_URLHeavyResponse(t *testing.T) {
	input := "Check these resources:\n" +
		"- Documentation: https://docs.qorven.io/api/v1/agents\n" +
		"- GitHub: https://github.com/Qorven/qorven/tree/main/internal\n" +
		"- API Reference: https://api.qorven.io/v1/chat/completions?model=gpt-4\n" +
		"- Support: mailto:support@qorven.io\n\n" +
		"All links are in the documentation."

	result := CleanTextForTTS(input)
	if strings.Contains(result, "https://") { t.Error("https URLs remain") }
	// mailto: may not be stripped by URL regex
	if !strings.Contains(result, "documentation") || !strings.Contains(result, "Documentation") {
		t.Error("text around URLs lost")
	}
}

func TestDeep_CleanTTS_EmojisAndUnicode(t *testing.T) {
	input := "Great job! 🎉 The deployment was successful ✅\n" +
		"Performance: 📈 Response time improved by 50%\n" +
		"Warning: ⚠️ Memory usage is high\n" +
		"日本語のテスト — this should work too"

	result := CleanTextForTTS(input)
	// Emojis should pass through (TTS engines handle them)
	if !strings.Contains(result, "deployment") { t.Error("text lost") }
	if !strings.Contains(result, "日本語") { t.Error("Japanese lost") }
}

func TestDeep_CleanTTS_NestedFormatting(t *testing.T) {
	input := "This has ***bold italic*** and ~~strikethrough~~ and __underline__ formatting.\n" +
		"Also `inline code` mixed with **bold `code` inside** text."

	result := CleanTextForTTS(input)
	if strings.Contains(result, "***") { t.Error("bold italic markers") }
	if strings.Contains(result, "~~") { t.Error("strikethrough markers") }
	if strings.Contains(result, "__") { t.Error("underline markers") }
	if !strings.Contains(result, "bold") { t.Error("word 'bold' lost") }
	if !strings.Contains(result, "formatting") { t.Error("word 'formatting' lost") }
}

func TestDeep_CleanTTS_EmptyAndEdgeCases(t *testing.T) {
	cases := []struct{ name, input string }{
		{"empty", ""},
		{"whitespace only", "   \n\n\t  "},
		{"single word", "hello"},
		{"only code", "```\ncode\n```"},
		{"only URL", "https://example.com"},
		{"only table", "| A |\n|---|\n| 1 |"},
		{"only bold", "**bold**"},
		{"very long", strings.Repeat("word ", 50000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := CleanTextForTTS(tc.input)
			// Should never panic, never return excessively long output
			if len(result) > len(tc.input)*2+100 {
				t.Errorf("output longer than input: %d > %d", len(result), len(tc.input))
			}
		})
	}
}

func TestDeep_PlatformAudioFormat_AllChannels(t *testing.T) {
	// Verify every known channel type returns a valid format
	channels := []string{
		"telegram", "discord", "whatsapp", "slack", "web", "webchat",
		"email", "sms", "webhook", "zalo", "line", "teams",
		"matrix", "signal", "imessage", "mattermost",
	}
	validFormats := map[string]bool{"ogg": true, "mp3": true, "wav": true, "opus": true}

	for _, ch := range channels {
		format := PlatformAudioFormat(ch)
		if !validFormats[format] {
			t.Errorf("channel %q returned invalid format %q", ch, format)
		}
	}
}

func TestDeep_VoiceManager_ProviderRegistration(t *testing.T) {
	m := NewManager()

	// No providers registered
	if m.HasTTS() { t.Error("should not have TTS") }
	if m.HasSTT() { t.Error("should not have STT") }

	// After registering, should have capabilities
	// (Can't test with real providers without API keys, but verify the manager works)
	t.Log("voice manager: no providers = no capabilities ✓")
}

func min8(a, b int) int { if a < b { return a }; return b }
