// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/ssestream"
)

// widgetRef is a media widget saved to disk so TUI can display a path.
type widgetRef struct {
	widgetType string // "image", "audio", "video"
	path       string // /tmp/qorven-*.{ext}
	label      string // human-readable summary (prompt, text, etc.)
}

// Messages emitted by the streaming pipeline. Consumed by Update in app.go.
type streamDoneMsg struct {
	content string
	tools   []toolEvent
	widgets []widgetRef
}
type streamErrorMsg struct{ err string }

type toolEvent struct {
	name     string
	args     string
	result   string
	status   string // running, done, error
	expanded bool   // user-toggled expanded view (shows full result)
}

// streamCancelMsg is sent when the user cancels an active stream via Esc.
type streamCancelMsg struct{}

// streamFrame is the wire shape we decode off each SSE frame. It is a
// superset of the legacy `{type, data}` and the canonical
// `{type, properties}` envelope; we read whichever is present. We also
// tolerate the OpenAI-style `choices[].delta.content` shape used by the
// chat-completions SSE path.
type streamFrame struct {
	// Envelope discriminator (works for both shapes).
	Type string `json:"type"`

	// Canonical envelope payload.
	Properties json.RawMessage `json:"properties,omitempty"`

	// Legacy shape — same slot as `data`.
	Data json.RawMessage `json:"data,omitempty"`

	// OpenAI-style streaming chunk.
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices,omitempty"`

	// Envelope identity (optional — used for de-duplication).
	ID string `json:"id,omitempty"`
}

// sendMessageCtx posts the user message and streams the response back. It
// converges two wire shapes the gateway emits — a legacy per-tool-name
// event and the canonical agent.progress envelope:
//
//   - legacy: {"type":"tool_start","data":{"name":"exec",...}}
//   - canonical: {"type":"agent.progress","properties":{"kind":"tool_start",...}}
//
// The canonical envelope is preferred when both appear; we de-dupe by
// envelope ID AND by `(type, argsHash)` fingerprint when no ID is present.
// The context is cancelled when the user presses Esc during streaming.
func (m *Model) sendMessageCtx(ctx context.Context, text string) tea.Cmd {
	return func() tea.Msg {
		if m.api == nil || m.api.http == nil {
			return streamErrorMsg{err: "not connected to gateway"}
		}

		body := map[string]any{
			"session_id": m.sessionID,
			"agent_id":   m.agentID,
			"message":    text,
			"stream":     true,
		}
		if m.thinkingLevel != "" && m.thinkingLevel != "off" {
			body["thinking_level"] = m.thinkingLevel
		}

		resp, err := m.api.http.PostRaw("/v1/chat/completions", body)
		if err != nil {
			return streamErrorMsg{err: err.Error()}
		}
		stream := ssestream.NewStreamReader[streamFrame](resp.Body)
		defer stream.Close()

		if resp.StatusCode >= 400 {
			return streamErrorMsg{err: fmt.Sprintf("API error %d", resp.StatusCode)}
		}

		var (
			accumulated strings.Builder
			tools       []toolEvent
			widgets     []widgetRef
			seenIDs     = make(map[string]struct{})
			seenFP      = make(map[string]struct{}, seenFPMax)
			seenFPOrd   []string
		)

		for stream.Next() {
			// Check for cancellation on every frame.
			select {
			case <-ctx.Done():
				partial := strings.TrimSpace(accumulated.String())
				if partial != "" {
					return streamDoneMsg{content: partial + "\n\n*(cancelled)*", tools: tools, widgets: widgets}
				}
				return streamDoneMsg{content: "*(cancelled)*", tools: tools, widgets: widgets}
			default:
			}

			frame := stream.Current()
			fp := fingerprint(frame)
			if frame.ID != "" {
				if _, ok := seenIDs[frame.ID]; ok {
					continue
				}
				if _, ok := seenFP[fp]; ok {
					seenIDs[frame.ID] = struct{}{}
					continue
				}
				seenIDs[frame.ID] = struct{}{}
				seenFPInsert(seenFP, &seenFPOrd, fp)
			} else {
				if _, ok := seenFP[fp]; ok {
					continue
				}
				seenFPInsert(seenFP, &seenFPOrd, fp)
			}
			for _, c := range frame.Choices {
				if c.Delta.Content != "" {
					accumulated.WriteString(c.Delta.Content)
				}
			}
			switch apievents.Type(frame.Type) {
			case apievents.TypeMessagePartUpdated:
				var p apievents.MessagePartUpdatedProps
				if err := json.Unmarshal(payload(frame), &p); err == nil && p.Kind == "text" {
					accumulated.Write(extractTextPayload(p.Payload))
				}
			case apievents.TypeAgentProgress:
				var p apievents.AgentProgressProps
				if err := json.Unmarshal(payload(frame), &p); err == nil {
					tools = applyAgentProgress(tools, p)
				}
			case apievents.TypeSessionError:
				var p apievents.SessionErrorProps
				if err := json.Unmarshal(payload(frame), &p); err == nil && p.Severity == "fatal" {
					return streamErrorMsg{err: p.Message}
				}
			}
			switch frame.Type {
			case "text_delta":
				var d struct{ Delta string `json:"delta"` }
				if err := json.Unmarshal(payload(frame), &d); err == nil {
					accumulated.WriteString(d.Delta)
				}
			case "part":
				handleLegacyPart(&tools, frame)
			case "tool_start":
				var d struct{ Name, Input string }
				if err := json.Unmarshal(payload(frame), &d); err == nil && d.Name != "" {
					tools = append(tools, toolEvent{name: d.Name, args: shorten(d.Input, 60), status: "running"})
				}
			case "tool_result", "tool_end":
				if len(tools) > 0 {
					tools[len(tools)-1].status = "done"
				}
			case "widget":
				if ref, ok := saveWidgetToTmp(payload(frame)); ok {
					widgets = append(widgets, ref)
				}
			}
		}
		if err := stream.Err(); err != nil {
			select {
			case <-ctx.Done():
				// Cancelled — not a real error.
				partial := strings.TrimSpace(accumulated.String())
				if partial != "" {
					return streamDoneMsg{content: partial + "\n\n*(cancelled)*", tools: tools, widgets: widgets}
				}
				return streamDoneMsg{content: "*(cancelled)*", tools: tools, widgets: widgets}
			default:
				return streamErrorMsg{err: err.Error()}
			}
		}
		return streamDoneMsg{content: accumulated.String(), tools: tools, widgets: widgets}
	}
}

