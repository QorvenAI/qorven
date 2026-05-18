// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// AgentInterface is the core abstraction for an AI agent execution loop.
// Implemented by *Loop; extracted as an interface for testability.
type AgentInterface interface {
	ID() string
	Run(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) (*RunResult, error)
	Model() string
	ProviderName() string
	Provider() providers.Provider
}

// RunRequest contains all parameters for a single agent run.
type RunRequest struct {
	AgentID        string
	SessionID      string
	SessionKey     string
	UserMessage    string
	UserID         string
	ChannelType    string
	Channel        string
	ChatTitle      string
	PeerKind       string // "direct", "group"
	HistoryLimit   int
	SkillFilter    []string
	ToolAllow      []string
	ExtraSystemPrompt string // injected group/topic prompt
	LightContext   bool
	NoTools        bool
	Summary        string // compressed context summary from previous turns
	NoPersist      bool
	Mode           string // "plan", "chat", etc.
	DelegationMode string // "auto" (default), "explicit" (@-mention only), "manual" (confirm each step)
	MemoryBulletin string // curated memory from Engine
	UserProfile    string // user profile context
	WorkingMemory  string // working memory context
	Stream         bool   // enable streaming
	Depth          string // recursion depth: "shallow", "balanced", "deep"
	Model          string // override model for this request
	ThinkingLevel  string // per-request override: off | medium | high (empty = use agent default)
	Images         []ImageInput
	Documents      []DocumentInput
	Audio          []AudioInput

	// ExtraTools is a per-run list of additional tools the LLM may
	// call on top of the Loop's global registry. Used by the
	// orchestrator to inject tenant-scoped Wasm plugins without
	// mutating the shared tool registry (which would leak tools
	// across tenants). Nil means "use global registry unchanged."
	//
	// Tools in this slice MUST already be wrapped with
	// permissions.WrapLazy if they are destructive. The Loop does
	// not re-wrap — it trusts its caller.
	//
	// consumed by ContextBuilder.BuildToolDefs and by the
	// tool-execution dispatcher. A tool name in ExtraTools that
	// collides with a global tool name SHADOWS the global one for
	// this run only.
	ExtraTools []tools.Tool

	// TenantID identifies the tenant owning this run. Today populated
	// by callers that build RunRequest from an authenticated HTTP
	// context (orchestrator handlers, HTTP protocol handler). Used
	// by wasm invocation metrics + as a safety label on any per-run
	// tool attribution. The Loop itself has l.tenantID for platform-
	// internal tools; this field is for per-request attribution.
	TenantID string

	DiscussionID  string // current discussion cluster, populated by gateway, used by task worker
	SourceChannel string // 'web', 'tui', 'telegram', 'whatsapp', etc.

	// Per-conversation Active Memory filters.
	// MemoryScopeAllow: if non-empty, only these memory scopes are searched
	// (values: "company", "team", "prime", "agent", "session").
	// MemoryScopeDeny:  always exclude these scopes even if allow is empty.
	// When both are nil the default hierarchy search applies unchanged.
	MemoryScopeAllow []string
	MemoryScopeDeny  []string
}

// ImageInput represents an image attached to a request.
type ImageInput struct {
	URL         string
	Base64      string
	ContentType string
}

// DocumentInput represents a document attached to a request.
type DocumentInput struct {
	Path        string
	Content     string
	ContentType string
}

// AudioInput represents audio attached to a request.
type AudioInput struct {
	Path        string
	Base64      string
	ContentType string
}

// RunResult contains the output of an agent run.
type RunResult struct {
	Content       string
	Media         []MediaResult
	ToolCalls     int
	ToolsUsed     []string
	Iterations    int
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	CostCents     float64
	DurationMs    int64
	Aborted       bool
	Error         error
	Parts         []MessagePart     // streaming parts
	Metadata      map[string]any    // additional metadata
	Sources       []Source          // web sources
	Thinking      string            // thinking/reasoning content
	TraceID       string            // distributed trace ID for this run
}

