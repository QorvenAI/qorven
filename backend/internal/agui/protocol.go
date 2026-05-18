// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package agui implements the AG-UI (Agent-User Interaction) protocol.
// It provides an HTTP handler at POST /v1/agui/stream that accepts a
// standard AG-UI RunAgentInput body and streams AG-UI events as
// newline-delimited JSON (application/x-ndjson).
//
// The mapping from Qorven StreamEvents to AG-UI events:
//
//	text_delta      → TEXT_MESSAGE_CONTENT (inside TEXT_MESSAGE_START/END)
//	thinking_delta  → STEP_STARTED (name="thinking") / STEP_FINISHED
//	tool_start      → TOOL_CALL_START + TOOL_CALL_ARGS
//	tool_result     → TOOL_CALL_END
//	done            → TEXT_MESSAGE_END + RUN_FINISHED
//	error           → RUN_ERROR
//
// State sync: after every completed turn a STATE_SNAPSHOT event is emitted
// carrying the session's messages so clients that store state server-side
// can rebuild from it without a separate REST call.
package agui

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/session"
)

// ── Wire types ────────────────────────────────────────────────────────────────

// EventType defines the standard AG-UI event discriminator.
type EventType string

const (
	// Lifecycle
	RunStarted  EventType = "RUN_STARTED"
	RunFinished EventType = "RUN_FINISHED"
	RunError    EventType = "RUN_ERROR"

	// Text streaming
	TextMessageStart   EventType = "TEXT_MESSAGE_START"
	TextMessageContent EventType = "TEXT_MESSAGE_CONTENT"
	TextMessageEnd     EventType = "TEXT_MESSAGE_END"

	// Tool calls
	ToolCallStart EventType = "TOOL_CALL_START"
	ToolCallArgs  EventType = "TOOL_CALL_ARGS"
	ToolCallEnd   EventType = "TOOL_CALL_END"

	// Thinking / reasoning
	StepStarted  EventType = "STEP_STARTED"
	StepFinished EventType = "STEP_FINISHED"

	// State synchronisation
	StateSnapshot    EventType = "STATE_SNAPSHOT"
	StateDelta       EventType = "STATE_DELTA"
	MessagesSnapshot EventType = "MESSAGES_SNAPSHOT"

	// Custom (pass-through for Qorven-specific events)
	Custom EventType = "CUSTOM"
)

// Event is a single AG-UI streaming event.
type Event struct {
	// Type is the AG-UI discriminator — always present.
	Type EventType `json:"type"`

	// Lifecycle fields
	RunID     string `json:"runId,omitempty"`
	ThreadID  string `json:"threadId,omitempty"`
	Error     string `json:"error,omitempty"`

	// Text streaming fields
	MessageID string `json:"messageId,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`

	// Tool call fields
	ToolCallID string         `json:"toolCallId,omitempty"`
	ToolName   string         `json:"toolName,omitempty"`
	Args       map[string]any `json:"args,omitempty"`
	Result     string         `json:"result,omitempty"`

	// Step fields (thinking/reasoning)
	StepName string `json:"stepName,omitempty"`

	// State fields
	Snapshot any `json:"snapshot,omitempty"`
	Delta    any `json:"delta,omitempty"`
	Messages any `json:"messages,omitempty"`

	// Custom payload
	Name  string `json:"name,omitempty"`
	Value any    `json:"value,omitempty"`

	// Timestamp (unix ms) — not in spec but useful for debugging
	Timestamp int64 `json:"timestamp,omitempty"`
}

// RunAgentInput is the request body for POST /v1/agui/stream.
// It mirrors the AG-UI RunAgentInput spec.
type RunAgentInput struct {
	// ThreadID is the session / conversation ID.
	ThreadID string `json:"threadId"`
	// RunID allows the client to supply a correlation id; server mints one if absent.
	RunID string `json:"runId,omitempty"`
	// AgentID selects which Qorven agent to invoke (agent_key or UUID).
	// Defaults to "default".
	AgentID string `json:"agentId,omitempty"`
	// Messages carries the full prior conversation; the last user message is sent.
	Messages []InputMessage `json:"messages"`
	// Model overrides the agent's configured model.
	Model string `json:"model,omitempty"`
	// State is optional client state; passed through as a STATE_SNAPSHOT echo.
	State any `json:"state,omitempty"`
}

// InputMessage is one message in the RunAgentInput messages array.
type InputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	ID      string `json:"id,omitempty"`
}

// ── AgentRunner interface ─────────────────────────────────────────────────────

// AgentRunner is the narrow interface the AG-UI handler depends on.
// *agent.Loop satisfies this.
type AgentRunner interface {
	Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error)
}

// ── Handler ───────────────────────────────────────────────────────────────────

// Handler implements POST /v1/agui/stream.
type Handler struct {
	runner   AgentRunner
	sessions *session.Store
	seq      atomic.Int64
}

// New constructs an AG-UI handler backed by runner.
func New(runner AgentRunner) *Handler {
	return &Handler{runner: runner}
}

// SetSessionStore attaches a session store so MESSAGES_SNAPSHOT includes
// the real prior conversation history, not just the current turn.
func (h *Handler) SetSessionStore(s *session.Store) { h.sessions = s }

