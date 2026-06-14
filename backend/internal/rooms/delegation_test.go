package rooms

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/workitems"
)

func ptr(s string) *string { return &s }

func TestIsSubordinate(t *testing.T) {
	head := "h1"
	subs := []*agent.Agent{
		{ID: "w1", ManagerID: ptr("h1")},
		{ID: "w2", ManagerID: ptr("h1")},
	}
	if !IsSubordinate("w1", subs) {
		t.Errorf("w1 should be a subordinate")
	}
	if IsSubordinate("x9", subs) {
		t.Errorf("x9 is not in the subordinate list")
	}
	if IsSubordinate(head, subs) {
		t.Errorf("the head is not its own subordinate")
	}
	if IsSubordinate("w1", nil) {
		t.Errorf("no subordinates → false")
	}
}

func TestCanDelegate(t *testing.T) {
	// Verify the four-level cascade semantics (CEO→COO→officer→worker→sub-task).
	// Depths 0–3 must be allowed; depth 4 must be refused.
	for depth := 0; depth < MaxDelegationDepth; depth++ {
		if !CanDelegate(depth, MaxDelegationDepth, true) {
			t.Errorf("depth %d should be allowed (< MaxDelegationDepth %d)", depth, MaxDelegationDepth)
		}
	}
	if CanDelegate(MaxDelegationDepth, MaxDelegationDepth, true) {
		t.Errorf("depth %d == MaxDelegationDepth → must be refused", MaxDelegationDepth)
	}
	if CanDelegate(MaxDelegationDepth+1, MaxDelegationDepth, true) {
		t.Errorf("depth %d > MaxDelegationDepth → must be refused", MaxDelegationDepth+1)
	}
	if CanDelegate(0, MaxDelegationDepth, false) {
		t.Errorf("budget not ok → denied regardless of depth")
	}
}

// fakeRunner records runs and returns canned results keyed by agent id.
type fakeRunner struct {
	mu      sync.Mutex
	results map[string]string
	ran     []string
}

func (f *fakeRunner) Run(ctx context.Context, agentID, task string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ran = append(f.ran, agentID)
	if r, ok := f.results[agentID]; ok {
		return r, nil
	}
	return "ok", nil
}

func TestOrchestrator_DelegateWork_FullLoop(t *testing.T) {
	var posts []string
	var hubPosts []string
	var transitions []string
	wiCreated := ""
	wiStatus := "assigned" // tracks the work item's status for CanTransition validation

	runner := &fakeRunner{results: map[string]string{"w1": "rebuilt the flow"}}

	o := &Orchestrator{
		Runner: runner,
		Subordinates: func(ctx context.Context, headID string) ([]*agent.Agent, error) {
			return []*agent.Agent{{ID: "w1", AgentKey: "eng"}}, nil
		},
		CreateWorkItem: func(ctx context.Context, ownerID, origin, requestedBy, title string) (string, error) {
			wiCreated = ownerID + "|" + origin
			return "wi-1", nil
		},
		// Validate against the REAL work-item status guard so an illegal
		// transition sequence (e.g. in_progress→done) fails the test rather
		// than being silently accepted.
		TransitionWorkItem: func(ctx context.Context, id, to, actor, detail string) error {
			from := wiStatus
			if !workitems.CanTransition(from, to) {
				return fmt.Errorf("illegal transition %s→%s", from, to)
			}
			wiStatus = to
			transitions = append(transitions, id+"|"+to)
			return nil
		},
		PostRoom: func(ctx context.Context, roomID, senderID, senderType, content string) {
			posts = append(posts, senderID+"|"+content)
		},
		PostHub: func(ctx context.Context, content string) bool {
			hubPosts = append(hubPosts, content)
			return true
		},
		RunHeadRollup: func(ctx context.Context, headID, prompt string) (string, error) {
			return "Done: onboarding rebuilt — @eng", nil
		},
		BudgetOK:   func(ctx context.Context, roomID string) bool { return true },
		RecordTurn: func(ctx context.Context, roomID, agentID string) {},
	}

	res := o.DelegateWork(context.Background(), DelegateInput{
		HeadID: "h1", Worker: "eng", Task: "rebuild onboarding", RoomID: "room1", Depth: 0,
	})
	if res.Error != "" {
		t.Fatalf("delegate errored: %s", res.Error)
	}
	if res.WorkItemID != "wi-1" {
		t.Fatalf("expected work item wi-1, got %q", res.WorkItemID)
	}
	if wiCreated != "w1|room:room1" {
		t.Errorf("work item owner/origin wrong: %q", wiCreated)
	}
	if len(runner.ran) == 0 || runner.ran[0] != "w1" {
		t.Fatalf("expected L3 w1 to run, got %v", runner.ran)
	}
	foundResult := false
	for _, p := range posts {
		if p == "eng|rebuilt the flow" {
			foundResult = true
		}
	}
	if !foundResult {
		t.Errorf("L3 result not posted in room; posts=%v", posts)
	}
	// Legal close path: in_progress → in_review → done.
	wantT := []string{"wi-1|in_progress", "wi-1|in_review", "wi-1|done"}
	if len(transitions) != len(wantT) {
		t.Fatalf("transitions: want %v got %v", wantT, transitions)
	}
	for i := range wantT {
		if transitions[i] != wantT[i] {
			t.Errorf("transition %d: want %s got %s", i, wantT[i], transitions[i])
		}
	}
	if len(hubPosts) != 1 {
		t.Errorf("expected 1 hub roll-up, got %v", hubPosts)
	}
}

