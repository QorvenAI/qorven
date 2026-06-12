// Copyright 2026 Qorven AI. All rights reserved.
package calendar

import "testing"

// These confirm the run-store method set compiles with the intended signatures.
// (DB-backed behavior is covered by the gateway integration at runtime — a nil
// pool Store is fine here; we only assert the interface shape.)
func TestRunStore_MethodSet(t *testing.T) {
	_ = (*Store).StartRun
	_ = (*Store).FinishRun
	_ = (*Store).GetRun
	_ = (*Store).ListRuns
}

func TestRunStatusConstants(t *testing.T) {
	if RunStatusRunning != "running" || RunStatusOK != "ok" || RunStatusError != "error" {
		t.Fatalf("run status constants drifted: %q %q %q", RunStatusRunning, RunStatusOK, RunStatusError)
	}
}
