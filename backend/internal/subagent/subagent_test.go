// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package subagent

import "testing"

func TestRunStatus_Constants(t *testing.T) {
	statuses := []RunStatus{"running", "completed", "failed", "killed"}
	for _, s := range statuses {
		if s == "" { t.Error("empty status") }
	}
}

func TestRun_Fields(t *testing.T) {
	r := Run{ID: "r1", ParentAgentID: "p1", ChildAgentID: "c1", Task: "do something"}
	if r.ID != "r1" { t.Error("wrong id") }
	if r.Task != "do something" { t.Error("wrong task") }
}

func TestLifecycleManager_New(t *testing.T) {
	lm := NewLifecycleManager(nil)
	if lm == nil { t.Fatal("nil") }
}

func TestLifecycleManager_ListActive_Empty(t *testing.T) {
	lm := NewLifecycleManager(nil)
	active := lm.ListActive()
	if len(active) != 0 { t.Error("should be empty") }
}
