// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package daemon

import (
	"context"
	"testing"
	"time"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func newReg() *Registry { return New() }

// stubProvider is a no-op AgentProvider for tests.
type stubProvider struct {
	id   string
	caps []string
}

func (s *stubProvider) ProviderID() string      { return s.id }
func (s *stubProvider) Name() string             { return s.id }
func (s *stubProvider) Capabilities() []string   { return s.caps }
func (s *stubProvider) Dispatch(_ context.Context, _ *Task) error { return nil }
func (s *stubProvider) Cancel(_ context.Context, _ string) error  { return nil }
func (s *stubProvider) Ping(_ context.Context) error              { return nil }

func registerAgent(reg *Registry, caps ...string) *AgentInstance {
	return reg.Register("test-agent", "stub", "model-1", caps, &stubProvider{id: "stub", caps: caps})
}

// ─── registration ─────────────────────────────────────────────────────────────

func TestRegisterUnregister(t *testing.T) {
	reg := newReg()

	inst := registerAgent(reg, "code", "frontend")
	if inst.ID == "" {
		t.Fatal("expected non-empty agent ID")
	}
	if inst.Status != StatusIdle {
		t.Fatalf("expected idle, got %s", inst.Status)
	}
	if len(reg.ListAgents()) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(reg.ListAgents()))
	}

	reg.Unregister(inst.ID, "test")
	if len(reg.ListAgents()) != 0 {
		t.Fatal("expected 0 agents after unregister")
	}
}

func TestRegisterFullUUID(t *testing.T) {
	reg := newReg()
	inst := registerAgent(reg, "code")
	// Full UUID is 36 chars (8-4-4-4-12 + 4 dashes)
	if len(inst.ID) != 36 {
		t.Fatalf("agent ID should be a full UUID (36 chars), got len=%d: %q", len(inst.ID), inst.ID)
	}
}

func TestHeartbeat(t *testing.T) {
	reg := newReg()
	inst := registerAgent(reg)
	if !reg.Heartbeat(inst.ID, StatusWorking) {
		t.Fatal("heartbeat should return true for known agent")
	}
	agents := reg.ListAgents()
	if agents[0].Status != StatusWorking {
		t.Fatalf("expected working, got %s", agents[0].Status)
	}
	if reg.Heartbeat("unknown", StatusIdle) {
		t.Fatal("heartbeat should return false for unknown agent")
	}
}

// ─── task lifecycle ───────────────────────────────────────────────────────────

func TestCreateAndCompleteTask(t *testing.T) {
	reg := newReg()
	inst := registerAgent(reg, "code")

	task := reg.CreateTask("write tests", "desc", inst.ID, "normal", "human", nil, nil)
	if task.ID == "" {
		t.Fatal("expected task ID")
	}
	if len(task.ID) != 36 {
		t.Fatalf("task ID should be full UUID (36 chars), got %d: %q", len(task.ID), task.ID)
	}

	// Give dispatchTo goroutine time to run.
	time.Sleep(10 * time.Millisecond)

	agents := reg.ListAgents()
	if len(agents) == 0 {
		t.Fatal("no agents")
	}

	if !reg.Complete(task.ID, "all done", []string{"main.go"}) {
		t.Fatal("Complete returned false")
	}

	agents = reg.ListAgents()
	if agents[0].Status != StatusIdle {
		t.Fatalf("agent should be idle after complete, got %s", agents[0].Status)
	}
	// TasksDone must be incremented (the bug fix).
	if agents[0].TasksDone != 1 {
		t.Fatalf("TasksDone should be 1 after complete, got %d", agents[0].TasksDone)
	}
	if agents[0].TasksFailed != 0 {
		t.Fatalf("TasksFailed should be 0 after success, got %d", agents[0].TasksFailed)
	}
}

