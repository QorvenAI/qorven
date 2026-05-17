// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// FlushConfig controls proactive memory extraction.
type FlushConfig struct {
	Enabled    bool
	MaxTurns   int           // max LLM iterations for flush (default 5)
	Timeout    time.Duration // max time for flush (default 90s)
	MinHistory int           // minimum messages to trigger flush (default 4)
}

// DefaultFlushConfig returns sensible defaults.
func DefaultFlushConfig() FlushConfig {
	return FlushConfig{
		Enabled:    true,
		MaxTurns:   5,
		Timeout:    90 * time.Second,
		MinHistory: 4,
	}
}

// FlushMemories extracts important information from a conversation before
// the session expires. Creates a temporary agent with memory tools to
// review the conversation and save relevant facts.
//
// memory persistence branches.
func FlushMemories(
	ctx context.Context,
	provider providers.Provider,
	model string,
	history []providers.Message,
	curated *CuratedStore,
	cfg FlushConfig,
) error {
	if !cfg.Enabled {
		return nil
	}
	if len(history) < cfg.MinHistory {
		slog.Debug("memory.flush: skipping, too few messages", "count", len(history))
		return nil
	}

	flushCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	// Read current memory state so the flush agent doesn't overwrite newer entries
	currentMemory := curated.ForSystemPrompt("memory")
	currentUser := curated.ForSystemPrompt("user")

	// Build flush prompt
	prompt := buildFlushPrompt(currentMemory, currentUser)

	// Build conversation context (last 20 messages for efficiency)
	recentHistory := history
	if len(recentHistory) > 20 {
		recentHistory = recentHistory[len(recentHistory)-20:]
	}

	messages := []providers.Message{
		{Role: "system", Content: prompt},
	}
	// Add conversation as context
	for _, msg := range recentHistory {
		if msg.Role == "system" {
			continue
		}
		messages = append(messages, providers.Message{
			Role:    msg.Role,
			Content: truncateFlush(msg.Content, 2000),
		})
	}
	// Flush instruction
	messages = append(messages, providers.Message{
		Role: "user",
		Content: "Review the conversation above. Save any important facts, preferences, " +
			"or decisions to memory. Do NOT respond to the user. Just extract and save, then stop.",
	})

	// Run flush (simple — no tool calls, just extract text and save directly)
	resp, err := provider.Chat(flushCtx, providers.ChatRequest{
		Model:    model,
		Messages: messages,
		Options:  map[string]any{"temperature": 0.3, "max_tokens": 1000},
	})
	if err != nil {
		slog.Warn("memory.flush: LLM call failed", "error", err)
		// Fall back to extractive method
		extractiveFallback(curated, history)
		return nil
	}

	// Parse the response for memory entries
	saved := parseFlushResponse(resp.Content, curated)
	slog.Info("memory.flush: completed", "saved", saved, "history_len", len(history))
	return nil
}

func buildFlushPrompt(currentMemory, currentUser string) string {
	var b strings.Builder
	b.WriteString("You are reviewing a conversation to extract durable information.\n\n")
	b.WriteString("Save important facts using these categories:\n")
	b.WriteString("- 'memory': environment facts, project conventions, tool quirks, lessons learned\n")
	b.WriteString("- 'user': who the user is — name, role, preferences, communication style\n\n")
	b.WriteString("IMPORTANT: Here is the current memory state. Do NOT overwrite or remove entries ")
	b.WriteString("unless the conversation reveals something that genuinely supersedes them.\n\n")
	if currentMemory != "" {
		b.WriteString(currentMemory + "\n\n")
	}
	if currentUser != "" {
		b.WriteString(currentUser + "\n\n")
	}
	b.WriteString("Format your response as:\nMEMORY: <fact to save>\nUSER: <user info to save>\n")
	b.WriteString("If nothing is worth saving, respond with: NOTHING_TO_SAVE\n")
	return b.String()
}

func parseFlushResponse(response string, curated *CuratedStore) int {
	saved := 0
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MEMORY:") {
			content := strings.TrimSpace(strings.TrimPrefix(line, "MEMORY:"))
			if content != "" && len(content) > 10 {
				if err := curated.Add("memory", content); err == nil {
					saved++
				}
			}
		} else if strings.HasPrefix(line, "USER:") {
			content := strings.TrimSpace(strings.TrimPrefix(line, "USER:"))
			if content != "" && len(content) > 10 {
				if err := curated.Add("user", content); err == nil {
					saved++
				}
			}
		}
	}
	return saved
}

// extractiveFallback uses regex patterns to extract facts when LLM flush fails.
// From Qorven's extractive_memory.go — last resort, no LLM needed.
func extractiveFallback(curated *CuratedStore, history []providers.Message) {
	var facts []string

	for _, msg := range history {
		if msg.Role != "user" {
			continue
		}
		content := msg.Content

		// Extract "my name is X" patterns
		if strings.Contains(strings.ToLower(content), "my name is") ||
			strings.Contains(strings.ToLower(content), "i'm called") ||
			strings.Contains(strings.ToLower(content), "call me") {
			facts = append(facts, fmt.Sprintf("User said: %s", truncateFlush(content, 200)))
		}

		// Extract preference patterns
		if strings.Contains(strings.ToLower(content), "i prefer") ||
			strings.Contains(strings.ToLower(content), "i always") ||
			strings.Contains(strings.ToLower(content), "i never") ||
			strings.Contains(strings.ToLower(content), "remember that") {
			facts = append(facts, fmt.Sprintf("User preference: %s", truncateFlush(content, 200)))
		}
	}

	for _, fact := range facts {
		curated.Add("user", fact) // ignore errors — best effort
	}

	if len(facts) > 0 {
		slog.Info("memory.flush.extractive", "saved", len(facts))
	}
}

func truncateFlush(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