// StreamEvent represents a streaming event from the agent loop.
type StreamEvent struct {
	Type    string
	Content string
	Delta   string
	Tool    string
	ToolID  string
	Args    map[string]any
	Result  string
	Error   string
	Done    bool
	Media   []MediaResult
	Data    any // generic data payload
}

// Event type constants
const (
	EventTypeText       = "text"
	EventTypeDelta      = "delta"
	EventTypeToolStart  = "tool_start"
	EventTypeToolEnd    = "tool_end"
	EventTypeThinking   = "thinking"
	EventTypeDone       = "done"
	EventTypeError      = "error"
	EventTypeMedia      = "media"
	EventTypeActivity   = "activity"
)

// TextEvent creates a text event.
func TextEvent(content string) StreamEvent {
	return StreamEvent{Type: EventTypeText, Content: content}
}

// DeltaEvent creates a delta (streaming chunk) event.
func DeltaEvent(delta string) StreamEvent {
	return StreamEvent{Type: EventTypeDelta, Delta: delta}
}

// ToolStartEvent creates a tool start event.
func ToolStartEvent(tool, toolID string, args map[string]any) StreamEvent {
	return StreamEvent{Type: EventTypeToolStart, Tool: tool, ToolID: toolID, Args: args}
}

// ToolEndEvent creates a tool end event.
func ToolEndEvent(tool, toolID, result string) StreamEvent {
	return StreamEvent{Type: EventTypeToolEnd, Tool: tool, ToolID: toolID, Result: result}
}

// DoneEvent creates a done event.
func DoneEvent() StreamEvent {
	return StreamEvent{Type: EventTypeDone, Done: true}
}

// ErrorEvent creates an error event.
func ErrorEvent(err string) StreamEvent {
	return StreamEvent{Type: EventTypeError, Error: err}
}

// MediaEvent creates a media event.
func MediaEvent(media []MediaResult) StreamEvent {
	return StreamEvent{Type: EventTypeMedia, Media: media}
}

// TitleEvent creates a title event.
func TitleEvent(title string) StreamEvent {
	return StreamEvent{Type: "title", Content: title}
}

// TagsEvent creates a tags event.
func TagsEvent(tags []string) StreamEvent {
	return StreamEvent{Type: "tags", Args: map[string]any{"tags": tags}}
}

// FollowUpEvent creates a follow-up suggestions event.
func FollowUpEvent(followUps []string) StreamEvent {
	return StreamEvent{Type: "follow_ups", Args: map[string]any{"follow_ups": followUps}}
}

// WidgetEvent creates a widget event.
func WidgetEvent(widget any) StreamEvent {
	return StreamEvent{Type: "widget", Data: widget}
}

// PartEvent creates a part event for streaming.
func PartEvent(part any) StreamEvent {
	return StreamEvent{Type: "part", Data: part}
}

// TextDelta creates a text delta for streaming.
func TextDelta(delta string) StreamEvent {
	return StreamEvent{Type: "text_delta", Delta: delta}
}

// ThinkingDelta creates a thinking delta for streaming.
func ThinkingDelta(delta string) StreamEvent {
	return StreamEvent{Type: "thinking_delta", Delta: delta}
}

// ToolStart creates a tool start event (single arg version for compatibility).
func ToolStart(tool string) StreamEvent {
	return StreamEvent{Type: EventTypeToolStart, Tool: tool}
}

// ToolResult creates a tool result event (two arg version for compatibility).
func ToolResult(tool, result string) StreamEvent {
	return StreamEvent{Type: EventTypeToolEnd, Tool: tool, Result: result}
}

// StreamReset tells the client to discard any text streamed before tool calls.
// Emitted when the LLM outputs text then decides to call a tool — the pre-tool
// text is narration/preamble, not the final answer.
func StreamReset() StreamEvent {
	return StreamEvent{Type: "stream_reset"}
}