func TestOrchestrator_DelegateWork_RejectsNonSubordinate(t *testing.T) {
	o := &Orchestrator{
		Runner: &fakeRunner{},
		Subordinates: func(ctx context.Context, headID string) ([]*agent.Agent, error) {
			return []*agent.Agent{{ID: "w1", AgentKey: "eng"}}, nil
		},
		BudgetOK:   func(ctx context.Context, roomID string) bool { return true },
		RecordTurn: func(ctx context.Context, roomID, agentID string) {},
	}
	res := o.DelegateWork(context.Background(), DelegateInput{
		HeadID: "h1", Worker: "cmo", Task: "x", RoomID: "room1", Depth: 0,
	})
	if res.Error == "" {
		t.Fatalf("delegating to a non-subordinate should error")
	}
}

func TestOrchestrator_DelegateWork_RefusesOverDepth(t *testing.T) {
	o := &Orchestrator{
		Runner: &fakeRunner{},
		Subordinates: func(ctx context.Context, headID string) ([]*agent.Agent, error) {
			return []*agent.Agent{{ID: "w1", AgentKey: "eng"}}, nil
		},
		BudgetOK:   func(ctx context.Context, roomID string) bool { return true },
		RecordTurn: func(ctx context.Context, roomID, agentID string) {},
	}
	res := o.DelegateWork(context.Background(), DelegateInput{
		HeadID: "h1", Worker: "eng", Task: "x", RoomID: "room1", Depth: MaxDelegationDepth,
	})
	if res.Error == "" {
		t.Fatalf("delegating at/over max depth should be refused")
	}
}

func TestOrchestrator_DelegateWork_WorkerFails(t *testing.T) {
	var transitions []string
	var hubPosts []string
	// runner returns an error for the worker.
	failing := &failingRunner{}
	o := &Orchestrator{
		Runner: failing,
		Subordinates: func(ctx context.Context, headID string) ([]*agent.Agent, error) {
			return []*agent.Agent{{ID: "w1", AgentKey: "eng"}}, nil
		},
		CreateWorkItem: func(ctx context.Context, ownerID, origin, requestedBy, title string) (string, error) {
			return "wi-1", nil
		},
		TransitionWorkItem: func(ctx context.Context, id, to, actor, detail string) error {
			transitions = append(transitions, id+"|"+to)
			return nil
		},
		PostRoom:   func(ctx context.Context, roomID, senderID, senderType, content string) {},
		PostHub:    func(ctx context.Context, content string) bool { hubPosts = append(hubPosts, content); return true },
		BudgetOK:   func(ctx context.Context, roomID string) bool { return true },
		RecordTurn: func(ctx context.Context, roomID, agentID string) {},
	}
	res := o.DelegateWork(context.Background(), DelegateInput{HeadID: "h1", Worker: "eng", Task: "x", RoomID: "room1", Depth: 0})
	if res.Error != "" {
		t.Fatalf("delegate should not error just because the worker run fails: %s", res.Error)
	}
	// failure path: in_progress then blocked, NOT done.
	want := []string{"wi-1|in_progress", "wi-1|blocked"}
	if len(transitions) != 2 || transitions[0] != want[0] || transitions[1] != want[1] {
		t.Errorf("failure transitions: want %v got %v", want, transitions)
	}
	// no roll-up on failure.
	if len(hubPosts) != 0 {
		t.Errorf("no hub roll-up should fire on worker failure, got %v", hubPosts)
	}
}

type failingRunner struct{}

func (failingRunner) Run(ctx context.Context, agentID, task string) (string, error) {
	return "", errors.New("worker boom")
}