// loadHomeAsync fires a background goroutine to populate the home dashboard.
func (m *Model) loadHomeAsync() tea.Cmd {
	api := m.api
	return func() tea.Msg {
		return homeLoadedMsg{data: api.getHomeDashboard()}
	}
}

// loadAgentHistoryAsync fetches historical messages and discussions for an agent.
func (m *Model) loadAgentHistoryAsync(agentID string) tea.Cmd {
	api := m.api
	return func() tea.Msg {
		msgs, err := api.listAgentMessages(agentID, 100)
		if err != nil {
			return agentHistoryLoadedMsg{agentID: agentID}
		}
		// Reverse: API returns newest-first, we want oldest-first for display.
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
		discs, _ := api.listDiscussions(agentID)
		return agentHistoryLoadedMsg{agentID: agentID, messages: msgs, discussions: discs}
	}
}

// pingGatewayAsync measures gateway roundtrip latency.
func (m *Model) pingGatewayAsync() tea.Cmd {
	api := m.api
	return func() tea.Msg {
		start := time.Now()
		status := api.getStatus()
		latency := time.Since(start)
		ok := status["status"] == "online" || status["status"] == "ok" || status["status"] != ""
		ms := int(latency.Milliseconds())
		latStr := fmt.Sprintf("%dms", ms)
		return pingResultMsg{ok: ok, latency: latStr}
	}
}

// scheduleNextPing returns a command that fires pingTickMsg after 30s.
func (m *Model) scheduleNextPing() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return pingTickMsg{}
	})
}

// subscribeRealTime connects to the gateway event stream and delivers one
// realTimeMsg at a time back to the TUI's Update loop. The cmd chain
// continues by re-subscribing after each message (or after a brief pause
// on error), giving a continuous stream without goroutine leaks.
func (m *Model) subscribeRealTime() tea.Cmd {
	api := m.api
	return func() tea.Msg {
		// Prefer daemon stream (has plan/task events). Notifications stream is
		// a bonus when available. Both fall back gracefully to rtConnectedMsg.
		resp, err := api.http.GetRaw("/v1/daemon/stream")
		if err != nil {
			return rtConnectedMsg{}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return rtConnectedMsg{}
		}

		stream := ssestream.NewStreamReader[streamFrame](resp.Body)
		defer stream.Close()

		for stream.Next() {
			frame := stream.Current()
			switch frame.Type {
			case "channel_message":
				var d struct {
					Channel string `json:"channel"`
					Content string `json:"content"`
					Author  string `json:"author"`
				}
				if json.Unmarshal(payload(frame), &d) == nil {
					badge := ""
					if d.Channel != "" {
						badge = "[" + d.Channel + "] "
					}
					return realTimeMsg{kind: "channel_message", payload: badge + d.Author + ": " + d.Content}
				}
			case "notification":
				var d struct {
					Title string `json:"title"`
					Body  string `json:"body"`
				}
				if json.Unmarshal(payload(frame), &d) == nil {
					return realTimeMsg{kind: "notification", payload: d.Title + " — " + d.Body}
				}
			case "plan_pending", "plan_created":
				var d struct {
					Title string `json:"title"`
					ID    string `json:"id"`
				}
				if json.Unmarshal(payload(frame), &d) == nil {
					title := d.Title
					if title == "" {
						title = d.ID
					}
					return realTimeMsg{kind: "plan_pending", payload: title}
				}
			case "task_update", "task_assigned", "task_completed", "task_failed":
				var d struct {
					Title  string `json:"title"`
					Status string `json:"status"`
					ID     string `json:"id"`
				}
				if json.Unmarshal(payload(frame), &d) == nil {
					status := d.Status
					if status == "" {
						status = strings.TrimPrefix(frame.Type, "task_")
					}
					label := d.Title
					if label == "" {
						label = d.ID
					}
					return realTimeMsg{kind: "task_update", payload: label + " → " + status}
				}
			case "agent_snapshot":
				// Initial state snapshot — ignore, no TUI action needed.
			}
		}
		return rtConnectedMsg{}
	}
}
