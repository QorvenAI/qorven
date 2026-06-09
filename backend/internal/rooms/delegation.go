// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package rooms

import (
	"context"

	"github.com/qorvenai/qorven/internal/agent"
)

// MaxDelegationDepth bounds the head→L3→head-rollup chain. Depth 0 = a head
// delegating to an L3 (the L3 run is depth 1); the head's roll-up wake is depth
// 2 and is summary-only, so no further delegation can start.
const MaxDelegationDepth = 1

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
