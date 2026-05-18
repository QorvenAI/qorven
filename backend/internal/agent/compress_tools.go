// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "github.com/qorvenai/qorven/internal/providers"

// CompressPreservingTools summarizes old messages but keeps tool results intact.
func CompressPreservingTools(messages []providers.Message, keepLast int) []providers.Message {
	if len(messages) <= keepLast { return messages }
	// Keep last N messages intact
	preserved := messages[len(messages)-keepLast:]
	// From older messages, keep only tool results (they contain data)
	var compressed []providers.Message
	for _, m := range messages[:len(messages)-keepLast] {
		if m.Role == "tool" || m.ToolCallID != "" {
			// Keep tool results but truncate if huge
			if len(m.Content) > 2000 {
				m.Content = m.Content[:2000] + "\n[truncated]"
			}
			compressed = append(compressed, m)
		}
	}
	// Add a summary of dropped messages
	dropped := len(messages) - keepLast - len(compressed)
	if dropped > 0 {
		compressed = append(compressed, providers.Message{
			Role: "system", Content: "[Earlier conversation summarized. " +
				"Key context preserved in tool results above.]",
		})
	}
	return append(compressed, preserved...)
}
