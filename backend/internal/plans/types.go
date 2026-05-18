// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package plans is the persistence + state-machine layer for the
// Phase 2 plan graph. Every plan the orchestrator executes is a row in
// `plans`, a DAG of rows in `plan_nodes`, and a set of static wirings
// in `plan_edges`.
//
// The runtime in internal/orchestrator/graph consumes this store.
// The HTTP layer in internal/gateway mounts it under /v1/plans.
package plans

import (
	"encoding/json"
	"fmt"
	"time"
)

// PlanStatus is the top-level lifecycle of a plan. Values are a closed
// enum enforced by a CHECK constraint in the DB.
type PlanStatus string

const (
	StatusDraft              PlanStatus = "draft"
	StatusPendingApproval    PlanStatus = "pending_approval"
	StatusApproved           PlanStatus = "approved"
	StatusRejected           PlanStatus = "rejected"
	StatusRevisionRequested  PlanStatus = "revision_requested"
	StatusRunning            PlanStatus = "running"
	StatusDone               PlanStatus = "done"
	StatusFailed             PlanStatus = "failed"
	StatusCancelled          PlanStatus = "cancelled"
	StatusArchived           PlanStatus = "archived"
)

// NodeKind enumerates the graph-runtime node types. Extending this
// requires a CHECK constraint migration.
type NodeKind string

const (
	KindPlanner        NodeKind = "planner"
	KindHumanFeedback  NodeKind = "human_feedback"
	KindAgentTask      NodeKind = "agent_task"
	KindReview         NodeKind = "review"
	KindPush           NodeKind = "push"
	KindPreview        NodeKind = "preview"
)

// NodeState tracks execution status per node.
type NodeState string

const (
	NodePending   NodeState = "pending"
	NodeRunning   NodeState = "running"
	NodeDone      NodeState = "done"
	NodeFailed    NodeState = "failed"
	NodeBlocked   NodeState = "blocked"
	NodeCancelled NodeState = "cancelled"
)

// EdgeCondition is the small expression on a plan_edges.condition column.
type EdgeCondition string

const (
	CondAlways    EdgeCondition = "always"
	CondApproved  EdgeCondition = "approved"
	CondRejected  EdgeCondition = "rejected"
	CondRevision  EdgeCondition = "revision"
	CondOnSuccess EdgeCondition = "on_success"
	CondOnError   EdgeCondition = "on_error"
)

// Plan is the top-level DAG record.
type Plan struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	ProjectID  string          `json:"project_id,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	Title      string          `json:"title"`
	Status     PlanStatus      `json:"status"`
	Spec       json.RawMessage `json:"spec,omitempty"`
	Summary    string          `json:"summary,omitempty"`
	CreatedBy  string          `json:"created_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	ArchivedAt *time.Time      `json:"archived_at,omitempty"`
}

// Node is one vertex in the plan graph.
type Node struct {
	ID            string          `json:"id"`
	PlanID        string          `json:"plan_id"`
	ParentID      string          `json:"parent_id,omitempty"`
	Kind          NodeKind        `json:"kind"`
	Title         string          `json:"title"`
	State         NodeState       `json:"state"`
	AssigneeSoul  string          `json:"assignee_soul,omitempty"`
	Inputs        json.RawMessage `json:"inputs,omitempty"`
	Artifacts     json.RawMessage `json:"artifacts,omitempty"`
	Error         string          `json:"error,omitempty"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	EndedAt       *time.Time      `json:"ended_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Edge is a static wiring between two nodes.
type Edge struct {
	PlanID    string        `json:"plan_id"`
	FromNode  string        `json:"from_node"`
	ToNode    string        `json:"to_node"`
	Condition EdgeCondition `json:"condition"`
}

// ValidTransition reports whether the status transition old → next is
// allowed. Used by the store's Update methods to reject illegal moves
// before hitting the DB.
//
// The graph runtime is the canonical driver. It transitions:
//
//	draft → running                             (plans with no human_feedback gate)
//	draft → pending_approval → approved → running (plans with gates)
//	running → done | failed | cancelled
//
// Ad-hoc or resumption paths may re-enter pending_approval from
// revision_requested. Terminal states reject all further moves.
func (old PlanStatus) ValidTransition(next PlanStatus) bool {
	if old == next {
		return true
	}
	switch old {
	case StatusDraft:
		return next == StatusPendingApproval ||
			next == StatusRunning || next == StatusCancelled
	case StatusPendingApproval:
		return next == StatusApproved || next == StatusRejected ||
			next == StatusRevisionRequested || next == StatusCancelled
	case StatusRevisionRequested:
		return next == StatusPendingApproval || next == StatusCancelled
	case StatusApproved:
		return next == StatusRunning || next == StatusCancelled
	case StatusRunning:
		return next == StatusDone || next == StatusFailed || next == StatusCancelled
	case StatusRejected, StatusDone, StatusFailed, StatusCancelled:
		return next == StatusArchived
	case StatusArchived:
		return false // terminal
	}
	return false
}

// ValidNodeTransition reports whether the node state transition old →
// next is allowed.
//
// Note: we allow pending → failed directly for the "no handler
// registered" path, where the node never runs but must still be
// terminalized so the plan can fail cleanly. The graph runtime is the
// only legitimate caller of that edge.
func (old NodeState) ValidNodeTransition(next NodeState) bool {
	if old == next {
		return true
	}
	switch old {
	case NodePending:
		return next == NodeRunning || next == NodeBlocked ||
			next == NodeCancelled || next == NodeFailed
	case NodeRunning:
		return next == NodeDone || next == NodeFailed || next == NodeCancelled || next == NodeBlocked
	case NodeBlocked:
		return next == NodePending || next == NodeRunning || next == NodeCancelled
	case NodeDone, NodeFailed, NodeCancelled:
		return false // terminal
	}
	return false
}

// errIllegalTransition is returned by Update methods on a rejected move.
type illegalTransition struct {
	Entity string
	From   string
	To     string
}

func (e *illegalTransition) Error() string {
	return fmt.Sprintf("plans: illegal %s transition %s → %s", e.Entity, e.From, e.To)
}

// IsIllegalTransition reports whether the error is an illegal transition.
func IsIllegalTransition(err error) bool {
	_, ok := err.(*illegalTransition)
	return ok
}
