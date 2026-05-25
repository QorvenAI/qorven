// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// TypeSynthesis identifies a cortex-synthesized memory.
const TypeSynthesis = "synthesis"

// SynthesisEntry is one extracted fact or decision from a session.
type SynthesisEntry struct {
	Type    string `json:"type"`    // "fact" | "decision" | "insight"
	Content string `json:"content"` // the synthesized text
}

// RunCortexSynthesis performs an async post-session knowledge extraction.
// It asks the LLM to derive key facts, decisions, and insights from the
// conversation history, then stores each as a synthesis memory.
//
// Call this in a goroutine — it should never block the agent's response.
func RunCortexSynthesis(
	ctx context.Context,
	provider providers.Provider,
	store *Store,
	tenantID, agentID, sessionID string,
	history []providers.Message,
) {
	if provider == nil || store == nil || len(history) < 4 {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build a compact transcript (last 20 turns max, ~6000 chars)
	transcript := buildTranscript(history, 20, 6000)
	if transcript == "" {
		return
	}

	prompt := `You are a knowledge extractor. Read the conversation below and output a JSON array of the most important facts, decisions, and insights worth remembering for future sessions. Focus on:
- Concrete decisions made (e.g. "team decided to use PostgreSQL for X")
- Key facts discovered (e.g. "the API rate limit is 1000 req/min")
- Actionable insights (e.g. "users prefer short responses over detailed ones")

Return ONLY valid JSON — no prose, no markdown:
[{"type":"fact"|"decision"|"insight","content":"..."}]

Maximum 5 entries. If nothing is worth extracting, return [].

CONVERSATION:
` + transcript

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{"temperature": 0.1, "max_tokens": 400},
	})
	if err != nil {
		slog.Debug("cortex.synthesis.llm_error", "session", sessionID, "error", err)
		return
	}

	entries, err := parseSynthesisEntries(resp.Content)
	if err != nil || len(entries) == 0 {
		return
	}

	for _, e := range entries {
		memType := TypeSynthesis
		if e.Type == "decision" {
			memType = TypeDecision
		} else if e.Type == "fact" {
			memType = TypeFact
		}
		_, saveErr := store.Save(ctx, tenantID, Memory{
			AgentID:    agentID,
			Type:       memType,
			Content:    e.Content,
			Source:     sessionID,
			SourceType: TypeSynthesis,
			Importance: 0.7,
			Tags:       []string{"cortex", e.Type},
		})
		if saveErr != nil {
			slog.Debug("cortex.synthesis.save_error", "session", sessionID, "error", saveErr)
		}
	}

	slog.Info("cortex.synthesis.complete", "session", sessionID, "agent", agentID, "entries", len(entries))
}

func buildTranscript(history []providers.Message, maxTurns, maxChars int) string {
	start := 0
	if len(history) > maxTurns {
		start = len(history) - maxTurns
	}
	var sb strings.Builder
	for _, msg := range history[start:] {
		role := "User"
		if msg.Role == "assistant" {
			role = "Assistant"
		} else if msg.Role != "user" {
			continue
		}
		line := role + ": " + truncateContent(msg.Content, 300) + "\n"
		if sb.Len()+len(line) > maxChars {
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}

func parseSynthesisEntries(raw string) ([]SynthesisEntry, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var entries []SynthesisEntry
	err := json.Unmarshal([]byte(raw), &entries)
	return entries, err
}

func truncateContent(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
