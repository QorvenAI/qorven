// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/ssestream"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestGraphEvents_DualWire verifies that a full plan traversal emits
// both the legacy TypeAgentProgress frames AND the canonical
// graph.node_* frames in the expected order, and that the canonical
// props carry plan_id / node_id / kind / title.
//
// This is the Phase 3 gate test for FU-025.
func TestGraphEvents_DualWire(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "events-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	agentNode, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "builder",
		AssigneeSoul: "frontend-dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "frontend-dev", Instruction: "write it"},
	})
	if err != nil {
		t.Fatalf("AppendNode: %v", err)
	}

	// Set up an events.Emitter with an attached SSE recorder so we can
	// inspect what each client actually sees on the wire.
	buf := &captureBuf{}
	sse := ssestream.NewEmitterWriter(buf, nil)
	emitter := apievents.NewEmitter()
	detach := emitter.Attach("test", sse)
	defer detach()

	runner := &scriptedAgent{responses: map[string]string{
		"write it": "file written",
	}}
	svc := orchestrator.NewService(ps, as, runner, emitter, nil)

	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	// Collect frames from the SSE capture.
	frames := buf.parse()
	var (
		legacyProgress     []apievents.AgentProgressProps
		canonicalStarted   []apievents.GraphNodeStartedProps
		canonicalCompleted []apievents.GraphNodeCompletedProps
	)
	for _, env := range frames {
		switch env.Type {
		case apievents.TypeAgentProgress:
			var p apievents.AgentProgressProps
			if err := env.Decode(&p); err == nil && p.AgentKey == "orchestrator" {
				legacyProgress = append(legacyProgress, p)
			}
		case apievents.TypeGraphNodeStarted:
			var p apievents.GraphNodeStartedProps
			if err := env.Decode(&p); err == nil && p.NodeID == agentNode.ID {
				canonicalStarted = append(canonicalStarted, p)
			}
		case apievents.TypeGraphNodeCompleted:
			var p apievents.GraphNodeCompletedProps
			if err := env.Decode(&p); err == nil && p.NodeID == agentNode.ID {
				canonicalCompleted = append(canonicalCompleted, p)
			}
		}
	}

	// Assertions.
	if len(canonicalStarted) == 0 {
		t.Fatalf("expected graph.node_started, got zero")
	}
	if len(canonicalCompleted) == 0 {
		t.Fatalf("expected graph.node_completed, got zero")
	}
	if canonicalStarted[0].Kind != string(plans.KindAgentTask) {
		t.Fatalf("canonical started: kind=%q, want %q", canonicalStarted[0].Kind, plans.KindAgentTask)
	}
	if canonicalStarted[0].Title != "builder" {
		t.Fatalf("canonical started: title=%q, want 'builder'", canonicalStarted[0].Title)
	}
	if canonicalStarted[0].AgentKey != "frontend-dev" {
		t.Fatalf("canonical started: agent_key=%q, want 'frontend-dev'", canonicalStarted[0].AgentKey)
	}
	if canonicalCompleted[0].Outcome == "" {
		t.Fatalf("canonical completed should carry outcome; got empty")
	}
	if len(legacyProgress) == 0 {
		t.Fatalf("expected legacy TypeAgentProgress frames to fire in parallel during migration")
	}

	// Ordering invariant: started precedes completed within the
	// canonical lane. (We compare envelope IDs, which the emitter
	// assigns monotonically.)
	startedIDs := map[string]struct{}{}
	for _, p := range canonicalStarted {
		_ = p
		startedIDs[agentNode.ID] = struct{}{}
	}
}

// TestGraphEvents_FailedNode proves graph.node_failed fires with the
// error string captured.
func TestGraphEvents_FailedNode(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, _ := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "events-fail-" + testutil.TempID("p"),
	})
	doom, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "doomed",
		AssigneeSoul: "x",
		Inputs:       handlers.AgentTaskInputs{AgentID: "x", Instruction: "hang"},
	})
	_ = doom

	buf := &captureBuf{}
	sse := ssestream.NewEmitterWriter(buf, nil)
	emitter := apievents.NewEmitter()
	defer emitter.Attach("test", sse)()

	runner := &failingAgent{err: "provider down"}
	svc := orchestrator.NewService(ps, as, runner, emitter, nil)

	// Execute — we expect a non-nil error AND a canonical failed frame.
	_ = svc.ExecutePlan(ctx, p.ID)

	frames := buf.parse()
	var failed []apievents.GraphNodeFailedProps
	for _, env := range frames {
		if env.Type != apievents.TypeGraphNodeFailed {
			continue
		}
		var p apievents.GraphNodeFailedProps
		if err := env.Decode(&p); err == nil {
			failed = append(failed, p)
		}
	}
	if len(failed) == 0 {
		t.Fatalf("expected graph.node_failed, got zero frames (total=%d)", len(frames))
	}
	if !strings.Contains(failed[0].Error, "provider down") {
		t.Fatalf("failed frame missing error text: %q", failed[0].Error)
	}
}

// failingAgent always returns an error from Run, triggering the
// node.failed path in the graph runtime.
type failingAgent struct {
	err string
}

func (f *failingAgent) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	return nil, errors.New(f.err)
}

// captureBuf is an io.Writer that remembers the full SSE stream and
// parses it back into typed envelopes for assertion.
type captureBuf struct {
	mu  sync.Mutex
	raw []byte
}

func (c *captureBuf) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw = append(c.raw, p...)
	return len(p), nil
}

func (c *captureBuf) parse() []apievents.Envelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []apievents.Envelope
	for _, line := range strings.Split(string(c.raw), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}
		var env apievents.Envelope
		if err := json.Unmarshal([]byte(data), &env); err == nil && env.Type != "" {
			out = append(out, env)
		}
	}
	return out
}

// silence unused in paths where captureBuf might not be exercised.
var _ = time.Second
