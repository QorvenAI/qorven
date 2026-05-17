// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/qorvenai/qorven/internal/providers"
)

// IntentType classifies user messages while agent is busy.
type IntentType string

const (
	IntentStatusQuery IntentType = "status_query"
	IntentCancel      IntentType = "cancel"
	IntentSteer       IntentType = "steer"
	IntentNewTask     IntentType = "new_task"
)

// cancelKeywords for fast-path detection of obvious cancel intents.
// Only matched on very short messages (≤ 15 runes) to avoid false positives.
// Includes Vietnamese, Chinese for international support.
var cancelKeywords = []string{
	"stop", "cancel", "abort", "thôi", "dừng", "hủy", "取消", "停",
	"nevermind", "never mind",
}

// QuickClassify does fast keyword-based classification for short messages.
// Only messages ≤ 15 runes are fast-pathed; longer messages go to LLM.
// Cancel keywords require whole-word match to avoid false positives.
func QuickClassify(msg string) (IntentType, bool) {
	lower := strings.ToLower(strings.TrimSpace(msg))
	if utf8.RuneCountInString(lower) > 15 {
		return "", false
	}
	// Exact "?" → status query
	if lower == "?" {
		return IntentStatusQuery, true
	}
	for _, kw := range cancelKeywords {
		if containsWholeWord(lower, kw) {
			return IntentCancel, true
		}
	}
	return "", false
}

// containsWholeWord checks if s contains kw as a whole word (not a substring).
// Word boundaries are: start/end of string, spaces, punctuation.
func containsWholeWord(s, kw string) bool {
	idx := strings.Index(s, kw)
	if idx < 0 {
		return false
	}
	// Check left boundary
	if idx > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:idx])
		if isWordChar(r) {
			return false
		}
	}
	// Check right boundary
	end := idx + len(kw)
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if isWordChar(r) {
			return false
		}
	}
	return true
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

const intentSystemPrompt = `You are an intent classifier. The user has sent a message while the AI assistant is busy processing a previous request.

Classify the user's intent into exactly ONE of these categories:
- status_query: The user is asking about progress, status, or what the assistant is currently doing
- cancel: The user wants to stop or cancel the current task
- steer: The user wants to add instructions or redirect the current task
- new_task: The user is sending a new unrelated request or message

Respond with ONLY the category name, nothing else.`

// ClassifyIntent uses keyword fast-path then LLM fallback.
func ClassifyIntent(ctx context.Context, provider providers.Provider, model, msg string) IntentType {
	if intent, ok := QuickClassify(msg); ok {
		return intent
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{Role: "system", Content: intentSystemPrompt},
			{Role: "user", Content: msg},
		},
		Model:   model,
		Options: map[string]any{"max_tokens": 20, "temperature": 0.0},
	})
	if err != nil {
		return IntentNewTask
	}

	result := strings.ToLower(strings.TrimSpace(resp.Content))
	switch {
	case strings.Contains(result, "status_query"):
		return IntentStatusQuery
	case strings.Contains(result, "cancel"):
		return IntentCancel
	case strings.Contains(result, "steer"):
		return IntentSteer
	default:
		return IntentNewTask
	}
}

// AgentActivityStatus tracks current agent activity for status queries.
type AgentActivityStatus struct {
	Phase     string    // "thinking", "tool_exec", "compacting"
	Tool      string    // current tool name (if in tool_exec phase)
	Iteration int       // current iteration number
	StartedAt time.Time // when the run started
}

// FormatStatusReply builds a user-friendly status response.
func FormatStatusReply(status *AgentActivityStatus) string {
	if status == nil {
		return "I'm working on your request..."
	}

	elapsed := time.Since(status.StartedAt).Round(time.Second)
	phase := formatPhase(status.Phase, status.Tool)

	return fmt.Sprintf("%s (iteration %d, %v elapsed)", phase, status.Iteration, elapsed)
}

// formatPhase returns a human-readable phase description.
func formatPhase(phase, tool string) string {
	switch phase {
	case "thinking":
		return "Thinking..."
	case "tool_exec":
		if tool != "" {
			return fmt.Sprintf("Running %s...", formatToolLabel(tool))
		}
		return "Executing tools..."
	case "compacting":
		return "Compacting context..."
	default:
		return "Processing..."
	}
}

// formatToolLabel returns a user-friendly label for a tool name.
func formatToolLabel(tool string) string {
	switch {
	case strings.HasPrefix(tool, "web"):
		return "web search"
	case tool == "exec":
		return "code execution"
	case tool == "browser":
		return "browser"
	case tool == "spawn":
		return "delegation"
	case strings.HasPrefix(tool, "memory"):
		return "memory"
	case strings.HasPrefix(tool, "file") || strings.HasPrefix(tool, "read") || strings.HasPrefix(tool, "write"):
		return "file operations"
	default:
		return tool
	}
}
