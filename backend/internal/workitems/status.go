// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package workitems implements durable delegation chains for the Operations
// Fabric: a work item's owner, status, what it's blocked on, and its audit log.
package workitems

// Work-item lifecycle statuses.
const (
	StatusOpen       = "open"
	StatusAssigned   = "assigned"
	StatusInProgress = "in_progress"
	StatusBlocked    = "blocked"
	StatusInReview   = "in_review"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

// allowed maps each status to the statuses it may transition to.
var allowed = map[string][]string{
	StatusOpen:       {StatusAssigned, StatusCancelled},
	StatusAssigned:   {StatusInProgress, StatusCancelled},
	StatusInProgress: {StatusBlocked, StatusInReview, StatusCancelled},
	StatusBlocked:    {StatusInProgress, StatusCancelled},
	StatusInReview:   {StatusInProgress, StatusDone, StatusCancelled},
	StatusDone:       {},
	StatusCancelled:  {},
}

// CanTransition reports whether a work item may move from `from` to `to`.
func CanTransition(from, to string) bool {
	nexts, ok := allowed[from]
	if !ok {
		return false
	}
	for _, n := range nexts {
		if n == to {
			return true
		}
	}
	return false
}
