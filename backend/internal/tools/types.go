// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
)

// Tool is the interface all tools must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any
	Execute(ctx context.Context, args map[string]any) *Result
}

// Result is the output of a tool execution.
type Result struct {
	ForLLM  string `json:"content"`          // sent to LLM as tool result
	ForUser string `json:"user_content"`     // shown to user (may differ)
	IsError bool   `json:"is_error"`
	Media   []MediaFile `json:"media,omitempty"` // file attachments
	// Widget is an optional structured UI payload. When set, the
	// agent loop emits a `widget` MessagePart so the chat UI renders
	// a rich card (weather, flight options, SQL table, screenshot,
	// etc.) alongside the tool's text result. Keep the shape simple:
	// WidgetType is the renderer key (see web/components/chat/part-renderer.tsx),
	// WidgetData is the serialisable payload the card expects.
	Widget *Widget `json:"widget,omitempty"`
	// Widgets is the many-widgets variant for tools that produce
	// multiple structured outputs — e.g. browse_and_act emits one
	// browser_step card per step. The loop iterates and emits each
	// in order. Prefer `Widget` when only one card applies.
	Widgets []Widget `json:"widgets,omitempty"`
}

// Widget is a structured UI payload from a tool, rendered client-side
// by the card registry. See web/components/chat/part-renderer.tsx
// WidgetPartRenderer for the list of supported types.
type Widget struct {
	Type string         `json:"type"` // "weather", "flights", "sql_result", ...
	Data map[string]any `json:"data"`
}

type MediaFile struct {
	Path     string `json:"path"`
	MimeType string `json:"mime_type"`
}

func TextResult(text string) *Result   { return &Result{ForLLM: text, ForUser: text} }
func ErrorResult(msg string) *Result   { return &Result{ForLLM: msg, ForUser: msg, IsError: true} }
func SuccessResult(msg string) *Result { return &Result{ForLLM: msg, ForUser: msg} }
func SilentResult(text string) *Result { return &Result{ForLLM: text, ForUser: ""} }

// ToolDefinition is the JSON schema sent to LLM providers.
type ToolDefinition struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToDefinition converts a Tool to a ToolDefinition for LLM APIs.
func ToDefinition(t Tool) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		},
	}
}

// Context keys for per-call values (prevents mutable state on shared tool instances).
type ctxKey string

const (
	ctxWorkspace   ctxKey = "tool_workspace"
	ctxChannel     ctxKey = "tool_channel"
	ctxChatID      ctxKey = "tool_chat_id"
	ctxSessionKey  ctxKey = "tool_session_key"
	ctxAgentID     ctxKey = "tool_agent_id"
	ctxUserID      ctxKey = "tool_user_id"
	ctxTenantID    ctxKey = "tool_tenant_id"
	ctxForkFunc    ctxKey = "tool_fork_func"
	ctxSandboxKey  ctxKey = "tool_sandbox_key"
)

func WithWorkspace(ctx context.Context, ws string) context.Context {
	return context.WithValue(ctx, ctxWorkspace, ws)
}
func WorkspaceFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxWorkspace).(string); ok { return v }
	return ""
}
func WithAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxAgentID, id)
}
func AgentIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxAgentID).(string); ok { return v }
	return ""
}
func WithSessionID(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ctxSessionKey, key)
}
func SessionIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxSessionKey).(string); ok { return v }
	return ""
}

// WithTenantID stamps the current tenant id on ctx. Used by the
// permission gate wrapper to populate permission_requests.tenant_id
// so multi-tenant RLS policies accept the row. Set on tool-execution
// ctx via l.executeTool (agent loop) and on gate ctx via scheduled
// plugin invocations.
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxTenantID, id)
}
func TenantIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxTenantID).(string); ok { return v }
	return ""
}
func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxUserID, id)
}
func UserIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxUserID).(string); ok { return v }
	return ""
}

const ctxDiscussionID ctxKey = "tool_discussion_id"

func WithDiscussionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxDiscussionID, id)
}
func DiscussionIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxDiscussionID).(string); ok { return v }
	return ""
}

// ForkFunc is a callback that spawns a subagent with a directive.
type ForkFunc func(ctx context.Context, directive string) (string, error)

func WithForkFunc(ctx context.Context, fn ForkFunc) context.Context {
	return context.WithValue(ctx, ctxForkFunc, fn)
}
func ForkFuncFromCtx(ctx context.Context) ForkFunc {
	if v, ok := ctx.Value(ctxForkFunc).(ForkFunc); ok { return v }
	return nil
}

// Sandbox context helpers
func WithSandboxKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, ctxSandboxKey, key)
}
func SandboxKeyFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxSandboxKey).(string); ok { return v }
	return ""
}

// MimeFromExt returns MIME type for common file extensions.
func MimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf": return "application/pdf"
	case ".csv": return "text/csv"
	case ".md": return "text/markdown"
	case ".txt": return "text/plain"
	case ".json": return "application/json"
	case ".html": return "text/html"
	case ".docx": return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx": return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".png": return "image/png"
	case ".jpg", ".jpeg": return "image/jpeg"
	default: return "application/octet-stream"
	}
}
