// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/delegation"
)

// captureRunAgent is a fake runAgent that records the context depth it received
// and returns a canned result. It implements the closure signature expected by
// NewDelegateTool.
type captureRunAgent struct {
	capturedDepth int
	called        bool
}

func (c *captureRunAgent) run(ctx context.Context, agentKey, message string) (string, error) {
	c.called = true
	c.capturedDepth = DelegationDepthFromCtx(ctx)
	return "ok from " + agentKey, nil
}

func TestDelegateTool_IncrementDepth(t *testing.T) {
	cap := &captureRunAgent{}
	tool := NewDelegateTool(cap.run, nil)

	// Call with depth 0 (absent from ctx — default).
	ctx := context.Background()
	result := tool.Execute(ctx, map[string]any{
		"agent": "researcher",
		"task":  "find something",
	})
	if result.IsError {
		t.Fatalf("unexpected error at depth 0: %s", result.ForLLM)
	}
	if !cap.called {
		t.Fatal("runAgent should have been called")
	}
	if cap.capturedDepth != 1 {
		t.Errorf("spawned agent should see depth 1, got %d", cap.capturedDepth)
	}
}

func TestDelegateTool_IncrementDepth_FromExisting(t *testing.T) {
	cap := &captureRunAgent{}
	tool := NewDelegateTool(cap.run, nil)

	// Simulate a mid-chain call at depth 2; spawned agent should see 3.
	ctx := WithDelegationDepth(context.Background(), 2)
	result := tool.Execute(ctx, map[string]any{
		"agent": "analyst",
		"task":  "crunch numbers",
	})
	if result.IsError {
		t.Fatalf("unexpected error at depth 2: %s", result.ForLLM)
	}
	if cap.capturedDepth != 3 {
		t.Errorf("spawned agent should see depth 3, got %d", cap.capturedDepth)
	}
}

func TestDelegateTool_RefusesAtCap(t *testing.T) {
	cap := &captureRunAgent{}
	tool := NewDelegateTool(cap.run, nil)

	// Call with depth == MaxDepth — must be refused without calling runAgent.
	ctx := WithDelegationDepth(context.Background(), delegation.MaxDepth)
	result := tool.Execute(ctx, map[string]any{
		"agent": "researcher",
		"task":  "do something",
	})
	if !result.IsError {
		t.Fatal("expected error result at cap depth")
	}
	if !strings.Contains(result.ForLLM, "delegation depth limit reached") {
		t.Errorf("error message should mention depth limit, got: %s", result.ForLLM)
	}
	if cap.called {
		t.Error("runAgent must NOT be called when depth is at the cap")
	}
}

func TestDelegateTool_RefusesAboveCap(t *testing.T) {
	cap := &captureRunAgent{}
	tool := NewDelegateTool(cap.run, nil)

	ctx := WithDelegationDepth(context.Background(), delegation.MaxDepth+2)
	result := tool.Execute(ctx, map[string]any{
		"agent": "writer",
		"task":  "write a report",
	})
	if !result.IsError {
		t.Fatal("expected error result above cap depth")
	}
	if cap.called {
		t.Error("runAgent must NOT be called when depth is above the cap")
	}
}

func TestDelegateTool_LastAllowedDepth(t *testing.T) {
	cap := &captureRunAgent{}
	tool := NewDelegateTool(cap.run, nil)

	// Depth MaxDepth-1 is the last depth at which delegation is still allowed.
	ctx := WithDelegationDepth(context.Background(), delegation.MaxDepth-1)
	result := tool.Execute(ctx, map[string]any{
		"agent": "worker",
		"task":  "execute task",
	})
	if result.IsError {
		t.Fatalf("depth %d should still be allowed: %s", delegation.MaxDepth-1, result.ForLLM)
	}
	if !cap.called {
		t.Fatal("runAgent should have been called at last allowed depth")
	}
	if cap.capturedDepth != delegation.MaxDepth {
		t.Errorf("spawned agent should see depth %d, got %d", delegation.MaxDepth, cap.capturedDepth)
	}
}

func TestDelegationDepthFromCtx_Default(t *testing.T) {
	if d := DelegationDepthFromCtx(context.Background()); d != 0 {
		t.Errorf("absent depth should default to 0, got %d", d)
	}
}

func TestWithDelegationDepth_RoundTrip(t *testing.T) {
	for _, depth := range []int{0, 1, 2, 3, 4, 10} {
		ctx := WithDelegationDepth(context.Background(), depth)
		if got := DelegationDepthFromCtx(ctx); got != depth {
			t.Errorf("depth %d: round-trip got %d", depth, got)
		}
	}
}
