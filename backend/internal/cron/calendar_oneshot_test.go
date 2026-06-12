// Copyright 2026 Qorven AI. All rights reserved.
package cron

import "testing"

// The handler must report success/failure so the calendar records real status
// instead of a hardcoded 'ok'. This asserts the result type shape.
func TestDBRunResult_Shape(t *testing.T) {
	r := DBRunResult{Success: true, ResultSnippet: "done", Tokens: 10, CostCents: 2}
	if !r.Success || r.ResultSnippet != "done" || r.Tokens != 10 || r.CostCents != 2 {
		t.Fatalf("DBRunResult fields drifted: %+v", r)
	}
}
