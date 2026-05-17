// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/providers"
)

// TracingSpan represents an observability span for LLM calls or tool executions.
type TracingSpan struct {
	ID        string
	ParentID  string
	Name      string
	StartTime time.Time
	EndTime   time.Time
	Attrs     map[string]any
	Status    string // "ok", "error"
	Error     string
}

// SpanEmitter emits tracing spans for observability.
type SpanEmitter struct {
	traceID   string
	agentID   string
	sessionID string
	onSpan    func(TracingSpan)
}

// NewSpanEmitter creates a span emitter.
func NewSpanEmitter(agentID, sessionID string, onSpan func(TracingSpan)) *SpanEmitter {
	return &SpanEmitter{
		traceID:   uuid.New().String(),
		agentID:   agentID,
		sessionID: sessionID,
		onSpan:    onSpan,
	}
}

// EmitLLMSpanStart emits the start of an LLM call span.
func (e *SpanEmitter) EmitLLMSpanStart(ctx context.Context, iteration int, model string, msgCount int) string {
	spanID := uuid.New().String()
	if e.onSpan != nil {
		e.onSpan(TracingSpan{
			ID:        spanID,
			Name:      "llm.call",
			StartTime: time.Now().UTC(),
			Attrs: map[string]any{
				"trace_id":   e.traceID,
				"agent_id":   e.agentID,
				"session_id": e.sessionID,
				"iteration":  iteration,
				"model":      model,
				"msg_count":  msgCount,
			},
		})
	}
	return spanID
}

// EmitLLMSpanEnd emits the end of an LLM call span.
func (e *SpanEmitter) EmitLLMSpanEnd(spanID string, startTime time.Time, resp *providers.ChatResponse, err error) {
	if e.onSpan == nil {
		return
	}

	span := TracingSpan{
		ID:        spanID,
		Name:      "llm.call.end",
		StartTime: startTime,
		EndTime:   time.Now().UTC(),
		Status:    "ok",
		Attrs: map[string]any{
			"trace_id":    e.traceID,
			"duration_ms": time.Since(startTime).Milliseconds(),
		},
	}

	if err != nil {
		span.Status = "error"
		span.Error = err.Error()
	} else if resp != nil && resp.Usage != nil {
		span.Attrs["input_tokens"] = resp.Usage.PromptTokens
		span.Attrs["output_tokens"] = resp.Usage.CompletionTokens
		span.Attrs["total_tokens"] = resp.Usage.TotalTokens
	}

	e.onSpan(span)
}

// EmitToolSpanStart emits the start of a tool execution span.
func (e *SpanEmitter) EmitToolSpanStart(toolName, toolCallID, argsJSON string) string {
	spanID := uuid.New().String()
	if e.onSpan != nil {
		e.onSpan(TracingSpan{
			ID:        spanID,
			Name:      "tool.exec",
			StartTime: time.Now().UTC(),
			Attrs: map[string]any{
				"trace_id":     e.traceID,
				"agent_id":     e.agentID,
				"tool_name":    toolName,
				"tool_call_id": toolCallID,
				"args_len":     len(argsJSON),
			},
		})
	}
	return spanID
}

// EmitToolSpanEnd emits the end of a tool execution span.
func (e *SpanEmitter) EmitToolSpanEnd(spanID string, startTime time.Time, toolName string, isError bool, resultLen int) {
	if e.onSpan == nil {
		return
	}

	status := "ok"
	if isError {
		status = "error"
	}

	e.onSpan(TracingSpan{
		ID:        spanID,
		Name:      "tool.exec.end",
		StartTime: startTime,
		EndTime:   time.Now().UTC(),
		Status:    status,
		Attrs: map[string]any{
			"trace_id":    e.traceID,
			"tool_name":   toolName,
			"duration_ms": time.Since(startTime).Milliseconds(),
			"result_len":  resultLen,
		},
	})
}

// TraceID returns the current trace ID.
func (e *SpanEmitter) TraceID() string {
	return e.traceID
}
