// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import "strings"

// gemini_compat.go — Gemini-specific message transformations.
// Gemini 2.5+ requires thought_signature echoed back on tool_call messages.
// When models don't return it, we collapse tool cycles to avoid HTTP 400.

// CollapseToolCallsWithoutSignature rewrites tool_call cycles that lack thought_signature.
// Gemini requires thought_signature echoed back; models that don't return it cause 400.
// The assistant's tool_calls are stripped, tool results folded into a user message.
func CollapseToolCallsWithoutSignature(msgs []Message) []Message {
	collapseIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 { continue }
		for _, tc := range m.ToolCalls {
			sig := ""
			if tc.Metadata != nil {
				sig = tc.Metadata["thought_signature"]
				if sig == "" { sig = tc.Metadata["thoughtSignature"] }
			}
			if strings.TrimSpace(sig) == "" {
				for _, tc2 := range m.ToolCalls { collapseIDs[tc2.ID] = true }
				break
			}
		}
	}
	if len(collapseIDs) == 0 { return msgs }

	result := make([]Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]

		// Strip tool_calls from assistant, keep content
		if m.Role == "assistant" && len(m.ToolCalls) > 0 && collapseIDs[m.ToolCalls[0].ID] {
			if m.Content != "" {
				result = append(result, Message{Role: "assistant", Content: m.Content})
			}
			// Fold consecutive tool results into one user message
			var parts []string
			for i+1 < len(msgs) && msgs[i+1].Role == "tool" && collapseIDs[msgs[i+1].ToolCallID] {
				i++
				if content := strings.TrimSpace(msgs[i].Content); content != "" {
					parts = append(parts, content)
				}
			}
			if len(parts) > 0 {
				result = append(result, Message{Role: "user", Content: strings.Join(parts, "\n\n")})
			}
			continue
		}

		// Skip orphaned tool results
		if m.Role == "tool" && collapseIDs[m.ToolCallID] { continue }

		result = append(result, m)
	}
	return result
}
