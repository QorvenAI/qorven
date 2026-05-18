// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tasks

import "testing"

func TestCanTransition_Valid(t *testing.T) {
	valid := [][2]TaskState{
		{StateBacklog, StateTodo},
		{StateTodo, StateInProgress},
		{StateInProgress, StateReview},
		{StateReview, StateDone},
	}
	for _, pair := range valid {
		if !CanTransition(pair[0], pair[1]) { t.Errorf("should allow %s → %s", pair[0], pair[1]) }
	}
}

func TestCanTransition_Invalid(t *testing.T) {
	if CanTransition(StateDone, StateBacklog) { t.Error("done → backlog should be invalid") }
}

func TestLifecycle_New(t *testing.T) {
	lc := NewLifecycle(nil)
	if lc == nil { t.Fatal("nil") }
}

func TestTask_Fields(t *testing.T) {
	task := Task{ID: "t1", Title: "Fix bug", Status: "backlog"}
	if task.Status != "backlog" { t.Error("wrong state") }
}

func TestTaskState_Values(t *testing.T) {
	states := []TaskState{StateBacklog, StateTodo, StateInProgress, StateReview, StateDone, StateCancelled}
	for _, s := range states {
		if s == "" { t.Error("empty state") }
	}
}
