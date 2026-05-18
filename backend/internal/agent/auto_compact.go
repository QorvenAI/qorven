// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// AutoCompactConfig controls automatic compaction behavior.
type AutoCompactConfig struct {
	Enabled          bool    `json:"enabled"`
	ThresholdPercent float64 `json:"threshold_pct"` // compact when context usage exceeds this %
	MaxOutputTokens  int     `json:"max_output_tokens"`
}

var DefaultAutoCompact = AutoCompactConfig{
	Enabled: true, ThresholdPercent: 80, MaxOutputTokens: 20000,
}

// AutoCompactState tracks compaction state per session.
type AutoCompactState struct {
	Compacted          bool
	TurnCounter        int
	ConsecutiveFailures int
	LastCompactedAt    time.Time
}

// ShouldAutoCompact checks if the conversation needs compaction.
func ShouldAutoCompact(messages []providers.Message, contextWindow int, cfg AutoCompactConfig) bool {
	if !cfg.Enabled || contextWindow == 0 { return false }
	tokens := estimateTokens(messages)
	threshold := int(float64(contextWindow) * cfg.ThresholdPercent / 100)
	return tokens > threshold
}

// MicroCompact compresses tool results in-place without losing key info.
// Targets large tool outputs (>2000 chars) that are older than 3 turns.
func MicroCompact(messages []providers.Message, currentTurn int) []providers.Message {
	result := make([]providers.Message, len(messages))
	copy(result, messages)
	for i := range result {
		if result[i].Role != "tool" { continue }
		if currentTurn-i < 3 { continue } // keep recent tool results intact
		if len(result[i].Content) < 2000 { continue }
		// Truncate to first 500 + last 200 chars with indicator
		content := result[i].Content
		result[i].Content = content[:500] + "\n\n[... truncated " +
			fmt.Sprintf("%d", len(content)-700) + " chars ...]\n\n" + content[len(content)-200:]
	}
	return result
}

// SessionMemoryExtract extracts key information from a conversation for persistence.
// Runs as a background task after each complete agent response.
const SessionMemoryExtractPrompt = `Extract key information from this conversation that should persist across sessions.

Focus on:
1. Decisions made and their rationale
2. User preferences discovered
3. Technical context established
4. Action items or follow-ups mentioned
5. Problems identified and solutions applied

Format as a concise markdown summary. Skip routine exchanges.`

// ExtractSessionMemory runs the extraction sub-agent.
func (l *Loop) ExtractSessionMemory(ctx context.Context, sessionID string, messages []providers.Message) (string, error) {
	if len(messages) < 4 { return "", nil } // too short to extract

	// Build transcript
	var sb strings.Builder
	for _, m := range messages[func() int { if len(messages) > 20 { return len(messages)-20 }; return 0 }():] {
		fmt.Fprintf(&sb, "**%s:** %s\n\n", m.Role, truncateStr(m.Content, 500))
	}

	result, err := l.Run(ctx, RunRequest{
		AgentID:     "system",
		SessionID:   sessionID + "-memory",
		UserMessage: "Extract session memory from this conversation:\n\n" + sb.String(),
		NoTools:     true,
		NoPersist:   true,
	}, func(event StreamEvent) {})
	if err != nil { return "", err }

	slog.Info("session_memory.extracted", "session", sessionID, "len", len(result.Content))
	return result.Content, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}
