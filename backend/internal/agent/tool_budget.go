// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "strings"

// ToolBudget controls how much output tools can produce per result and per turn.
// Prevents context window blowout from large file reads or verbose command output.
type ToolBudget struct {
	PerResultChars int // max chars per single tool result (default 50K)
	PerTurnChars   int // max total chars across all tool results in one turn (default 200K)
	PreviewChars   int // chars shown in truncation preview (default 2K)
}

// DefaultToolBudget returns sensible per-result and per-turn limits
// tuned for long-running agent sessions.
func DefaultToolBudget() ToolBudget {
	return ToolBudget{PerResultChars: 50_000, PerTurnChars: 200_000, PreviewChars: 2_000}
}

// EnforceResult truncates a single tool result if it exceeds PerResultChars.
// Returns the (possibly truncated) content and whether truncation occurred.
func (tb ToolBudget) EnforceResult(content string) (string, bool) {
	limit := tb.PerResultChars
	if limit <= 0 { limit = 50_000 }
	if len(content) <= limit {
		return content, false
	}

	preview := tb.PreviewChars
	if preview <= 0 { preview = 2_000 }

	// Keep head + tail with important-tail detection
	headSize := limit - preview - 100 // 100 for the truncation notice
	if headSize < 0 { headSize = limit / 2 }

	head := content[:headSize]
	var tail string
	if HasImportantTail(content, preview) {
		tail = content[len(content)-preview:]
	}

	var b strings.Builder
	b.WriteString(head)
	b.WriteString("\n\n[... truncated ")
	b.WriteString(formatBytes(len(content) - headSize - len(tail)))
	b.WriteString(" ...]\n\n")
	if tail != "" {
		b.WriteString(tail)
	}
	return b.String(), true
}

// EnforceTurn checks if the cumulative tool output for a turn exceeds PerTurnChars.
// Returns remaining budget. If <=0, further tool results should be summarized.
func (tb ToolBudget) EnforceTurn(usedChars int) int {
	limit := tb.PerTurnChars
	if limit <= 0 { limit = 200_000 }
	return limit - usedChars
}

