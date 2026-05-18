// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// looksLikeToolIntent detects when the LLM describes a tool action in text
// instead of actually calling the tool.
func looksLikeToolIntent(content string) bool {
	lower := strings.ToLower(content)
	patterns := []string{
		"i'll search", "i will search", "let me search",
		"i'll look up", "i will look up", "let me look up",
		"i'll fetch", "i will fetch", "let me fetch",
		"i'll find", "i will find", "let me find",
		"searching for", "looking up", "fetching",
		"i'll check", "i will check", "let me check the web",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) { return true }
	}
	return false
}

// ShouldEnforceToolUse returns true if we should retry with tool_choice:"required"
func ShouldEnforceToolUse(content string, hadToolCalls bool, modelCaps providers.ProviderCapabilities) bool {
	if hadToolCalls { return false } // model already called tools
	if !modelCaps.SupportsTools { return false }
	return looksLikeToolIntent(content)
}

// ToolChoiceForRetry returns the tool_choice to use on retry.
func ToolChoiceForRetry(attempt int) string {
	if attempt == 0 { return "auto" }
	return "required" // force tool use on retry
}



