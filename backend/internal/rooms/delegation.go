// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package rooms

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/delegation"
)

// MaxDelegationDepth is the maximum number of delegation hops allowed in a
// single work cascade. The authoritative value lives in the shared
// internal/delegation package (delegation.MaxDepth) so both this package and
// the tools package can reference it without an import cycle.
// CanDelegate(depth, MaxDelegationDepth, true) is true for depths 0–3 and
// false for 4, giving exactly four active hops:
// CEO (0) → COO (1) → officer (2) → worker (3) → sub-task runner (4 refused).
const MaxDelegationDepth = delegation.MaxDepth

// IsSubordinate reports whether workerID is among the head's direct subordinates.
// (Direct manager_id only — matches agent.GetSubordinates.) Pure.
func IsSubordinate(workerID string, subordinates []*agent.Agent) bool {
	for _, s := range subordinates {
		if s.ID == workerID {
			return true
		}
	}
	return false
}

// CanDelegate reports whether a delegation may proceed at the given depth, with
// the configured max depth and whether the room budget currently allows a turn.
// Pure.
func CanDelegate(depth, maxDepth int, budgetOK bool) bool {
	if !budgetOK {
		return false
	}
	return depth < maxDepth
}

// RunMeta is the goroutine-local context the orchestrator threads through a
// delegation chain (it owns each goroutine, so this is not on RunRequest).
type RunMeta struct {
	RoomID     string
	WorkItemID string
	Depth      int
	HeadID     string // the delegating head (for roll-up)
}

// AgentRunner runs an agent on a task and returns its text result. The gateway
// implements it over agent.Loop.Run; tests inject a fake. This is the seam that
// keeps the whole orchestrator unit-testable without an LLM.
type AgentRunner interface {
	Run(ctx context.Context, agentID, task string) (result string, err error)
}

// Orchestrator drives the head→L3 delegation loop. All side effects are injected
// as function fields so the whole loop is unit-testable with fakes + no LLM. The
// gateway wires these to the real stores and agent loop.
type Orchestrator struct {
	Runner AgentRunner

	Subordinates       func(ctx context.Context, headID string) ([]*agent.Agent, error)
	CreateWorkItem     func(ctx context.Context, ownerID, origin, requestedBy, title string) (string, error)
	TransitionWorkItem func(ctx context.Context, id, to, actor, detail string) error
	PostRoom           func(ctx context.Context, roomID, senderID, senderType, content string)
	PostHub            func(ctx context.Context, content string) bool
	RunHeadRollup      func(ctx context.Context, headID, prompt string) (string, error)
	BudgetOK           func(ctx context.Context, roomID string) bool
	RecordTurn         func(ctx context.Context, roomID, agentID string)
}

// DelegateInput is one delegation request from a head's delegate_work tool call.
type DelegateInput struct {
	HeadID string
	Worker string
	Task   string
	RoomID string
	Depth  int
}

// DelegateResult is returned synchronously to the head's tool call.
type DelegateResult struct {
	WorkItemID string
	WorkerKey  string
	Error      string
}

