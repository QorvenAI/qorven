// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RunLifecycle handles run lifecycle events and tracing.
type RunLifecycle struct {
	agentID        string
	agentUUID      uuid.UUID
	traceCollector TraceCollector
	onEvent        func(AgentEvent)
}

// TraceCollector interface for trace operations.
type TraceCollector interface {
	CreateTrace(ctx context.Context, trace *TraceData) error
	FinishTrace(ctx context.Context, traceID uuid.UUID, status string, errMsg, output string)
	SetTraceStatus(ctx context.Context, traceID uuid.UUID, status string)
	PreviewMaxLen() int
}

// TraceData represents a trace record.
type TraceData struct {
	ID            uuid.UUID
	RunID         string
	SessionKey    string
	UserID        string
	Channel       string
	Name          string
	InputPreview  string
	Status        string
	StartTime     time.Time
	CreatedAt     time.Time
	Tags          []string
	AgentID       *uuid.UUID
	ParentTraceID *uuid.UUID
	TeamID        *uuid.UUID
}

// AgentEvent represents an event from the agent loop.
type AgentEvent struct {
	Type          string
	AgentID       string
	RunID         string
	RunKind       string
	DelegationID  string
	TeamID        string
	TeamTaskID    string
	ParentAgentID string
	UserID        string
	Channel       string
	ChatID        string
	SessionKey    string
	TenantID      uuid.UUID
	Payload       any
}

// NewRunLifecycle creates a new run lifecycle handler.
func NewRunLifecycle(agentID string, agentUUID uuid.UUID, collector TraceCollector, onEvent func(AgentEvent)) *RunLifecycle {
	return &RunLifecycle{
		agentID:        agentID,
		agentUUID:      agentUUID,
		traceCollector: collector,
		onEvent:        onEvent,
	}
}

// EmitRunStarted emits a run started event.
func (rl *RunLifecycle) EmitRunStarted(runID, message string) {
	if rl.onEvent != nil {
		rl.onEvent(AgentEvent{
			Type:    "run_started",
			AgentID: rl.agentID,
			RunID:   runID,
			Payload: map[string]any{"message": message},
		})
	}
}

// EmitRunCompleted emits a run completed event.
func (rl *RunLifecycle) EmitRunCompleted(runID, content string, usage *UsageStats, media []MediaResult) {
	if rl.onEvent == nil {
		return
	}
	payload := map[string]any{"content": content}
	if usage != nil {
		payload["usage"] = map[string]any{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
	}
	if len(media) > 0 {
		payload["media"] = media
	}
	rl.onEvent(AgentEvent{
		Type:    "run_completed",
		AgentID: rl.agentID,
		RunID:   runID,
		Payload: payload,
	})
}

// EmitRunFailed emits a run failed event.
func (rl *RunLifecycle) EmitRunFailed(runID string, err error) {
	if rl.onEvent != nil {
		rl.onEvent(AgentEvent{
			Type:    "run_failed",
			AgentID: rl.agentID,
			RunID:   runID,
			Payload: map[string]string{"error": err.Error()},
		})
	}
}

// EmitRunCancelled emits a run cancelled event.
func (rl *RunLifecycle) EmitRunCancelled(runID string) {
	if rl.onEvent != nil {
		rl.onEvent(AgentEvent{
			Type:    "run_cancelled",
			AgentID: rl.agentID,
			RunID:   runID,
		})
	}
}

// UsageStats represents token usage statistics.
type UsageStats struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// CreateTrace creates a new trace for a run.
func (rl *RunLifecycle) CreateTrace(ctx context.Context, runID, sessionKey, userID, channel, name, input string, tags []string) (uuid.UUID, error) {
	if rl.traceCollector == nil {
		return uuid.Nil, nil
	}

	traceID := uuid.New()
	now := time.Now().UTC()
	trace := &TraceData{
		ID:           traceID,
		RunID:        runID,
		SessionKey:   sessionKey,
		UserID:       userID,
		Channel:      channel,
		Name:         name,
		InputPreview: truncateForPreview(input, rl.traceCollector.PreviewMaxLen()),
		Status:       "running",
		StartTime:    now,
		CreatedAt:    now,
		Tags:         tags,
	}
	if rl.agentUUID != uuid.Nil {
		trace.AgentID = &rl.agentUUID
	}

	if err := rl.traceCollector.CreateTrace(ctx, trace); err != nil {
		return uuid.Nil, err
	}
	return traceID, nil
}

// FinishTrace completes a trace.
func (rl *RunLifecycle) FinishTrace(ctx context.Context, traceID uuid.UUID, status, errMsg, output string) {
	if rl.traceCollector == nil || traceID == uuid.Nil {
		return
	}
	rl.traceCollector.FinishTrace(ctx, traceID, status, errMsg, truncateForPreview(output, rl.traceCollector.PreviewMaxLen()))
}

func truncateForPreview(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
