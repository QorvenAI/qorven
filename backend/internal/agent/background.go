// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/llm"
)

// BackgroundTasks generates title, tags, and follow-ups after agent responds.
type BackgroundTasks struct {
	provider llm.Provider
	model    string // cheap model for background tasks
}

func NewBackgroundTasks(provider llm.Provider, model string) *BackgroundTasks {
	// Leave model as "" — the provider will use its own default.
	return &BackgroundTasks{provider: provider, model: model}
}

type BackgroundResult struct {
	Title     string   `json:"title,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	FollowUps []string `json:"follow_ups,omitempty"`
}

// GenerateTitle creates a 3-5 word title with emoji.
func (bt *BackgroundTasks) GenerateTitle(ctx context.Context, userMsg, assistantMsg string) string {
	resp, err := bt.provider.Chat(ctx, llm.ChatRequest{
		Model: bt.model,
		Messages: []llm.Message{
			{Role: "system", Content: "Generate a concise 3-5 word title with one emoji for this conversation. Output ONLY the title, nothing else."},
			{Role: "user", Content: userMsg},
			{Role: "assistant", Content: assistantMsg[:min(len(assistantMsg), 500)]},
		},
		MaxTokens: 20,
	})
	if err != nil { slog.Warn("bg.title.error", "error", err); return "" }
	return strings.TrimSpace(resp.Content)
}

// GenerateTags creates 1-3 broad + 1-3 specific tags.
func (bt *BackgroundTasks) GenerateTags(ctx context.Context, userMsg, assistantMsg string) []string {
	resp, err := bt.provider.Chat(ctx, llm.ChatRequest{
		Model: bt.model,
		Messages: []llm.Message{
			{Role: "system", Content: `Generate tags for this conversation. Output JSON: {"tags":["tag1","tag2","tag3"]}. Include 1-3 broad + 1-3 specific tags.`},
			{Role: "user", Content: userMsg},
			{Role: "assistant", Content: assistantMsg[:min(len(assistantMsg), 500)]},
		},
		MaxTokens: 60,
	})
	if err != nil { slog.Warn("bg.tags.error", "error", err); return nil }
	var result struct{ Tags []string `json:"tags"` }
	json.Unmarshal([]byte(resp.Content), &result)
	return result.Tags
}

// GenerateFollowUps suggests 3-5 follow-up questions.
func (bt *BackgroundTasks) GenerateFollowUps(ctx context.Context, userMsg, assistantMsg string) []string {
	resp, err := bt.provider.Chat(ctx, llm.ChatRequest{
		Model: bt.model,
		Messages: []llm.Message{
			{Role: "system", Content: `Suggest 3 follow-up questions the user might ask next. Output JSON: {"follow_ups":["Q1?","Q2?","Q3?"]}. Write from user's perspective.`},
			{Role: "user", Content: userMsg},
			{Role: "assistant", Content: assistantMsg[:min(len(assistantMsg), 500)]},
		},
		MaxTokens: 100,
	})
	if err != nil { slog.Warn("bg.followups.error", "error", err); return nil }
	var result struct{ FollowUps []string `json:"follow_ups"` }
	json.Unmarshal([]byte(resp.Content), &result)
	return result.FollowUps
}