func TestCreateAndFailTask(t *testing.T) {
	reg := newReg()
	inst := registerAgent(reg, "code")

	task := reg.CreateTask("risky work", "desc", inst.ID, "normal", "human", nil, nil)
	time.Sleep(10 * time.Millisecond)

	if !reg.Fail(task.ID, "something broke", false) {
		t.Fatal("Fail returned false")
	}

	agents := reg.ListAgents()
	if agents[0].TasksFailed != 1 {
		t.Fatalf("TasksFailed should be 1 after fail, got %d", agents[0].TasksFailed)
	}
	if agents[0].TasksDone != 0 {
		t.Fatalf("TasksDone should be 0 after fail, got %d", agents[0].TasksDone)
	}
}

func TestFreeAgentBugFix(t *testing.T) {
	// Regression: before the fix, freeAgent set Status=Idle before checking
	// Status==Working, so TasksDone was never incremented.
	reg := newReg()
	inst := registerAgent(reg, "backend")

	// Manually put agent into working state (as dispatchTo would).
	reg.mu.Lock()
	ag := reg.agents[inst.ID]
	ag.Status = StatusWorking
	ag.CurrentTask = "task-123"
	reg.tasks["task-123"] = &Task{ID: "task-123", Owner: inst.ID, Status: TaskInProgress}
	reg.mu.Unlock()

	reg.freeAgent(inst.ID, "task-123", true)

	reg.mu.RLock()
	done := reg.agents[inst.ID].TasksDone
	failed := reg.agents[inst.ID].TasksFailed
	status := reg.agents[inst.ID].Status
	reg.mu.RUnlock()

	if done != 1 {
		t.Fatalf("TasksDone should be 1, got %d", done)
	}
	if failed != 0 {
		t.Fatalf("TasksFailed should be 0, got %d", failed)
	}
	if status != StatusIdle {
		t.Fatalf("status should be idle, got %s", status)
	}
}

// ─── dispatch by capability ───────────────────────────────────────────────────

func TestAutoDispatchByCapability(t *testing.T) {
	reg := newReg()
	_ = reg.Register("backend-agent", "stub", "m", []string{"backend"}, &stubProvider{id: "stub", caps: []string{"backend"}})
	front := reg.Register("frontend-agent", "stub", "m", []string{"frontend"}, &stubProvider{id: "stub", caps: []string{"frontend"}})

	// Create a task requiring "frontend" — should go to front agent.
	task := reg.CreateTask("build UI", "desc", "", "normal", "human", nil, map[string]any{
		"owner_capability": "frontend",
	})
	// Give autoDispatch goroutine time to run.
	time.Sleep(20 * time.Millisecond)

	got := reg.GetTask(task.ID)
	if got == nil {
		t.Fatal("task not found")
	}
	if got.Owner != front.ID {
		t.Fatalf("expected task owner to be frontend-agent %s, got %q", front.ID, got.Owner)
	}
}

func TestAutoDispatchNoMatchingAgent(t *testing.T) {
	reg := newReg()
	// Only code agent, but task needs "design".
	_ = registerAgent(reg, "code")

	task := reg.CreateTask("logo", "desc", "", "normal", "human", nil, map[string]any{
		"owner_capability": "design",
	})
	time.Sleep(20 * time.Millisecond)

	got := reg.GetTask(task.ID)
	// Should remain queued with no owner.
	if got.Status != TaskQueued {
		t.Fatalf("expected queued (no matching agent), got %s", got.Status)
	}
	if got.Owner != "" {
		t.Fatalf("expected empty owner, got %q", got.Owner)
	}
}

// ─── ListTasks ordering ───────────────────────────────────────────────────────

