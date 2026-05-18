// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package orchestrator exposes the plan-graph runtime as a single
// embeddable service. The gateway constructs one Service at boot and
// calls ExecutePlan(planID) from the approval-resolved wakeup path.
package orchestrator

import (
	"context"
	"log/slog"

	"github.com/qorvenai/qorven/internal/agent"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator/graph"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/plans"
)

// actorKey is the context key used to thread the approval actor through
// ExecutePlan → graph emitter for audit-trail enrichment (FU-023).
type actorKey struct{}

// WithActor returns a context that carries the actor string. The graph
// emitter picks it up when emitting node lifecycle events.
func WithActor(ctx context.Context, actor string) context.Context {
	if actor == "" {
		return ctx
	}
	return context.WithValue(ctx, actorKey{}, actor)
}

// actorFromCtx returns the actor stored by WithActor, or "" if unset.
func actorFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(actorKey{}).(string); ok {
		return v
	}
	return ""
}

// Service is the high-level façade the gateway interacts with.
type Service struct {
	runtime *graph.Runtime
	// plans is exposed to sibling code in this package (sweeper) but
	// not to external callers — the field stays lowercase. Tests in
	// this package access it directly.
	plans  *plans.Store
	logger *slog.Logger
}

// NewService builds a Service from the stores + agent runner. Returns
// nil when plans or agent is nil (tests that do not need orchestration).
// prefer NewServiceWithTools when a tenant-tool resolver
// (plugins.Loader) is available — NewService is the back-compat shim
// for tests + installs without dynamic plugins.
func NewService(pl *plans.Store, ap *approvals.Store, agt handlers.AgentRunner, events *apievents.Emitter, logger *slog.Logger) *Service {
	return NewServiceWithTools(pl, ap, agt, nil, events, logger)
}

// NewServiceWithTools is the Phase 5.3 constructor. The resolver
// argument is nil-safe: callers without plugin support pass nil and
// the handler set behaves as before.
func NewServiceWithTools(
	pl *plans.Store,
	ap *approvals.Store,
	agt handlers.AgentRunner,
	resolver handlers.TenantToolResolver,
	events *apievents.Emitter,
	logger *slog.Logger,
) *Service {
	if pl == nil || agt == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	reg := handlers.RegisterAll(graph.NewRegistry(), handlers.Config{
		Agent: agt,
		Tools: resolver,
	})
	rt := graph.NewRuntime(graph.Config{
		Plans:     pl,
		Approvals: ap,
		Registry:  reg,
		Logger:    logger,
		Emit:      buildGraphEmitter(pl, events),
	})
	return &Service{runtime: rt, plans: pl, logger: logger}
}

