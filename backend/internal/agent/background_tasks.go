// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// BackgroundTasks generates title, tags, and follow-ups after the main response.
// Mentor: use gemini-2.0-flash for all tasks, run 3 goroutines in parallel.
func (l *Loop) RunBackgroundTasks(
	ctx context.Context,
	provider providers.Provider,
	agentID, sessionID, model string,
	userMessage, assistantResponse string,
	isFirstMessage bool,
	onEvent func(StreamEvent),
) {
	// Use a short timeout — these are background tasks
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Use BackgroundModel (cheap/fast) if configured — saves expensive tokens for the main loop.
	// Falls back to the agent's primary model if not set.
	bgModel := l.BackgroundModel
	if bgModel == "" {
		bgModel = model
	}

	var wg sync.WaitGroup

	// Title: only on first exchange
	if isFirstMessage {
		wg.Add(1)
		go func() {
			defer wg.Done()
			title := generateTitle(ctx, provider, bgModel, userMessage, assistantResponse)
			if title != "" {
				onEvent(TitleEvent(title))
				// Save to DB
				if l.agentStore != nil && l.agentStore.Pool() != nil {
					l.agentStore.Pool().Exec(ctx,
						`UPDATE sessions SET title = $1 WHERE id = $2`,
						title, sessionID)
				}
				slog.Info("background.title", "session", sessionID, "title", title)
			}
		}()
	}

	// Tags: only on first exchange
	if isFirstMessage {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tags := generateTags(ctx, provider, bgModel, userMessage, assistantResponse)
			if len(tags) > 0 {
				onEvent(TagsEvent(tags))
				slog.Info("background.tags", "session", sessionID, "tags", tags)
			}
		}()
	}

	// Follow-ups: every exchange
	wg.Add(1)
	go func() {
		defer wg.Done()
		followUps := generateFollowUps(ctx, provider, bgModel, userMessage, assistantResponse)
		if len(followUps) > 0 {
			onEvent(FollowUpEvent(followUps))
		}
	}()

	wg.Wait()
}

func generateTitle(ctx context.Context, provider providers.Provider, model, userMsg, assistantMsg string) string {
	prompt := fmt.Sprintf(`Generate a concise 3-5 word title with one emoji for this conversation.
Output ONLY JSON: {"title": "your title here"}

User: %s
Assistant: %s`, truncate(userMsg, 500), truncate(assistantMsg, 500))

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:   model,
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{"max_tokens": 50, "temperature": 0.7},
	})
	if err != nil {
		return ""
	}
	return extractJSONField(resp.Content, "title")
}

func generateTags(ctx context.Context, provider providers.Provider, model, userMsg, assistantMsg string) []string {
	prompt := fmt.Sprintf(`Generate 1-3 tags categorizing this conversation.
Output ONLY JSON: {"tags": ["tag1", "tag2"]}

User: %s
Assistant: %s`, truncate(userMsg, 500), truncate(assistantMsg, 500))

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:   model,
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{"max_tokens": 50, "temperature": 0.5},
	})
	if err != nil {
		return nil
	}
	return extractJSONStringArray(resp.Content, "tags")
}

func generateFollowUps(ctx context.Context, provider providers.Provider, model, userMsg, assistantMsg string) []string {
	// Mentor: generate from last exchange only (not full conversation)
	prompt := fmt.Sprintf(`Suggest 3 follow-up questions the user might ask next.
Output ONLY JSON: {"follow_ups": ["Question 1?", "Question 2?", "Question 3?"]}

User: %s
Assistant: %s`, truncate(userMsg, 500), truncate(assistantMsg, 300))

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:   model,
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{"max_tokens": 150, "temperature": 0.8},
	})
	if err != nil {
		return nil
	}
	return extractJSONStringArray(resp.Content, "follow_ups")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractJSONField(text, field string) string {
	// Find JSON in response (may have extra text around it)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text[start:end+1]), &m); err != nil {
		return ""
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

func extractJSONStringArray(text, field string) []string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < 0 || end <= start {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text[start:end+1]), &m); err != nil {
		return nil
	}
	arr, ok := m[field].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}