func TestListTasksOrdering(t *testing.T) {
	reg := newReg()
	_ = registerAgent(reg, "code")

	// Create three tasks with slight delays so CreatedAt differs.
	t1 := reg.CreateTask("first", "", "", "normal", "human", nil, nil)
	time.Sleep(2 * time.Millisecond)
	t2 := reg.CreateTask("second", "", "", "normal", "human", nil, nil)
	time.Sleep(2 * time.Millisecond)
	t3 := reg.CreateTask("third", "", "", "normal", "human", nil, nil)

	all := reg.ListTasks("", "", 0)
	if len(all) < 3 {
		t.Fatalf("expected at least 3 tasks, got %d", len(all))
	}

	// Find our three tasks (there may be others created by autoDispatch).
	ids := map[string]int{t1.ID: 0, t2.ID: 0, t3.ID: 0}
	var order []string
	for _, t := range all {
		if _, ok := ids[t.ID]; ok {
			order = append(order, t.ID)
		}
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 known tasks in result, got %d", len(order))
	}
	// Newest first: t3, t2, t1.
	if order[0] != t3.ID || order[1] != t2.ID || order[2] != t1.ID {
		t.Fatalf("wrong order: got %v, want [%s %s %s]", order, t3.ID, t2.ID, t1.ID)
	}
}

func TestListTasksLimit(t *testing.T) {
	reg := newReg()
	for range 5 {
		reg.CreateTask("t", "", "", "normal", "human", nil, nil)
	}
	got := reg.ListTasks("", "", 3)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
}

// ─── plan approval ────────────────────────────────────────────────────────────

func TestPlanSingleTaskAutoApproves(t *testing.T) {
	reg := newReg()
	_ = registerAgent(reg, "code")

	plan := reg.ProposePlan("solo", "one task", "human", []PlanTask{
		{ID: "t1", Title: "do it", OwnerCapability: "code", Priority: "normal"},
	})
	// Single-task plans auto-approve asynchronously.
	time.Sleep(30 * time.Millisecond)

	got := reg.GetPlan(plan.ID)
	if got.Status != PlanApproved && got.Status != PlanExecuting && got.Status != PlanDone {
		t.Fatalf("single-task plan should auto-approve, got %s", got.Status)
	}
}

func TestPlanMultiTaskRequiresApproval(t *testing.T) {
	reg := newReg()
	plan := reg.ProposePlan("multi", "two tasks", "human", []PlanTask{
		{ID: "t1", Title: "task 1", OwnerCapability: "code"},
		{ID: "t2", Title: "task 2", OwnerCapability: "backend"},
	})
	if plan.Status != PlanPending {
		t.Fatalf("multi-task plan should be pending, got %s", plan.Status)
	}

	if !reg.ApprovePlan(plan.ID, "admin", "") {
		t.Fatal("ApprovePlan returned false")
	}
	got := reg.GetPlan(plan.ID)
	if got.Status != PlanApproved {
		t.Fatalf("expected approved, got %s", got.Status)
	}
}

func TestPlanReject(t *testing.T) {
	reg := newReg()
	plan := reg.ProposePlan("cancel-me", "", "human", []PlanTask{
		{ID: "t1", Title: "task 1"},
		{ID: "t2", Title: "task 2"},
	})
	if !reg.RejectPlan(plan.ID, "admin", "nope") {
		t.Fatal("RejectPlan returned false")
	}
	if reg.GetPlan(plan.ID).Status != PlanRejected {
		t.Fatalf("expected rejected")
	}
	// Rejecting again must fail (not pending).
	if reg.RejectPlan(plan.ID, "admin", "") {
		t.Fatal("double-reject should return false")
	}
}

// ─── SSE fan-out ─────────────────────────────────────────────────────────────

func TestSubscribeReceivesEvents(t *testing.T) {
	reg := newReg()
	subID, ch := reg.Subscribe()
	defer reg.Unsubscribe(subID)

	registerAgent(reg, "code")

	select {
	case evt := <-ch:
		if evt.Type != EvtAgentRegistered {
			t.Fatalf("expected agent_registered, got %s", evt.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for agent_registered event")
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	reg := newReg()
	subID, ch := reg.Subscribe()
	reg.Unsubscribe(subID)

	// Channel should be closed.
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("channel should be closed after Unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out — channel not closed")
	}
}
