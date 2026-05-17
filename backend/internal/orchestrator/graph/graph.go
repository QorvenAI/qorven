// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package graph is the plan execution runtime. It reads a plan +
// nodes + edges from internal/plans and drives them to completion.
//
// The runtime supports:
//
//   - Per-node handlers registered by NodeKind. Handlers return either a
//     Command (state mutation) or a PauseSignal (park until resumed).
//   - Conditional edges. After a node returns success, the runtime picks
//     the outgoing edge whose condition matches the handler's Outcome.
//   - Resume. A paused plan is resumed by calling Run again — the runtime
//     re-scans nodes looking for ones whose upstream has become
//     resolvable.
//   - Cancellation. ctx.Done() is honored at every hop; the run exits
//     with context.Canceled and no further DB writes.
package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/plans"
)

// Outcome is the handler's verdict on a node. It drives edge selection:
// the runtime follows every outgoing edge whose Condition matches the
// Outcome (plus "always" edges). Multiple matching edges fan out.
type Outcome string

const (
	OutcomeSuccess  Outcome = "on_success"
	OutcomeError    Outcome = "on_error"
	OutcomeApproved Outcome = "approved"
	OutcomeRejected Outcome = "rejected"
	OutcomeRevision Outcome = "revision"
)

// PauseSignal is returned by handlers that cannot complete synchronously
// — typically human_feedback waiting for user approval. The runtime
// records the pause and exits the run. A later call to ResumePlan (via
// external event, e.g. approval resolved) re-enters the runtime.
type PauseSignal struct {
	Reason   string // human-readable
	Metadata map[string]any
}

// Error satisfies the error interface so handlers can `return nil, &PauseSignal{}`.
func (p *PauseSignal) Error() string {
	if p.Reason == "" {
		return "graph: paused"
	}
	return "graph: paused: " + p.Reason
}

// Handler executes a single node. Returns (outcome, artifacts, err).
//   - outcome drives edge selection when err == nil.
//   - artifacts is merged into plan_nodes.artifacts atomically.
//   - err != nil puts the node in state=failed UNLESS the error is a
//     *PauseSignal, in which case the node is set to state=blocked and
//     the run exits cleanly (returns ErrPaused).
type Handler func(ctx context.Context, h *HandlerContext) (outcome Outcome, artifacts any, err error)

// HandlerContext gives handlers access to the plan, node, and the
// shared services they need to do real work.
type HandlerContext struct {
	Plan      *plans.Plan
	Node      *plans.Node
	Plans     *plans.Store
	Approvals *approvals.Store

	// Emit records progress on the node without transitioning state.
	// Handlers call this to signal long-running progress; the runtime
	// turns the call into a graph-level event (agent.progress, etc.).
	Emit func(kind string, detail map[string]any)

	// Logger is scoped to the plan+node.
	Logger *slog.Logger
}