// buildGraphEmitter returns the Emit callback the graph runtime calls
// on every node lifecycle transition. Phase 3 (FU-025) ships two
// parallel frames per event:
//
//  1. Legacy: apievents.TypeAgentProgress with Kind=<raw graph kind>.
//     Retained for backward compatibility with Phase 2 consumers
//     during the migration window.
//  2. Canonical: apievents.TypeGraphNode{Started|Completed|Paused|Failed}
//     with a typed props struct carrying plan_id/node_id/kind/etc.
//
// When the legacy emitter is retired the Legacy frame
// becomes a no-op. Clients should dedupe on envelope id.
func buildGraphEmitter(pl *plans.Store, events *apievents.Emitter) func(ctx context.Context, planID, nodeID, kind string, detail map[string]any) {
	return func(ctx context.Context, planID, nodeID, kind string, detail map[string]any) {
		if events == nil {
			return
		}

		// Legacy frame consumers.
		legacyDetail := map[string]any{"plan_id": planID, "node_id": nodeID}
		for k, v := range detail {
			legacyDetail[k] = v
		}
		_ = events.Emit(ctx, apievents.SinkAll,
			apievents.TypeAgentProgress,
			apievents.AgentProgressProps{
				AgentKey: "orchestrator",
				Kind:     kind,
				Detail:   legacyDetail,
			})

		// Canonical frame typed lifecycle events.
		base := apievents.GraphNodeBase{PlanID: planID, NodeID: nodeID, Actor: actorFromCtx(ctx)}
		// Best-effort enrich from the plans store. Failure to look up
		// the node is not fatal — we still emit with the minimal base.
		if pl != nil {
			if n, err := pl.GetNode(ctx, nodeID); err == nil {
				base.Kind = string(n.Kind)
				base.Title = n.Title
				base.AgentKey = n.AssigneeSoul
			}
		}
		switch kind {
		case "node.started":
			_ = events.Emit(ctx, apievents.SinkAll,
				apievents.TypeGraphNodeStarted,
				apievents.GraphNodeStartedProps{GraphNodeBase: base})
		case "node.completed":
			outcome := stringOrEmpty(detail, "outcome")
			_ = events.Emit(ctx, apievents.SinkAll,
				apievents.TypeGraphNodeCompleted,
				apievents.GraphNodeCompletedProps{
					GraphNodeBase:    base,
					Outcome:          outcome,
					ArtifactsExcerpt: trimArtifacts(detail),
				})
		case "node.paused":
			_ = events.Emit(ctx, apievents.SinkAll,
				apievents.TypeGraphNodePaused,
				apievents.GraphNodePausedProps{
					GraphNodeBase: base,
					Reason:        stringOrEmpty(detail, "reason"),
					ApprovalID:    stringOrEmpty(detail, "approval_id"),
				})
		case "node.failed":
			_ = events.Emit(ctx, apievents.SinkAll,
				apievents.TypeGraphNodeFailed,
				apievents.GraphNodeFailedProps{
					GraphNodeBase: base,
					Error:         stringOrEmpty(detail, "error"),
				})
		default:
			// Handler-level progress (passthrough from HandlerContext.Emit)
			// intentionally stays on the legacy path only. Phase 3 does
			// not mint a distinct canonical type for each free-form
			// kind; clients that want structured tool progress still
			// consume agent.progress.
		}
	}
}

// stringOrEmpty returns detail[key] as a string, or "" on missing /
// wrong type.
func stringOrEmpty(detail map[string]any, key string) string {
	if detail == nil {
		return ""
	}
	if v, ok := detail[key].(string); ok {
		return v
	}
	return ""
}

// trimArtifacts returns at most a handful of top-level scalar entries
// from detail to ship inline. Structured payloads stay on the row.
// Bounded output prevents unbounded event payloads when a handler
// returns a large map.
func trimArtifacts(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	out := make(map[string]any, 4)
	count := 0
	for k, v := range detail {
		if count >= 8 {
			break
		}
		// Keep scalars; drop nested maps/slices in the excerpt.
		switch v.(type) {
		case string, bool, int, int32, int64, float32, float64:
			out[k] = v
			count++
		}
	}
	return out
}

// ExecutePlan runs (or resumes) a plan to its next quiescent state
// (done / failed / paused awaiting approval). Returns nil on a clean
// resumable pause, the graph's error otherwise.
func (s *Service) ExecutePlan(ctx context.Context, planID string) error {
	if s == nil || s.runtime == nil {
		return nil
	}
	err := s.runtime.Run(ctx, planID)
	if err == nil {
		return nil
	}
	// Pause is a normal signal, not an error.
	if err == graph.ErrPaused {
		return nil
	}
	s.logger.Error("orchestrator.ExecutePlan failed", "plan_id", planID, "err", err)
	return err
}

// Runtime exposes the underlying graph runtime for tests.
func (s *Service) Runtime() *graph.Runtime { return s.runtime }

// Ensure the agent package import pulls for RunRequest metadata passed
// through handlers.Config. Reserved for future pass-through.
var _ = agent.RunRequest{}
