// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "testing"

func TestDelegatedTask_HasTargetURL(t *testing.T) {
	task := &DelegatedTask{
		ID:        "del-1",
		TargetURL: "/code?tab=build&project=proj-abc",
		Context:   map[string]any{"project_id": "proj-abc"},
	}
	if task.TargetURL == "" {
		t.Fatal("TargetURL must be settable")
	}
	if task.Context["project_id"] != "proj-abc" {
		t.Fatalf("Context project_id mismatch: %v", task.Context["project_id"])
	}
}
