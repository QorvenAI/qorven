// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"strings"
	"unicode/utf8"
)

// ValidateInput checks a string for dangerous content.
// Returns error message or empty string if valid.
func ValidateInput(field, value string, maxLen int) string {
	if maxLen > 0 && utf8.RuneCountInString(value) > maxLen {
		return field + " exceeds max length"
	}
	if strings.ContainsAny(value, "\x00") {
		return field + " contains null bytes"
	}
	return ""
}

// ValidateAgentInput validates agent creation/update fields.
func ValidateAgentInput(name, prompt, model string) string {
	if err := ValidateInput("display_name", name, 100); err != "" { return err }
	if err := ValidateInput("system_prompt", prompt, 50000); err != "" { return err }
	if err := ValidateInput("model", model, 200); err != "" { return err }
	return ""
}

// ValidateMessageInput validates chat message input.
func ValidateMessageInput(message string) string {
	if len(message) > 500000 { return "message too long (max 500KB)" }
	if strings.ContainsAny(message, "\x00") { return "message contains null bytes" }
	return ""
}

// ScrubSensitive removes API keys and tokens from log strings.
func ScrubSensitive(s string) string {
	// Scrub common key patterns
	for _, prefix := range []string{"sk-", "fc-", "tvly-", "pplx-", "Bearer ", "xoxb-", "xapp-", "ghp_", "AIza"} {
		if idx := strings.Index(s, prefix); idx >= 0 {
			end := idx + len(prefix)
			for end < len(s) && s[end] != ' ' && s[end] != '"' && s[end] != '\'' && s[end] != ',' {
				end++
			}
			s = s[:idx+len(prefix)] + "***" + s[end:]
		}
	}
	return s
}
