// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package events

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/ssestream"
)

func TestNewEnvelope_RoundTrip(t *testing.T) {
	props := PlanProposedProps{
		ProjectID: "prj-1",
		PlanID:    "plan-1",
		Raw:       "## plan",
		Summary:   "a todo app",
	}
	env, err := NewEnvelope(TypePlanProposed, props)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.Type != TypePlanProposed {
		t.Fatalf("type: %s", env.Type)
	}
	var back PlanProposedProps
	if err := env.Decode(&back); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if back.Summary != "a todo app" || back.ProjectID != "prj-1" {
		t.Fatalf("roundtrip lost data: %+v", back)
	}
}

func TestRegistryCoverage(t *testing.T) {
	// Every published Type constant must be registered. Forgetting to
	// register is a compile-time-equivalent mistake; this test guards it.
	expected := []Type{
		TypeSessionCreated, TypeSessionUpdated, TypeSessionIdle, TypeSessionError, TypeSessionCancelled,
		TypeMessageUpdated, TypeMessagePartUpdated, TypeMessagePartRemoved,
		TypePlanProposed, TypePlanApproved, TypePlanRejected, TypePlanRevisionRequested,
		TypeAgentSpawned, TypeAgentStarted, TypeAgentProgress, TypeAgentCompleted, TypeAgentError,
		TypeRoomPosted, TypeRoomDecision,
		TypeFileEdited, TypeFileWatcherUpdated,
		TypeBuildPhase, TypeBuildProgress,
		TypeGitHubRepoCreated, TypeGitHubPROpened, TypeGitHubPRReady, TypeGitHubCIStatus, TypeGitHubCommitPending,
		TypePreviewReady, TypeLSPDiagnostics,
		TypePermissionRequested, TypePermissionReplied,
		TypeTodoUpdated,
		TypeGraphNodeStarted, TypeGraphNodeCompleted, TypeGraphNodePaused, TypeGraphNodeFailed,
	}
	if len(typeRegistry) != len(expected) {
		t.Fatalf("registry size mismatch: %d vs %d expected", len(typeRegistry), len(expected))
	}
	for _, t2 := range expected {
		if !IsKnown(t2) {
			t.Errorf("type %s missing from registry", t2)
		}
	}
	// NewPropsFor must return a non-nil pointer for every known type.
	for _, t2 := range expected {
		if NewPropsFor(t2) == nil {
			t.Errorf("NewPropsFor(%s) returned nil", t2)
		}
	}
}

func TestEmitter_NoSubscribers_NoOp(t *testing.T) {
	e := NewEmitter()
	if err := e.Emit(context.Background(), SinkAll, TypeSessionIdle, SessionIdleProps{SessionID: "s1"}); err != nil {
		t.Fatalf("emit with no subscribers must not error: %v", err)
	}
}

func TestEmitter_ToSSE(t *testing.T) {
	e := NewEmitter()
	rec := httptest.NewRecorder()
	sse, err := ssestream.NewEmitter(rec)
	if err != nil {
		t.Fatalf("ssestream: %v", err)
	}
	detach := e.Attach("sub-1", sse)
	defer detach()

	if err := e.Emit(context.Background(), SinkSSE, TypeAgentStarted, AgentStartedProps{
		ProjectID: "prj-1", AgentKey: "prj-builder", Role: "backend",
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"agent.started"`) {
		t.Fatalf("missing envelope: %s", body)
	}
	// Properties must round-trip.
	var env Envelope
	// Extract the first "data: " line after the SSE frame for inspection.
	// Robustly, locate '{' and last '}' and unmarshal.
	start := strings.Index(body, "{")
	end := strings.LastIndex(body, "}")
	if start < 0 || end < 0 {
		t.Fatalf("no JSON in body: %s", body)
	}
	if err := json.Unmarshal([]byte(body[start:end+1]), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Type != TypeAgentStarted {
		t.Fatalf("unexpected type: %s", env.Type)
	}
}

func TestEmitter_DetachRemoves(t *testing.T) {
	e := NewEmitter()
	rec := httptest.NewRecorder()
	sse, _ := ssestream.NewEmitter(rec)
	detach := e.Attach("s1", sse)
	detach()

	if err := e.Emit(context.Background(), SinkSSE, TypeSessionIdle, SessionIdleProps{SessionID: "x"}); err != nil {
		t.Fatalf("emit after detach must not error: %v", err)
	}
	if strings.Contains(rec.Body.String(), "session.idle") {
		t.Fatalf("detached subscriber still received event: %s", rec.Body.String())
	}
}

func TestEmitter_UnknownTypeStillEmits(t *testing.T) {
	e := NewEmitter()
	rec := httptest.NewRecorder()
	sse, _ := ssestream.NewEmitter(rec)
	detach := e.Attach("s1", sse)
	defer detach()

	custom := Type("test.custom")
	if err := e.Emit(context.Background(), SinkSSE, custom, map[string]string{"x": "y"}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "test.custom") {
		t.Fatalf("custom type not emitted: %s", rec.Body.String())
	}
}
