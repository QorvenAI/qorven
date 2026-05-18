// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package graph

import (
	"encoding/json"
	"testing"

	"github.com/qorvenai/qorven/internal/plans"
)

// These tests exercise the pure-Go helpers without a database, so CI
// without Postgres still verifies the core logic.

func TestArtifactsJSON_NilReturnsNil(t *testing.T) {
	if got := artifactsJSON(nil); got != nil {
		t.Fatalf("nil → got %s", got)
	}
}

func TestArtifactsJSON_EmptyMapReturnsNil(t *testing.T) {
	if got := artifactsJSON(map[string]any{}); got != nil {
		t.Fatalf("empty map → %s", got)
	}
}

func TestArtifactsJSON_RawMessagePassthrough(t *testing.T) {
	raw := json.RawMessage(`{"ok":true}`)
	got := artifactsJSON(raw)
	if string(got) != `{"ok":true}` {
		t.Fatalf("passthrough: %s", got)
	}
}

func TestArtifactsJSON_MapMarshals(t *testing.T) {
	got := artifactsJSON(map[string]any{"k": 1})
	if string(got) != `{"k":1}` {
		t.Fatalf("marshal: %s", got)
	}
}

func TestAllTerminal(t *testing.T) {
	cases := []struct {
		name   string
		states []plans.NodeState
		want   bool
	}{
		{"all done", []plans.NodeState{plans.NodeDone, plans.NodeDone}, true},
		{"mixed done/pending", []plans.NodeState{plans.NodeDone, plans.NodePending}, false},
		{"done+failed+cancelled", []plans.NodeState{plans.NodeDone, plans.NodeFailed, plans.NodeCancelled}, true},
		{"one running", []plans.NodeState{plans.NodeRunning}, false},
		{"empty", []plans.NodeState{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes := make([]*plans.Node, len(tc.states))
			for i, s := range tc.states {
				nodes[i] = &plans.Node{State: s}
			}
			if got := allTerminal(nodes); got != tc.want {
				t.Fatalf("allTerminal: got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAnyFailed(t *testing.T) {
	nodes := []*plans.Node{{State: plans.NodeDone}, {State: plans.NodeDone}}
	if anyFailed(nodes) {
		t.Fatalf("no failed but detected")
	}
	nodes = append(nodes, &plans.Node{State: plans.NodeFailed})
	if !anyFailed(nodes) {
		t.Fatalf("failed node not detected")
	}
}

func TestArtifactHasString(t *testing.T) {
	raw := json.RawMessage(`{"approval":"approved","x":1}`)
	if !artifactHasString(raw, "approval", "approved") {
		t.Fatalf("approval/approved not matched")
	}
	if artifactHasString(raw, "approval", "rejected") {
		t.Fatalf("false positive")
	}
	if artifactHasString(raw, "missing", "x") {
		t.Fatalf("missing key false positive")
	}
	if artifactHasString(nil, "x", "y") {
		t.Fatalf("nil payload false positive")
	}
	if artifactHasString(json.RawMessage(`garbage`), "a", "b") {
		t.Fatalf("garbage payload false positive")
	}
}

func TestEdgeSatisfied_AlwaysAndOnSuccess(t *testing.T) {
	parent := &plans.Node{State: plans.NodeDone}
	for _, cond := range []plans.EdgeCondition{plans.CondAlways, plans.CondOnSuccess} {
		if !edgeSatisfied(parent, &plans.Edge{Condition: cond}) {
			t.Fatalf("%s should satisfy on done", cond)
		}
	}
	parent.State = plans.NodeFailed
	for _, cond := range []plans.EdgeCondition{plans.CondAlways, plans.CondOnSuccess} {
		if edgeSatisfied(parent, &plans.Edge{Condition: cond}) {
			t.Fatalf("%s should NOT satisfy on failed", cond)
		}
	}
}

func TestEdgeSatisfied_ApprovalRouting(t *testing.T) {
	parent := &plans.Node{
		State:     plans.NodeDone,
		Artifacts: json.RawMessage(`{"approval":"approved"}`),
	}
	if !edgeSatisfied(parent, &plans.Edge{Condition: plans.CondApproved}) {
		t.Fatalf("approved edge should satisfy")
	}
	if edgeSatisfied(parent, &plans.Edge{Condition: plans.CondRejected}) {
		t.Fatalf("rejected edge should not satisfy on approved artifact")
	}

	parent.Artifacts = json.RawMessage(`{"approval":"rejected"}`)
	if !edgeSatisfied(parent, &plans.Edge{Condition: plans.CondRejected}) {
		t.Fatalf("rejected edge should satisfy on rejected artifact")
	}

	parent.Artifacts = json.RawMessage(`{"approval":"revision_requested"}`)
	if !edgeSatisfied(parent, &plans.Edge{Condition: plans.CondRevision}) {
		t.Fatalf("revision edge should satisfy")
	}
}

func TestEdgeSatisfied_OnErrorIsConservative(t *testing.T) {
	// on_error is documented as reserved — it does not fire in Phase 2.
	parent := &plans.Node{State: plans.NodeDone}
	if edgeSatisfied(parent, &plans.Edge{Condition: plans.CondOnError}) {
		t.Fatalf("on_error should not activate today")
	}
}

func TestPauseSignal_ErrorString(t *testing.T) {
	p := &PauseSignal{Reason: "awaiting review"}
	if p.Error() != "graph: paused: awaiting review" {
		t.Fatalf("error: %s", p.Error())
	}
	p = &PauseSignal{}
	if p.Error() != "graph: paused" {
		t.Fatalf("default error: %s", p.Error())
	}
}