// ServeHTTP handles POST /v1/agui/stream.
// It reads RunAgentInput from the request body, runs the agent, and streams
// AG-UI events as newline-delimited JSON (one JSON object per line).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input RunAgentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(input.Messages) == 0 {
		http.Error(w, "messages required", http.StatusBadRequest)
		return
	}

	// Derive the user prompt from the last user message.
	userMsg := ""
	for i := len(input.Messages) - 1; i >= 0; i-- {
		if input.Messages[i].Role == "user" {
			userMsg = input.Messages[i].Content
			break
		}
	}
	if userMsg == "" {
		http.Error(w, "no user message found", http.StatusBadRequest)
		return
	}

	runID := input.RunID
	if runID == "" {
		runID = uuid.New().String()
	}
	threadID := input.ThreadID
	if threadID == "" {
		threadID = uuid.New().String()
	}
	agentID := input.AgentID
	if agentID == "" {
		agentID = "default"
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(ev Event) {
		ev.Timestamp = time.Now().UnixMilli()
		b, err := json.Marshal(ev)
		if err != nil {
			slog.Warn("agui: marshal event", "type", ev.Type, "err", err)
			return
		}
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
	}

	// Emit optional client state echo first.
	if input.State != nil {
		send(Event{Type: StateSnapshot, RunID: runID, ThreadID: threadID, Snapshot: input.State})
	}

	send(Event{Type: RunStarted, RunID: runID, ThreadID: threadID})

	msgID := uuid.New().String()
	textStarted := false
	activeToolID := ""
	thinkingActive := false

	req := agent.RunRequest{
		AgentID:     agentID,
		SessionID:   threadID,
		UserMessage: userMsg,
		Channel:     "agui",
		Model:       input.Model,
		Stream:      true,
		NoPersist:   false,
	}

	_, runErr := h.runner.Run(r.Context(), req, func(ev agent.StreamEvent) {
		switch ev.Type {
		case "text_delta":
			if ev.Delta == "" {
				return
			}
			if !textStarted {
				send(Event{Type: TextMessageStart, RunID: runID, ThreadID: threadID,
					MessageID: msgID, Role: "assistant"})
				textStarted = true
			}
			send(Event{Type: TextMessageContent, RunID: runID, ThreadID: threadID,
				MessageID: msgID, Content: ev.Delta})

		case "thinking_delta":
			if ev.Delta == "" {
				return
			}
			if !thinkingActive {
				send(Event{Type: StepStarted, RunID: runID, ThreadID: threadID, StepName: "thinking"})
				thinkingActive = true
			}
			// Thinking content piped as a CUSTOM event — spec doesn't define it
			// but clients can subscribe to it for reasoning display.
			send(Event{Type: Custom, RunID: runID, ThreadID: threadID,
				Name: "thinking_delta", Value: ev.Delta})

		case "tool_start":
			if thinkingActive {
				send(Event{Type: StepFinished, RunID: runID, ThreadID: threadID, StepName: "thinking"})
				thinkingActive = false
			}
			activeToolID = ev.ToolID
			if activeToolID == "" {
				activeToolID = uuid.New().String()
			}
			send(Event{Type: ToolCallStart, RunID: runID, ThreadID: threadID,
				ToolCallID: activeToolID, ToolName: ev.Tool})
			if len(ev.Args) > 0 {
				send(Event{Type: ToolCallArgs, RunID: runID, ThreadID: threadID,
					ToolCallID: activeToolID, Args: ev.Args})
			}

		case "tool_result", "tool_end":
			tid := ev.ToolID
			if tid == "" {
				tid = activeToolID
			}
			result := ev.Result
			if result == "" {
				if s, ok := ev.Data.(string); ok {
					result = s
				}
			}
			send(Event{Type: ToolCallEnd, RunID: runID, ThreadID: threadID,
				ToolCallID: tid, ToolName: ev.Tool, Result: result})
			activeToolID = ""

		case "error":
			if textStarted {
				send(Event{Type: TextMessageEnd, RunID: runID, ThreadID: threadID, MessageID: msgID})
			}
			if thinkingActive {
				send(Event{Type: StepFinished, RunID: runID, ThreadID: threadID, StepName: "thinking"})
			}
			send(Event{Type: RunError, RunID: runID, ThreadID: threadID, Error: ev.Error})
		}
	})

	if runErr != nil {
		if !textStarted {
			// Nothing was streamed yet — emit a bare error.
			send(Event{Type: RunError, RunID: runID, ThreadID: threadID, Error: runErr.Error()})
			return
		}
		send(Event{Type: TextMessageEnd, RunID: runID, ThreadID: threadID, MessageID: msgID})
		if thinkingActive {
			send(Event{Type: StepFinished, RunID: runID, ThreadID: threadID, StepName: "thinking"})
		}
		send(Event{Type: RunError, RunID: runID, ThreadID: threadID, Error: runErr.Error()})
		return
	}

	if textStarted {
		send(Event{Type: TextMessageEnd, RunID: runID, ThreadID: threadID, MessageID: msgID})
	}
	if thinkingActive {
		send(Event{Type: StepFinished, RunID: runID, ThreadID: threadID, StepName: "thinking"})
	}

	// MESSAGES_SNAPSHOT — emit the full conversation for stateful clients.
	// Prefer DB history over the client-supplied messages array: the session
	// store is authoritative and includes messages from other channels.
	snapshot := []map[string]string{}
	if h.sessions != nil && threadID != "" {
		if hist, err := h.sessions.GetHistory(r.Context(), threadID); err == nil && len(hist) > 0 {
			snapshot = make([]map[string]string, 0, len(hist))
			for _, m := range hist {
				snapshot = append(snapshot, map[string]string{"role": m.Role, "content": m.Content})
			}
		}
	}
	if len(snapshot) == 0 {
		// Fall back to client-supplied messages + the assistant reply.
		snapshot = make([]map[string]string, 0, len(input.Messages)+1)
		for _, m := range input.Messages {
			snapshot = append(snapshot, map[string]string{"role": m.Role, "content": m.Content})
		}
	}
	send(Event{Type: MessagesSnapshot, RunID: runID, ThreadID: threadID, Messages: snapshot})

	send(Event{Type: RunFinished, RunID: runID, ThreadID: threadID})
}
