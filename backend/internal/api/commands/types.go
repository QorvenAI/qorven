// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package commands defines the canonical command surface shared between
// the Qorven web UI and the bubbletea TUI. Every user-initiated action
// (send a prompt, open the model picker, resize the TUI, show a toast)
// flows through one of these endpoints.
//
// The contract is the same across clients:
//
//	POST /v1/commands/<verb>    Content-Type: application/json
//	Request:  typed struct below
//	Response: {"ok":true,...} or {"error":"..."}
//
// Rule: if a capability exists on the web and on the TUI, it lives here
// as one endpoint. Do not add client-specific variants.
package commands

import "encoding/json"

// AppendPromptRequest appends text to the in-progress prompt draft for
// a given session. Does NOT submit the prompt. Used by the web /code page
// when a user drags text into the input, and by the TUI when a slash
// command is accepted.
type AppendPromptRequest struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

// ClearPromptRequest empties the draft for a session.
type ClearPromptRequest struct {
	SessionID string `json:"session_id"`
}

// SubmitPromptRequest commits the current draft. If Text is non-empty
// the server treats it as a replacement of the draft before submission.
// The response is fire-and-forget — the actual model stream is delivered
// on the session's SSE channel.
type SubmitPromptRequest struct {
	SessionID string            `json:"session_id"`
	AgentID   string            `json:"agent_id,omitempty"`
	Text      string            `json:"text,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ExecuteCommandRequest runs any named command by ID. Currently supported:
//
//	clear           — clears session messages
//	new_session     — creates a fresh session bound to AgentID
//	toggle_sidebar  — client-side instruction; server echoes to all clients
//	open_theme      — opens the theme picker
//	abort           — stops the active run for the session
//
// Args is an opaque JSON object delivered to the command handler.
type ExecuteCommandRequest struct {
	Command string          `json:"command"`
	Args    json.RawMessage `json:"args,omitempty"`
}

// OpenSessionsRequest opens the session picker UI. Filter is optional.
type OpenSessionsRequest struct {
	AgentID string `json:"agent_id,omitempty"`
	Filter  string `json:"filter,omitempty"`
}

// OpenModelsRequest opens the model picker UI. Scope may be "global" or
// a session id; "global" changes the default, a session id changes that
// session only.
type OpenModelsRequest struct {
	Scope string `json:"scope,omitempty"`
}

// OpenThemesRequest opens the theme picker.
type OpenThemesRequest struct{}

// ShowToastRequest tells every attached client to display a transient
// notification. Level is "info"|"success"|"warn"|"error".
type ShowToastRequest struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	// DurationMS defaults to 4000 when zero. -1 means sticky.
	DurationMS int `json:"duration_ms,omitempty"`
	// Actor tags the origin for deduplication ("tui"|"web"|"server").
	Actor string `json:"actor,omitempty"`
}

// ResizeRequest is sent by the TUI on terminal resize. Cols/Rows are the
// current cell dimensions. The server uses them for PTY sizing
// and discards otherwise.
type ResizeRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// Response is the canonical shape for every command reply. At least one
// of Error or OK is set. Data carries command-specific payload (e.g.
// SubmitPrompt returns {"message_id": "..."} here).
type Response struct {
	OK    bool           `json:"ok"`
	Error string         `json:"error,omitempty"`
	Code  string         `json:"code,omitempty"`
	Data  map[string]any `json:"data,omitempty"`
}