// Registry holds Handlers keyed by plans.NodeKind.
type Registry struct {
	handlers map[plans.NodeKind]Handler
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry { return &Registry{handlers: map[plans.NodeKind]Handler{}} }

// Register installs a handler for a kind. Replaces any existing handler
// for that kind. Returns the registry for fluent chaining.
func (r *Registry) Register(kind plans.NodeKind, h Handler) *Registry {
	r.handlers[kind] = h
	return r
}

// Handler returns the registered handler, or nil.
func (r *Registry) Handler(kind plans.NodeKind) Handler { return r.handlers[kind] }

// Config wires the runtime's dependencies.
type Config struct {
	Plans     *plans.Store
	Approvals *approvals.Store
	Registry  *Registry
	Logger    *slog.Logger

	// Emit is the plan-level progress hook. Handlers' Emit bubbles up
	// to here after being decorated with node metadata. ctx carries any
	// actor hint set by the caller (e.g. sweeper.resumeOne via ActorKey).
	Emit func(ctx context.Context, planID, nodeID, kind string, detail map[string]any)

	// MaxHopsPerRun bounds how many nodes a single Run call can execute
	// before it exits voluntarily (protects against misconfigured cycles).
	// Zero means 256.
	MaxHopsPerRun int
}

// Runtime is the per-plan execution engine. One Runtime instance is
// shared across all plans.
type Runtime struct {
	cfg Config
}

// NewRuntime builds a Runtime.
func NewRuntime(cfg Config) *Runtime {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.MaxHopsPerRun <= 0 {
		cfg.MaxHopsPerRun = byom.Load().GraphMaxHops
	}
	if cfg.Registry == nil {
		cfg.Registry = NewRegistry()
	}
	return &Runtime{cfg: cfg}
}

// Run executes the plan starting from whatever nodes are currently
// runnable (pending state + all upstream ends resolved).
//
// Contract:
//   - Returns nil when the plan has reached a terminal state (done /
//     cancelled) or when MaxHopsPerRun is exhausted.
//   - Returns ErrPaused when the run is waiting on human_feedback.
//     Callers resume by invoking Run again once the approval flips.
//   - Returns ctx.Err() on cancellation; any in-flight node is put
//     into state=cancelled and the run unwinds cleanly.
//   - Returns a handler error for fatal node failures (wrapped with
//     the plan+node context).
func (r *Runtime) Run(ctx context.Context, planID string) error {
	if planID == "" {
		return errors.New("graph: plan_id required")
	}
	plan, err := r.cfg.Plans.GetPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("graph: load plan: %w", err)
	}
	if plan.Status == plans.StatusDone || plan.Status == plans.StatusFailed || plan.Status == plans.StatusCancelled {
		return nil // nothing to do
	}

	logger := r.cfg.Logger.With("plan_id", planID, "tenant_id", plan.TenantID)

	// Transition to Running on first execution. Idempotent.
	if plan.Status != plans.StatusRunning {
		if _, err := r.cfg.Plans.UpdatePlanStatus(ctx, planID, plans.StatusRunning); err != nil {
			if !plans.IsIllegalTransition(err) {
				return fmt.Errorf("graph: plan→running: %w", err)
			}
		}
	}

	hops := 0
	paused := false
	// Nodes that paused during THIS run are not re-dispatched within
	// the same call — pause always yields back to the caller. Resuming
	// happens on the next Run invocation.
	pausedInThisRun := make(map[string]bool)
	for hops < r.cfg.MaxHopsPerRun {
		if err := ctx.Err(); err != nil {
			return err
		}

		nodes, err := r.cfg.Plans.ListNodesByPlan(ctx, planID)
		if err != nil {
			return fmt.Errorf("graph: list nodes: %w", err)
		}

		runnable, err := r.selectRunnable(ctx, plan, nodes)
		if err != nil {
			return err
		}
		// Filter out nodes we've already paused in this run — the caller
		// owns the resume trigger.
		filtered := runnable[:0]
		for _, n := range runnable {
			if !pausedInThisRun[n.ID] {
				filtered = append(filtered, n)
			}
		}
		runnable = filtered
		if len(runnable) == 0 {
			// Either the plan is complete, or every remaining runnable
			// node paused earlier in this run.
			break
		}

		for _, node := range runnable {
			if err := ctx.Err(); err != nil {
				return err
			}
			handler := r.cfg.Registry.Handler(node.Kind)
			if handler == nil {
				markErr := fmt.Errorf("graph: no handler for node kind %q", node.Kind)
				r.fail(ctx, node, markErr)
				return markErr
			}
			hops++

			// Transition pending → running (or blocked → running).
			if _, err := r.cfg.Plans.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
				NodeID: node.ID, Next: plans.NodeRunning,
			}); err != nil {
				if !plans.IsIllegalTransition(err) {
					return fmt.Errorf("graph: node→running: %w", err)
				}
			}
			if r.cfg.Emit != nil {
				r.cfg.Emit(ctx, plan.ID, node.ID, "node.started", map[string]any{
					"kind":  string(node.Kind),
					"title": node.Title,
				})
			}

			hctx := &HandlerContext{
				Plan:      plan,
				Node:      node,
				Plans:     r.cfg.Plans,
				Approvals: r.cfg.Approvals,
				Emit: func(kind string, detail map[string]any) {
					if r.cfg.Emit != nil {
						r.cfg.Emit(ctx, plan.ID, node.ID, kind, detail)
					}
				},
				Logger: logger.With("node_id", node.ID, "kind", string(node.Kind)),
			}

			outcome, artifacts, hErr := handler(ctx, hctx)

			// Pause signal: block node, exit run cleanly.
			var pause *PauseSignal
			if errors.As(hErr, &pause) {
				blockArtifacts := artifactsJSON(artifacts)
				if _, err := r.cfg.Plans.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
					NodeID: node.ID, Next: plans.NodeBlocked, Artifacts: blockArtifacts,
				}); err != nil {
					return fmt.Errorf("graph: node→blocked: %w", err)
				}
				if r.cfg.Emit != nil {
					r.cfg.Emit(ctx, plan.ID, node.ID, "node.paused", map[string]any{
						"reason": pause.Reason,
					})
				}
				paused = true
				pausedInThisRun[node.ID] = true
				continue
			}

			if hErr != nil {
				// Handler-level failure is fatal to the plan UNLESS the
				// handler emits OutcomeError AND an outgoing on_error
				// edge exists — in that case the graph routes to an
				// error-handling sibling.
				if outcome == OutcomeError && r.hasOutgoingErrorEdge(ctx, plan.ID, node.ID) {
					// Record outcome in artifacts so the on_error path has
					// an audit trail of what was intended (FU-027).
					errorArtifacts := artifactsWithOutcome(artifacts, outcome)
					r.failWithArtifacts(ctx, node, hErr, errorArtifacts)
					continue
				}
				r.fail(ctx, node, hErr)
				_, _ = r.cfg.Plans.UpdatePlanStatus(ctx, planID, plans.StatusFailed)
				return fmt.Errorf("graph: node %s failed: %w", node.ID, hErr)
			}

			// Terminal success: node → done, merge artifacts.
			doneArtifacts := artifactsJSON(artifacts)
			if _, err := r.cfg.Plans.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
				NodeID: node.ID, Next: plans.NodeDone, Artifacts: doneArtifacts,
			}); err != nil {
				return fmt.Errorf("graph: node→done: %w", err)
			}
			if r.cfg.Emit != nil {
				r.cfg.Emit(ctx, plan.ID, node.ID, "node.completed", map[string]any{
					"outcome": string(outcome),
				})
			}
		}
	}

	if hops >= r.cfg.MaxHopsPerRun {
		return fmt.Errorf("graph: plan %s exceeded max hops (%d) — suspected cycle",
			planID, r.cfg.MaxHopsPerRun)
	}

	// Assess terminal status.
	if paused {
		return ErrPaused
	}
	nodes, err := r.cfg.Plans.ListNodesByPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("graph: final status: %w", err)
	}
	if allTerminal(nodes) {
		if anyFailed(nodes) {
			_, _ = r.cfg.Plans.UpdatePlanStatus(ctx, planID, plans.StatusFailed)
		} else {
			_, _ = r.cfg.Plans.UpdatePlanStatus(ctx, planID, plans.StatusDone)
		}
	}
	return nil
}

