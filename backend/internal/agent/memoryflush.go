// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// Default memory flush prompts.
const (
	DefaultMemoryFlushPrompt = "Pre-compaction memory flush. " +
		"Append durable memories to memory/YYYY-MM-DD.md (create memory/ if needed). " +
		"If the file already exists, APPEND only — do not overwrite existing entries. " +
		"If nothing to store, reply with NO_REPLY."

	DefaultMemoryFlushSystemPrompt = "Pre-compaction memory flush turn. " +
		"The session is near auto-compaction; capture durable memories to disk. " +
		"Append to memory/YYYY-MM-DD.md only. " +
		"If the file already exists, append — do not overwrite. " +
		"You may reply, but usually NO_REPLY is correct."
)

// MemoryFlushConfig holds flush configuration.
type MemoryFlushConfig struct {
	Enabled      bool
	Prompt       string
	SystemPrompt string
}

// DefaultMemoryFlushConfig returns default flush settings.
func DefaultMemoryFlushConfig() *MemoryFlushConfig {
	return &MemoryFlushConfig{
		Enabled:      true,
		Prompt:       DefaultMemoryFlushPrompt,
		SystemPrompt: DefaultMemoryFlushSystemPrompt,
	}
}

// MemoryFlusher handles pre-compaction memory flush.
type MemoryFlusher struct {
	provider    providers.Provider
	model       string
	toolReg     ToolRegistry
	memStore    MemoryStore
	sessionID   string
	agentID     string
	workspace   string
}

// ToolRegistry interface for tool execution.
type ToolRegistry interface {
	Execute(ctx context.Context, name string, args map[string]any) *ToolResultMF
	ProviderDefs() []providers.ToolDefinition
}

// ToolResultMF from tool execution (memory flush specific).
type ToolResultMF struct {
	ForLLM  string
	IsError bool
}

// MemoryStore interface for memory persistence.
type MemoryStore interface {
	GetDocument(ctx context.Context, agentID, userID, path string) (string, error)
	PutDocument(ctx context.Context, agentID, userID, path, content string) error
	IndexDocument(ctx context.Context, agentID, userID, path string) error
}

// NewMemoryFlusher creates a new flusher.
func NewMemoryFlusher(provider providers.Provider, model string, toolReg ToolRegistry, memStore MemoryStore, sessionID, agentID, workspace string) *MemoryFlusher {
	return &MemoryFlusher{
		provider:  provider,
		model:     model,
		toolReg:   toolReg,
		memStore:  memStore,
		sessionID: sessionID,
		agentID:   agentID,
		workspace: workspace,
	}
}

// Run executes a memory flush turn.
func (f *MemoryFlusher) Run(ctx context.Context, history []providers.Message, summary string, cfg *MemoryFlushConfig) error {
	if cfg == nil {
		cfg = DefaultMemoryFlushConfig()
	}
	if !cfg.Enabled {
		return nil
	}

	slog.Info("memory flush: starting", "session", f.sessionID)

	flushCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	// Replace YYYY-MM-DD placeholder with today's date
	today := time.Now().Format("2006-01-02")
	flushPrompt := strings.ReplaceAll(cfg.Prompt, "YYYY-MM-DD", today)
	flushSystemPrompt := strings.ReplaceAll(cfg.SystemPrompt, "YYYY-MM-DD", today)

	var messages []providers.Message

	// System prompt
	messages = append(messages, providers.Message{
		Role:    "system",
		Content: flushSystemPrompt,
	})

	// Include conversation summary for context
	if summary != "" {
		messages = append(messages, providers.Message{
			Role:    "user",
			Content: fmt.Sprintf("[Previous conversation summary]\n%s", summary),
		})
		messages = append(messages, providers.Message{
			Role:    "assistant",
			Content: "Understood.",
		})
	}

	// Include recent history (last 10 messages for context)
	recentHistory := history
	if len(recentHistory) > 10 {
		recentHistory = recentHistory[len(recentHistory)-10:]
	}
	for _, msg := range recentHistory {
		if msg.Role == "user" || msg.Role == "assistant" {
			messages = append(messages, msg)
		}
	}

	// Flush prompt
	messages = append(messages, providers.Message{
		Role:    "user",
		Content: flushPrompt,
	})

	// Get tool definitions
	var toolDefs []providers.ToolDefinition
	if f.toolReg != nil {
		toolDefs = f.toolReg.ProviderDefs()
	}

	// Run LLM iteration loop (max 5 iterations for flush)
	for range 5 {
		resp, err := f.provider.Chat(flushCtx, providers.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
			Model:    f.model,
			Options: map[string]any{
				"max_tokens":  4096,
				"temperature": 0.3,
			},
		})
		if err != nil {
			slog.Warn("memory flush: LLM call failed", "error", err)
			f.extractiveFallback(flushCtx, history, "LLM error")
			return err
		}

		// No tool calls → done
		if len(resp.ToolCalls) == 0 {
			content := SanitizeAssistantContent(resp.Content)
			if IsSilentReply(content) {
				slog.Info("memory flush: NO_REPLY, trying extractive fallback")
				f.extractiveFallback(flushCtx, history, "NO_REPLY")
			} else if content != "" {
				slog.Info("memory flush: completed with response", "content_len", len(content))
			}
			break
		}

		// Process tool calls
		assistantMsg := providers.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		for _, tc := range resp.ToolCalls {
			slog.Info("memory flush: tool call", "tool", tc.Name)

			var result *ToolResultMF
			if f.toolReg != nil {
				result = f.toolReg.Execute(flushCtx, tc.Name, tc.Arguments)
			} else {
				result = &ToolResultMF{ForLLM: "Tool registry not available", IsError: true}
			}

			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    result.ForLLM,
				ToolCallID: tc.ID,
			})
		}
	}

	slog.Info("memory flush: completed", "session", f.sessionID)
	return nil
}

// extractiveFallback runs regex-based extraction when LLM flush fails.
func (f *MemoryFlusher) extractiveFallback(ctx context.Context, history []providers.Message, reason string) {
	if f.memStore == nil {
		return
	}

	// Limit input to last 20 messages
	if len(history) > 20 {
		history = history[len(history)-20:]
	}

	extracted := ExtractiveMemoryFallback(history)
	if extracted == "" {
		slog.Info("memory flush: extractive fallback produced no content", "session", f.sessionID, "reason", reason)
		return
	}

	docPath := fmt.Sprintf("memory/%s-auto-extract.md", time.Now().Format("2006-01-02"))

	// Append to existing document if it exists
	existing, err := f.memStore.GetDocument(ctx, f.agentID, "", docPath)
	if err == nil && existing != "" {
		extracted = existing + "\n\n---\n\n" + extracted
	}

	if err := f.memStore.PutDocument(ctx, f.agentID, "", docPath, extracted); err != nil {
		slog.Warn("memory flush: extractive fallback write failed", "session", f.sessionID, "error", err)
		return
	}

	if err := f.memStore.IndexDocument(ctx, f.agentID, "", docPath); err != nil {
		slog.Warn("memory flush: extractive fallback index failed", "session", f.sessionID, "error", err)
	}

	slog.Info("memory flush: extractive fallback saved", "session", f.sessionID, "reason", reason, "path", docPath)
}
