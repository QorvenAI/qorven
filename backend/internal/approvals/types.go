// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package approvals implements the approval state machine on top of
// the Phase 2 plan graph. An approval lives against a single
// human_feedback plan_node and carries a comment thread.
//
// States:
//
//	pending → approved                      (reviewer accepted)
//	pending → rejected                      (reviewer halted the plan)
//	pending → revision_requested            (reviewer asked for edits)
//	revision_requested → pending            (planner re-ran; new approval row)
//
// Idempotent Resolve(): retrying an approve on an already-approved
// approval is a no-op, not an error. This matches the ruling's P1
// expectation and prevents double-charging cost meters from client
// retries.
package approvals

import (
	"encoding/json"
	"fmt"
	"time"
)

// State is the approval lifecycle.
type State string

const (
	StatePending            State = "pending"
	StateApproved           State = "approved"
	StateRejected           State = "rejected"
	StateRevisionRequested  State = "revision_requested"
)

// Approval is the persisted record.
type Approval struct {
	ID          string          `json:"id"`
	PlanID      string          `json:"plan_id"`
	NodeID      string          `json:"node_id"`
	State       State           `json:"state"`
	RequestedBy string          `json:"requested_by"`
	ResolvedBy  string          `json:"resolved_by,omitempty"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
	Budget      json.RawMessage `json:"budget,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Comment is one entry in the approval's thread.
type Comment struct {
	ID         string    `json:"id"`
	ApprovalID string    `json:"approval_id"`
	Author     string    `json:"author"`
	AuthorIs   string    `json:"author_is"` // user | agent | system
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// illegalTransition signals the caller tried to move from a terminal
// state. Idempotent no-ops (e.g. approve-when-approved) return nil.
type illegalTransition struct {
	From State
	To   State
}

func (e *illegalTransition) Error() string {
	return fmt.Sprintf("approvals: illegal transition %s → %s", e.From, e.To)
}

// IsIllegalTransition reports whether the error is an illegal transition.
func IsIllegalTransition(err error) bool {
	_, ok := err.(*illegalTransition)
	return ok
}