// DelegateWork validates and runs a delegation: resolve+guard the worker, gate on
// depth+budget, create the work item, post the hand-off note, run the L3, post its
// report, close the work item, and roll a summary up to the hub. Synchronous.
func (o *Orchestrator) DelegateWork(ctx context.Context, in DelegateInput) DelegateResult {
	subs, err := o.Subordinates(ctx, in.HeadID)
	if err != nil {
		return DelegateResult{Error: "could not load your team"}
	}
	worker := ResolveMention(in.Worker, subs)
	if worker == nil || !IsSubordinate(worker.ID, subs) {
		return DelegateResult{Error: fmt.Sprintf("%q is not on your team — you can only delegate to your direct reports", in.Worker)}
	}
	budgetOK := o.BudgetOK(ctx, in.RoomID)
	if !CanDelegate(in.Depth, MaxDelegationDepth, budgetOK) {
		if !budgetOK {
			return DelegateResult{Error: "this room has hit its activity limit for now — try again shortly"}
		}
		return DelegateResult{Error: "delegation depth limit reached — cannot delegate further from here"}
	}

	wiID, err := o.CreateWorkItem(ctx, worker.ID, "room:"+in.RoomID, in.HeadID, in.Task)
	if err != nil {
		return DelegateResult{Error: "could not create the work item"}
	}
	o.RecordTurn(ctx, in.RoomID, worker.ID)
	o.PostRoom(ctx, in.RoomID, "system", "system", fmt.Sprintf("→ assigned to @%s: %s", worker.AgentKey, in.Task))

	// Detach from the request context so a client disconnect can't strand the
	// work item mid-chain — the L3 run + transitions must always complete.
	o.runWorkerAndReport(context.WithoutCancel(ctx), RunMeta{RoomID: in.RoomID, WorkItemID: wiID, Depth: in.Depth + 1, HeadID: in.HeadID}, worker, in.Task)

	return DelegateResult{WorkItemID: wiID, WorkerKey: worker.AgentKey}
}

// runWorkerAndReport runs the L3, posts its result in the room, closes the work
// item, and wakes the head once for the roll-up. The legal close path is
// assigned→in_progress→in_review→done (done is only reachable via in_review);
// a failed run goes assigned→in_progress→blocked. Each step only proceeds if the
// prior one succeeded, so a transition error can never leave an illegal jump.
func (o *Orchestrator) runWorkerAndReport(ctx context.Context, meta RunMeta, worker *agent.Agent, task string) {
	result, err := o.Runner.Run(ctx, worker.ID, task)
	if err != nil || result == "" {
		o.PostRoom(ctx, meta.RoomID, worker.AgentKey, "soul", "I couldn't complete that — flagging it for review.")
		if terr := o.TransitionWorkItem(ctx, meta.WorkItemID, "in_progress", worker.ID, "started"); terr != nil {
			slog.Warn("rooms.delegation.transition_inprogress_failed", "work_item", meta.WorkItemID, "err", terr)
		} else {
			_ = o.TransitionWorkItem(ctx, meta.WorkItemID, "blocked", worker.ID, "run failed")
		}
		slog.Warn("rooms.delegation.worker_failed", "work_item", meta.WorkItemID, "err", err)
		return
	}
	o.PostRoom(ctx, meta.RoomID, worker.AgentKey, "soul", result)
	// Walk the legal close path: in_progress → in_review → done.
	if terr := o.TransitionWorkItem(ctx, meta.WorkItemID, "in_progress", worker.ID, "started"); terr != nil {
		slog.Warn("rooms.delegation.transition_inprogress_failed", "work_item", meta.WorkItemID, "err", terr)
	} else if terr := o.TransitionWorkItem(ctx, meta.WorkItemID, "in_review", worker.ID, "reported"); terr != nil {
		slog.Warn("rooms.delegation.transition_inreview_failed", "work_item", meta.WorkItemID, "err", terr)
	} else if terr := o.TransitionWorkItem(ctx, meta.WorkItemID, "done", worker.ID, "accepted"); terr != nil {
		slog.Warn("rooms.delegation.transition_done_failed", "work_item", meta.WorkItemID, "err", terr)
	}
	o.rollUp(ctx, meta, worker, task)
}

// rollUp wakes the head summary-only and posts its one-line summary to the hub.
func (o *Orchestrator) rollUp(ctx context.Context, meta RunMeta, worker *agent.Agent, task string) {
	prompt := fmt.Sprintf("Your team member @%s just completed: %q. Post a single one-line completion summary for company leadership. No tools, just the summary line.", worker.AgentKey, task)
	summary, err := o.RunHeadRollup(ctx, meta.HeadID, prompt)
	if err != nil || summary == "" {
		summary = fmt.Sprintf("✅ %s — completed by @%s", task, worker.AgentKey)
	}
	if !o.PostHub(ctx, summary) {
		slog.Info("rooms.delegation.no_hub", "work_item", meta.WorkItemID)
	}
}
