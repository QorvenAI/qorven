// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// RunContext tracks an active agent run for streaming/reaction event forwarding.
type RunContext struct {
	ChannelName       string
	ChatID            string
	MessageID         string
	Metadata          map[string]string
	Streaming         bool
	BlockReplyEnabled bool
	ToolStatusEnabled bool
	mu                sync.Mutex
	streamBuffer      string
	inToolPhase       bool
	stream            ChannelStream
	thinkingBuffer    string
	hasThinking       bool
	thinkingDone      bool
	tagParseSkipped   bool
}

// HandleAgentEvent routes agent lifecycle events to streaming/reaction channels.
func (m *Manager) HandleAgentEvent(eventType, runID string, payload map[string]any) {
	val, ok := m.runs.Load(runID)
	if !ok {
		return
	}
	rc := val.(*RunContext)

	m.mu.RLock()
	ch := m.channels[rc.ChannelName]
	m.mu.RUnlock()
	if ch == nil {
		return
	}

	ctx := context.Background()

	// Forward to StreamingChannel
	if sc, ok := ch.(StreamingChannel); ok && rc.Streaming {
		switch eventType {
		case "run.started":
			stream, err := sc.CreateStream(ctx, rc.ChatID, true)
			if err != nil {
				slog.Debug("stream start failed", "channel", rc.ChannelName, "error", err)
			} else {
				rc.mu.Lock()
				rc.stream = stream
				rc.mu.Unlock()
			}

		case "thinking":
			if !sc.ReasoningStreamEnabled() {
				break
			}
			content := extractString(payload, "content")
			if content != "" {
				rc.mu.Lock()
				rc.thinkingBuffer += content
				rc.hasThinking = true
				thinkText := rc.thinkingBuffer
				currentStream := rc.stream
				rc.mu.Unlock()
				if currentStream != nil {
					currentStream.Update(ctx, formatReasoningPreview(thinkText))
				}
			}

		case "tool.call":
			rc.mu.Lock()
			currentStream := rc.stream
			rc.stream = nil
			rc.inToolPhase = true
			rc.thinkingDone = false
			rc.thinkingBuffer = ""
			rc.hasThinking = false
			rc.tagParseSkipped = false
			rc.mu.Unlock()
			if currentStream != nil {
				currentStream.Stop(ctx)
			}

			toolName := extractString(payload, "name")
			if toolName != "" && rc.ToolStatusEnabled && !rc.Streaming {
				statusText := formatToolStatus(toolName)
				outMeta := copyRoutingMeta(rc.Metadata)
				outMeta["placeholder_update"] = "true"
				m.bus.PublishOutbound(OutboundMessage{
					RecipientID: rc.ChatID,
					Content:     statusText,
					Metadata:    outMeta,
				})
			}

		case "chunk":
			content := extractString(payload, "content")
			if content != "" {
				rc.mu.Lock()
				needNewStream := rc.inToolPhase
				if needNewStream {
					rc.streamBuffer = ""
					rc.inToolPhase = false
				}

				needTransition := rc.hasThinking && !rc.thinkingDone
				if needTransition {
					rc.thinkingDone = true
					rc.streamBuffer = ""
				}
				reasoningStream := rc.stream
				rc.mu.Unlock()

				if needTransition && reasoningStream != nil {
					reasoningStream.Stop(ctx)
				}

				if needNewStream || needTransition {
					stream, err := sc.CreateStream(ctx, rc.ChatID, false)
					if err != nil {
						slog.Debug("stream restart failed", "channel", rc.ChannelName, "error", err)
					} else {
						rc.mu.Lock()
						rc.stream = stream
						rc.mu.Unlock()
					}
				}

				rc.mu.Lock()
				rc.streamBuffer += content
				fullText := rc.streamBuffer
				currentStream := rc.stream
				rc.mu.Unlock()
				if currentStream != nil {
					currentStream.Update(ctx, fullText)
				}
			}

		case "run.completed":
			rc.mu.Lock()
			currentStream := rc.stream
			rc.stream = nil
			rc.mu.Unlock()
			if currentStream != nil {
				currentStream.Stop(ctx)
				sc.FinalizeStream(ctx, rc.ChatID, currentStream)
			}

		case "run.failed", "run.cancelled":
			rc.mu.Lock()
			currentStream := rc.stream
			rc.stream = nil
			rc.mu.Unlock()
			if currentStream != nil {
				currentStream.Stop(ctx)
			}
		}
	}

	// Forward to ReactionChannel
	if reactionCh, ok := ch.(ReactionChannel); ok {
		status := ""
		switch eventType {
		case "run.started":
			status = "thinking"
		case "tool.call":
			toolName := extractString(payload, "name")
			status = resolveToolReactionStatus(toolName)
		case "run.completed":
			status = "done"
		case "run.failed":
			status = "error"
		case "run.cancelled":
			status = "done"
		}
		if status != "" {
			if err := reactionCh.OnReactionEvent(ctx, rc.ChatID, rc.MessageID, status); err != nil {
				slog.Debug("reaction event failed", "channel", rc.ChannelName, "status", status, "error", err)
			}
		}
	}

	// Clean up on terminal events
	if eventType == "run.completed" || eventType == "run.failed" || eventType == "run.cancelled" {
		m.runs.Delete(runID)
	}
}

// RegisterRun registers a new run for event tracking.
func (m *Manager) RegisterRun(runID string, rc *RunContext) {
	m.runs.Store(runID, rc)
}

func extractString(payload map[string]any, key string) string {
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func copyRoutingMeta(src map[string]string) map[string]string {
	out := make(map[string]string)
	for _, k := range []string{"message_thread_id", "local_key", "group_id"} {
		if v := src[k]; v != "" {
			out[k] = v
		}
	}
	return out
}

var toolStatusMap = map[string]string{
	"read_file":  "📝 Reading file...",
	"write_file": "📝 Writing file...",
	"exec":       "⚡ Running code...",
	"web_search": "🔍 Searching the web...",
	"web_fetch":  "🔍 Fetching web content...",
	"browser":    "🌐 Browsing...",
	"spawn":      "👥 Delegating task...",
}

func formatToolStatus(toolName string) string {
	if s, ok := toolStatusMap[toolName]; ok {
		return s
	}
	if strings.HasPrefix(toolName, "mcp_") {
		return "🔌 Using external tool..."
	}
	return fmt.Sprintf("🔧 Running %s...", toolName)
}

func formatReasoningPreview(thinking string) string {
	if thinking == "" {
		return ""
	}
	const maxRunes = 4096
	text := "_Reasoning:_\n" + thinking
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes-3]) + "..."
	}
	return text
}

func resolveToolReactionStatus(toolName string) string {
	switch {
	case strings.HasPrefix(toolName, "web") || toolName == "browser":
		return "web"
	case toolName == "exec":
		return "coding"
	default:
		return "tool"
	}
}
