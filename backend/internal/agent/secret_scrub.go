// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// SecretScrubber provides two-layer output scrubbing:
//  1. Exact-match redaction of known secret values (proactive)
//  2. Regex-based leak detection for unknown secrets (reactive)
//
type SecretScrubber struct {
	secrets       []secretEntry // name→value pairs, sorted longest-first
	maxSecretLen  int
	tail          string // rolling buffer for chunk boundary handling
}

type secretEntry struct {
	name  string
	value string
}

// NewSecretScrubber creates a scrubber with known secret name/value pairs.
func NewSecretScrubber(secrets map[string]string) *SecretScrubber {
	entries := make([]secretEntry, 0, len(secrets))
	maxLen := 0
	for name, value := range secrets {
		if value == "" {
			continue
		}
		entries = append(entries, secretEntry{name: name, value: value})
		if len(value) > maxLen {
			maxLen = len(value)
		}
	}
	// Sort by descending value length — longer secrets replaced first
	// to prevent partial replacement when one secret is a prefix of another.
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].value) > len(entries[j].value)
	})
	return &SecretScrubber{secrets: entries, maxSecretLen: maxLen}
}

// ScrubChunk scrubs a streaming chunk, handling secrets split across boundaries.
// Holds back maxSecretLen chars to catch splits. Call Flush() when stream ends.
func (s *SecretScrubber) ScrubChunk(chunk string) string {
	if len(s.secrets) == 0 {
		return chunk
	}

	combined := s.tail + chunk
	scrubbed := s.exactMatch(combined)

	// Hold back last maxSecretLen chars for boundary handling
	if s.maxSecretLen > 0 && len(scrubbed) > s.maxSecretLen {
		splitAt := len(scrubbed) - s.maxSecretLen
		// Respect UTF-8 boundaries
		for splitAt > 0 && splitAt < len(scrubbed) && !isUTF8Start(scrubbed[splitAt]) {
			splitAt--
		}
		s.tail = scrubbed[splitAt:]
		return scrubbed[:splitAt]
	}

	// Entire scrubbed text fits in hold-back window — buffer it all
	s.tail = scrubbed
	return ""
}

// Flush returns any remaining buffered content with a final scrub pass.
func (s *SecretScrubber) Flush() string {
	tail := s.tail
	s.tail = ""
	if tail == "" {
		return ""
	}
	return s.exactMatch(tail)
}

// ScrubAll scrubs a complete string (non-streaming). Convenience wrapper.
func (s *SecretScrubber) ScrubAll(text string) string {
	return s.exactMatch(text)
}

func (s *SecretScrubber) exactMatch(text string) string {
	result := text
	for _, entry := range s.secrets {
		result = strings.ReplaceAll(result, entry.value, "[REDACTED:"+entry.name+"]")
	}
	return result
}

func isUTF8Start(b byte) bool {
	return b&0xC0 != 0x80 // Not a continuation byte
}

// --- Layer 2: Regex Leak Detection ---

// 11 patterns for known API key formats.
var leakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[a-zA-Z0-9_-]{20,}`),                                                   // OpenAI (includes sk-proj-)
	regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]{20,}`),                                              // Anthropic
	regexp.MustCompile(`sk-or-[a-zA-Z0-9_-]{20,}`),                                               // OpenRouter
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),                                                        // AWS Access Key
	regexp.MustCompile(`-----BEGIN[A-Z \r\n]*PRIVATE KEY-----`),                                   // PEM header
	regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),                                                    // GitHub PAT
	regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),                                                  // Google API
	regexp.MustCompile(`[MN][A-Za-z0-9]{23,}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}`),            // Discord
	regexp.MustCompile(`xoxb-[0-9]{10,}-[0-9A-Za-z-]+`),                                          // Slack bot
	regexp.MustCompile(`xapp-[0-9]-[A-Z0-9]+-[0-9]+-[a-f0-9]+`),                                 // Slack app
	regexp.MustCompile(`\d{8,}:[A-Za-z0-9_-]{35}`),                                               // Telegram bot
	regexp.MustCompile(`gsk_[a-zA-Z0-9]{20,}`),                                                   // Groq
}

// Full PEM block pattern (header + body + footer).
var pemBlockPattern = regexp.MustCompile(`(?s)-----BEGIN[A-Z \r\n]*PRIVATE KEY-----.*?-----END[A-Z \r\n]*PRIVATE KEY-----`)

// ScanForLeaks checks content for potential secret leaks, including encoded forms.
// Returns the first matched secret string, or empty if clean.
func ScanForLeaks(content string) string {
	// Layer 1: Plaintext patterns
	if match := matchLeakPatterns(content); match != "" {
		return match
	}

	// Layer 2: URL-encoded secrets
	if decoded, err := url.QueryUnescape(content); err == nil && decoded != content {
		if match := matchLeakPatterns(decoded); match != "" {
			return "url-encoded: " + match
		}
	}

	// Layer 3: Base64-encoded secrets
	b64Pattern := regexp.MustCompile(`[A-Za-z0-9+/]{24,}={0,2}`)
	for _, segment := range b64Pattern.FindAllString(content, 10) {
		if decoded, err := base64.StdEncoding.DecodeString(segment); err == nil {
			if match := matchLeakPatterns(string(decoded)); match != "" {
				return "base64-encoded: " + match
			}
		}
		if decoded, err := base64.URLEncoding.DecodeString(segment); err == nil {
			if match := matchLeakPatterns(string(decoded)); match != "" {
				return "base64-encoded: " + match
			}
		}
	}

	// Layer 4: Hex-encoded secrets
	hexPattern := regexp.MustCompile(`(?i)(?:0x)?([0-9a-f]{40,})`)
	for _, caps := range hexPattern.FindAllStringSubmatch(content, 5) {
		if len(caps) < 2 {
			continue
		}
		if decoded, err := hex.DecodeString(caps[1]); err == nil {
			if match := matchLeakPatterns(string(decoded)); match != "" {
				return "hex-encoded: " + match
			}
		}
	}

	return ""
}

func matchLeakPatterns(content string) string {
	for _, pattern := range leakPatterns {
		if match := pattern.FindString(content); match != "" {
			return match
		}
	}
	return ""
}

// ScrubLeaks replaces ALL detected leak patterns with [LEAKED_SECRET_REDACTED].
// Used on egress paths (tool results, agent responses).
func ScrubLeaks(content string) string {
	// First: full PEM blocks (header + body + footer)
	result := pemBlockPattern.ReplaceAllString(content, "[LEAKED_SECRET_REDACTED]")
	// Then: individual patterns
	for _, pattern := range leakPatterns {
		result = pattern.ReplaceAllString(result, "[LEAKED_SECRET_REDACTED]")
	}
	return result
}
