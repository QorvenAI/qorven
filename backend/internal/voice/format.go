// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"fmt"
	"regexp"
	"strings"
)

// Audio processing utilities: markdown stripping for TTS, format detection, transcript tags.
// Every TTS call should go through CleanTextForTTS before synthesis.

// CleanTextForTTS strips all markdown, code, URLs, and formatting that sounds bad when spoken.
// This is the single entry point — call this before any TTS synthesis.
func CleanTextForTTS(text string) string {
	text = stripCodeBlocks(text)
	text = stripInlineCode(text)
	text = stripMarkdownFormatting(text)
	text = stripURLs(text)
	text = stripHTMLTags(text)
	text = stripTables(text)
	text = cleanWhitespace(text)
	return strings.TrimSpace(text)
}

// --- Individual strippers ---

var codeBlockRe = regexp.MustCompile("(?s)```[\\w]*\\n?(.*?)```")

func stripCodeBlocks(text string) string {
	// Replace code blocks with a spoken summary
	return codeBlockRe.ReplaceAllStringFunc(text, func(match string) string {
		lines := strings.Count(match, "\n")
		if lines <= 2 { return "[code snippet]" }
		return "[code block with " + strings.TrimSpace(strings.Split(match, "\n")[0][3:]) + "]"
	})
}

func stripInlineCode(text string) string {
	return regexp.MustCompile("`([^`]+)`").ReplaceAllString(text, "$1")
}

func stripMarkdownFormatting(text string) string {
	// Bold: **text** or __text__ → text
	text = regexp.MustCompile(`\*\*(.+?)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`__(.+?)__`).ReplaceAllString(text, "$1")
	// Italic: *text* or _text_ → text (careful not to strip bullet points)
	text = regexp.MustCompile(`(?:^|\s)\*([^*\n]+)\*(?:\s|$)`).ReplaceAllString(text, " $1 ")
	// Strikethrough: ~~text~~ → text
	text = regexp.MustCompile(`~~(.+?)~~`).ReplaceAllString(text, "$1")
	// Headers: # text → text
	text = regexp.MustCompile(`(?m)^#{1,6}\s+`).ReplaceAllString(text, "")
	// Bullet points: - text or * text → text
	text = regexp.MustCompile(`(?m)^[\-\*]\s+`).ReplaceAllString(text, "")
	// Links: [text](url) → text
	text = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(text, "$1")
	// Images: ![alt](url) → alt
	text = regexp.MustCompile(`!\[([^\]]*)\]\([^)]+\)`).ReplaceAllString(text, "$1")
	return text
}

var urlRe = regexp.MustCompile(`https?://[^\s<>\[\]()]+`)

func stripURLs(text string) string {
	return urlRe.ReplaceAllString(text, "")
}

func stripHTMLTags(text string) string {
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, "")
}

func stripTables(text string) string {
	// Replace markdown tables with spoken description
	lines := strings.Split(text, "\n")
	result := []string{}
	inTable := false
	tableRows := 0
	for _, line := range lines {
		isTableRow := strings.HasPrefix(strings.TrimSpace(line), "|") && strings.HasSuffix(strings.TrimSpace(line), "|")
		isSeparator := regexp.MustCompile(`^\|[-:|]+\|$`).MatchString(strings.TrimSpace(line))

		if isTableRow && !isSeparator {
			if !inTable {
				inTable = true
				tableRows = 0
			}
			tableRows++
			continue
		}
		if isSeparator { continue }
		if inTable {
			result = append(result, "[table with "+fmt.Sprintf("%d", tableRows)+" rows]")
			inTable = false
		}
		result = append(result, line)
	}
	if inTable {
		result = append(result, "[table with "+fmt.Sprintf("%d", tableRows)+" rows]")
	}
	return strings.Join(result, "\n")
}

func cleanWhitespace(text string) string {
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	text = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(text, " ")
	return text
}

// --- Platform Audio Format ---

// PlatformAudioFormat returns the preferred audio format for a channel type.
func PlatformAudioFormat(channelType string) string {
	switch channelType {
	case "telegram":
		return "ogg" // Telegram voice messages use OGG/Opus
	case "whatsapp":
		return "ogg" // WhatsApp also uses OGG
	case "discord":
		return "ogg" // Discord uses Opus in OGG
	case "web", "webchat":
		return "mp3" // Web browsers prefer MP3
	default:
		return "mp3" // MP3 is universally supported
	}
}

// --- Voice Transcript Tags (for LLM context) ---

// BuildVoiceTranscriptTag wraps a voice transcription in XML tags so the LLM
// knows the content came from a voice message, not typed text.
func BuildVoiceTranscriptTag(filename, mime, transcript string) string {
	return "<voice_transcript name=\"" + filename + "\" mime=\"" + mime + "\">\n" + transcript + "\n</voice_transcript>"
}

// BuildVoiceMessagePrefix creates the prefix injected before a voice transcript
// so the LLM understands the user spoke rather than typed.
func BuildVoiceMessagePrefix(transcript string) string {
	return `[Voice message. Reply concisely for speech — use city names not full addresses, say "degrees Celsius" not "°C", avoid markdown formatting, keep responses under 3 sentences unless asked for detail. Do not ask generic follow-up questions — only suggest next steps if clearly relevant.]

User said: "` + transcript + `"`
}

// --- Audio Format Detection ---

// DetectAudioFormat guesses the audio format from the first bytes (magic numbers).
func DetectAudioFormat(data []byte) string {
	if len(data) < 4 { return "unknown" }
	// OGG: starts with "OggS"
	if data[0] == 'O' && data[1] == 'g' && data[2] == 'g' && data[3] == 'S' { return "ogg" }
	// MP3: starts with 0xFF 0xFB or ID3 tag
	if (data[0] == 0xFF && (data[1]&0xE0) == 0xE0) || (data[0] == 'I' && data[1] == 'D' && data[2] == '3') { return "mp3" }
	// WAV: starts with "RIFF"
	if data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' { return "wav" }
	// FLAC: starts with "fLaC"
	if data[0] == 'f' && data[1] == 'L' && data[2] == 'a' && data[3] == 'C' { return "flac" }
	// WebM: starts with 0x1A 0x45 0xDF 0xA3
	if data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 { return "webm" }
	// M4A/MP4: "ftyp" at offset 4
	if len(data) >= 8 && data[4] == 'f' && data[5] == 't' && data[6] == 'y' && data[7] == 'p' { return "m4a" }
	return "unknown"
}
