// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"sync"
	"testing"
)

// TestSetState_FiresCallbackOnTransition verifies that the onStateChange callback
// fires exactly once per real state transition and is skipped for no-op transitions.
func TestSetState_FiresCallbackOnTransition(t *testing.T) {
	type call struct {
		agentID string
		state   RuntimeState
	}

	var mu sync.Mutex
	var calls []call

	rt := NewAgentRuntime("agent-1", "tenant-1", func(_ context.Context, _ string, _ WakeupSignal) {})
	rt.onStateChange = func(agentID string, state RuntimeState) {
		mu.Lock()
		calls = append(calls, call{agentID, state})
		mu.Unlock()
	}

	// idle → working (real transition — should fire)
	rt.setState(RuntimeWorking)
	// working → working (no-op — should NOT fire)
	rt.setState(RuntimeWorking)
	// working → idle (real transition — should fire)
	rt.setState(RuntimeIdle)

	mu.Lock()
	n := len(calls)
	mu.Unlock()

	if n != 2 {
		t.Fatalf("expected 2 callback calls, got %d: %+v", n, calls)
	}

	mu.Lock()
	c0, c1 := calls[0], calls[1]
	mu.Unlock()

	if c0.agentID != "agent-1" || c0.state != RuntimeWorking {
		t.Errorf("call[0]: want (agent-1, working), got (%s, %s)", c0.agentID, c0.state)
	}
	if c1.agentID != "agent-1" || c1.state != RuntimeIdle {
		t.Errorf("call[1]: want (agent-1, idle), got (%s, %s)", c1.agentID, c1.state)
	}
}

// TestSetState_NilCallbackSafe verifies setState does not panic when no callback is set.
func TestSetState_NilCallbackSafe(t *testing.T) {
	rt := NewAgentRuntime("agent-2", "tenant-1", func(_ context.Context, _ string, _ WakeupSignal) {})
	// onStateChange is nil by default — must not panic
	rt.setState(RuntimeWorking)
	rt.setState(RuntimeSuspended)
	rt.setState(RuntimeIdle)
}

// TestRuntimeManager_SetOnStateChange_Propagates verifies that SetOnStateChange
// wires the callback onto newly created runtimes via EnsureRuntime.
func TestRuntimeManager_SetOnStateChange_Propagates(t *testing.T) {
	type call struct {
		agentID string
		state   RuntimeState
	}

	var mu sync.Mutex
	var calls []call

	mgr := NewRuntimeManager(context.Background(), func(_ context.Context, _ string, _ WakeupSignal) {})
	mgr.SetOnStateChange(func(agentID string, state RuntimeState) {
		mu.Lock()
		calls = append(calls, call{agentID, state})
		mu.Unlock()
	})

	rt := mgr.EnsureRuntime("agent-3", "tenant-1")

	// Trigger a state transition directly (white-box — same package).
	rt.setState(RuntimeWorking)
	rt.setState(RuntimeIdle)

	mu.Lock()
	n := len(calls)
	mu.Unlock()

	// We may also receive the idle→idle no-op from Run() starting, but we triggered
	// idle→working and working→idle, so at minimum 2 calls must be present.
	if n < 2 {
		t.Fatalf("expected at least 2 callback calls, got %d: %+v", n, calls)
	}
}

// TestRuntimeManager_SetOnStateChange_BackFills verifies that SetOnStateChange
// back-fills existing runtimes created before the setter was called.
func TestRuntimeManager_SetOnStateChange_BackFills(t *testing.T) {
	mgr := NewRuntimeManager(context.Background(), func(_ context.Context, _ string, _ WakeupSignal) {})
	// Create runtime BEFORE registering the callback.
	rt := mgr.EnsureRuntime("agent-4", "tenant-1")

	var fired bool
	mgr.SetOnStateChange(func(_ string, _ RuntimeState) {
		fired = true
	})

	// After back-fill the runtime should have the callback.
	rt.setState(RuntimeWorking)

	if !fired {
		t.Fatal("callback was not back-filled onto pre-existing runtime")
	}
}
