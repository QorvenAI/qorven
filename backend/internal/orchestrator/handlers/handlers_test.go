// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/orchestrator/graph"
	"github.com/qorvenai/qorven/internal/plans"
)

// stubRunner is a minimal AgentRunner used to drive handlers without
// touching the real agent loop. It streams the scripted events to
// onEvent and returns nil.
type stubRunner struct {
	events  []agent.StreamEvent
	err     error
	seenReq *agent.RunRequest
}

func (s *stubRunner) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	s.seenReq = &req
	for _, e := range s.events {
		onEvent(e)
	}
	if s.err != nil {
		return nil, s.err
	}
	return &agent.RunResult{}, nil
}

func TestFindJSONBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		sWant  int
		eWant  int
	}{
		{"plain", `{"x":1}`, 0, 6},
		{"leading prose", `plan below: {"k":"v"} done`, 12, 20},
		{"nested", `{"outer":{"inner":true}}`, 0, 23},
		{"string with brace", `{"greeting":"{}"}`, 0, 16},
		{"none", "no json here", -1, -1},
		{"only open brace", "{oops", -1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, e := findJSONBoundaries(tc.input)
			if s != tc.sWant || e != tc.eWant {
				t.Fatalf("got (%d,%d) want (%d,%d) for %q", s, e, tc.sWant, tc.eWant, tc.input)
			}
		})
	}
}

func TestBuildPlannerPrompt_IncludesDescriptionAndStack(t *testing.T) {
	p := buildPlannerPrompt(PlannerInputs{Description: "todo app", Stack: "next + prisma"})
	if !strings.Contains(p, "todo app") {
		t.Fatalf("missing description in prompt: %s", p)
	}
	if !strings.Contains(p, "next + prisma") {
		t.Fatalf("missing stack in prompt: %s", p)
	}
}

func TestPlanner_NoRunner(t *testing.T) {
	h := Planner(Config{})
	_, _, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: &plans.Node{},
	})
	if err == nil {
		t.Fatalf("expected nil-runner error")
	}
}

func TestPlanner_EmptyInputs(t *testing.T) {
	h := Planner(Config{Agent: &stubRunner{}})
	node := &plans.Node{Inputs: json.RawMessage(`{}`)}
	_, _, err := h(context.Background(), &graph.HandlerContext{Plan: &plans.Plan{}, Node: node})
	if err == nil {
		t.Fatalf("expected description-required error")
	}
}

func TestPlanner_StreamsJSON(t *testing.T) {
	runner := &stubRunner{events: []agent.StreamEvent{
		{Type: "text_delta", Delta: "prose "},
		{Type: "text_delta", Delta: `{"stack":"next","summary":"x","agents":[]}`},
	}}
	h := Planner(Config{Agent: runner})
	node := &plans.Node{Inputs: jsonMust(PlannerInputs{Description: "app"})}
	outcome, artifacts, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: node,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if outcome != graph.OutcomeSuccess {
		t.Fatalf("outcome: %s", outcome)
	}
	m := artifacts.(map[string]any)
	if m["plan"] == nil {
		t.Fatalf("plan not parsed: %+v", m)
	}
	plan := m["plan"].(map[string]any)
	if plan["stack"] != "next" {
		t.Fatalf("stack: %v", plan["stack"])
	}
	// The runner should have received Prime as agent by default.
	if runner.seenReq == nil || runner.seenReq.AgentID != "prime" {
		t.Fatalf("agent id: %+v", runner.seenReq)
	}
}

func TestPlanner_NoJSONReturnsError(t *testing.T) {
	runner := &stubRunner{events: []agent.StreamEvent{
		{Type: "text_delta", Delta: "no json at all"},
	}}
	h := Planner(Config{Agent: runner})
	node := &plans.Node{Inputs: jsonMust(PlannerInputs{Description: "app"})}
	outcome, artifacts, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: node,
	})
	if err == nil {
		t.Fatal("expected error when agent produces no JSON object")
	}
	if outcome != graph.OutcomeError {
		t.Fatalf("expected OutcomeError, got %s", outcome)
	}
	m := artifacts.(map[string]any)
	if m["raw"] != "no json at all" {
		t.Fatalf("raw text not preserved in error artifacts: %+v", m["raw"])
	}
}

func TestPlanner_AgentError(t *testing.T) {
	runner := &stubRunner{err: errors.New("provider down")}
	h := Planner(Config{Agent: runner})
	node := &plans.Node{Inputs: jsonMust(PlannerInputs{Description: "app"})}
	_, _, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: node,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestAgentTask_NoRunner(t *testing.T) {
	h := AgentTask(Config{})
	_, _, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: &plans.Node{Inputs: jsonMust(AgentTaskInputs{AgentID: "x", Instruction: "y"})},
	})
	if err == nil {
		t.Fatalf("expected nil-runner error")
	}
}

func TestAgentTask_RequiresInstruction(t *testing.T) {
	h := AgentTask(Config{Agent: &stubRunner{}})
	_, _, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: &plans.Node{Inputs: jsonMust(AgentTaskInputs{AgentID: "x"})},
	})
	if err == nil {
		t.Fatalf("expected instruction-required error")
	}
}

func TestAgentTask_AssigneeFallback(t *testing.T) {
	runner := &stubRunner{events: []agent.StreamEvent{
		{Type: "text_delta", Delta: "done"},
	}}
	h := AgentTask(Config{Agent: runner})
	node := &plans.Node{
		AssigneeSoul: "frontend-dev",
		Inputs:       jsonMust(AgentTaskInputs{Instruction: "write hello"}),
	}
	outcome, artifacts, err := h(context.Background(), &graph.HandlerContext{
		Plan: &plans.Plan{}, Node: node,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if outcome != graph.OutcomeSuccess {
		t.Fatalf("outcome: %s", outcome)
	}
	m := artifacts.(map[string]any)
	if m["agent_id"] != "frontend-dev" {
		t.Fatalf("agent_id fallback: %v", m["agent_id"])
	}
	if m["output"] != "done" {
		t.Fatalf("output: %v", m["output"])
	}
}

func TestRegisterAll(t *testing.T) {
	reg := graph.NewRegistry()
	RegisterAll(reg, Config{Agent: &stubRunner{}})
	for _, kind := range []plans.NodeKind{plans.KindPlanner, plans.KindHumanFeedback, plans.KindAgentTask} {
		if reg.Handler(kind) == nil {
			t.Fatalf("missing handler for %s", kind)
		}
	}
}

func jsonMust(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