// selectRunnable walks the plan's nodes and returns those that are
// immediately executable: state is pending (or blocked after a pause),
// and every upstream node resolves with a matching outgoing edge.
func (r *Runtime) selectRunnable(ctx context.Context, plan *plans.Plan, nodes []*plans.Node) ([]*plans.Node, error) {
	byID := make(map[string]*plans.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	edges, err := r.cfg.Plans.ListEdgesByPlan(ctx, plan.ID)
	if err != nil {
		return nil, err
	}
	// Build reverse adjacency: node id → incoming edges.
	incoming := make(map[string][]*plans.Edge, len(nodes))
	for _, e := range edges {
		incoming[e.ToNode] = append(incoming[e.ToNode], e)
	}

	var runnable []*plans.Node
	for _, n := range nodes {
		if n.State != plans.NodePending && n.State != plans.NodeBlocked {
			continue
		}
		// Root nodes (no incoming edges) are runnable as long as they
		// have a handler — otherwise the runtime would loop. We let the
		// caller build the plan with at least one runnable root.
		ins := incoming[n.ID]
		if len(ins) == 0 {
			runnable = append(runnable, n)
			continue
		}
		allSatisfied := true
		for _, e := range ins {
			parent := byID[e.FromNode]
			if parent == nil {
				allSatisfied = false
				break
			}
			if !edgeSatisfied(parent, e) {
				allSatisfied = false
				break
			}
		}
		if allSatisfied {
			runnable = append(runnable, n)
		}
	}
	return runnable, nil
}

// edgeSatisfied reports whether the parent's state satisfies the edge
// condition. "always" requires parent == done; "approved"/"rejected"/etc.
// require the approvals state for the parent's node. The condition
// evaluation here is intentionally simple — the graph does not model
// arbitrary expressions.
func edgeSatisfied(parent *plans.Node, e *plans.Edge) bool {
	if e.Condition == plans.CondOnError {
		return parent.State == plans.NodeFailed
	}
	if parent.State != plans.NodeDone {
		return false
	}
	switch e.Condition {
	case plans.CondAlways, plans.CondOnSuccess:
		return true
	case plans.CondApproved:
		// Parent was a human_feedback node; its artifacts carry the
		// approval verdict.
		return artifactHasString(parent.Artifacts, "approval", "approved")
	case plans.CondRejected:
		return artifactHasString(parent.Artifacts, "approval", "rejected")
	case plans.CondRevision:
		return artifactHasString(parent.Artifacts, "approval", "revision_requested")
	}
	return false
}

// hasOutgoingErrorEdge reports whether the node has an outgoing edge
// with condition=on_error. When true, a handler that returns
// OutcomeError can continue execution via that edge; otherwise failure
// is fatal to the plan. We check the plans store for the edge list.
func (r *Runtime) hasOutgoingErrorEdge(ctx context.Context, planID, nodeID string) bool {
	edges, err := r.cfg.Plans.OutgoingEdges(ctx, planID, nodeID)
	if err != nil {
		return false
	}
	for _, e := range edges {
		if e.Condition == plans.CondOnError {
			return true
		}
	}
	return false
}

// fail transitions the node to failed and emits the node.failed event.
func (r *Runtime) fail(ctx context.Context, node *plans.Node, err error) {
	r.failWithArtifacts(ctx, node, err, nil)
}

// failWithArtifacts is fail with optional artifacts (used when routing
// via an on_error edge so the outcome is preserved for audit — FU-027).
func (r *Runtime) failWithArtifacts(ctx context.Context, node *plans.Node, err error, artifacts []byte) {
	if _, uErr := r.cfg.Plans.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
		NodeID: node.ID, Next: plans.NodeFailed, Error: err.Error(), Artifacts: artifacts,
	}); uErr != nil {
		r.cfg.Logger.Error("graph: failed to mark node failed",
			"plan_id", node.PlanID, "node_id", node.ID, "err", uErr)
	}
	if r.cfg.Emit != nil {
		r.cfg.Emit(ctx, node.PlanID, node.ID, "node.failed", map[string]any{
			"error": err.Error(),
		})
	}
	_ = time.Now() // reserved for latency instrumentation
}

// artifactsWithOutcome merges the handler's artifacts map with an
// "outcome" key set to the handler's Outcome. Used by the on_error
// routing path so the audit record shows what the handler intended.
func artifactsWithOutcome(artifacts any, outcome Outcome) []byte {
	var m map[string]any
	switch v := artifacts.(type) {
	case map[string]any:
		m = make(map[string]any, len(v)+1)
		for k, val := range v {
			m[k] = val
		}
	default:
		m = make(map[string]any, 1)
	}
	m["outcome"] = string(outcome)
	return artifactsJSON(m)
}

// ErrPaused signals the caller that Run exited because one or more
// nodes returned a PauseSignal. Call Run again when upstream signals
// make one of those nodes runnable.
var ErrPaused = errors.New("graph: paused")
