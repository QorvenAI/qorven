// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"testing"
)

func TestCancel_StopsLoop(t *testing.T) {
	ac := NewAutonomousController(nil, DefaultAutonomousConfig())

	ctx, cancel := context.WithCancel(context.Background())
	st := &AutonomousState{
		SessionID: "s1",
		Status:    AutonomousRunning,
		cancel:    cancel,
	}

	ac.mu.Lock()
	ac.active["s1"] = st
	ac.mu.Unlock()

	if !ac.Cancel("s1") {
		t.Fatal("cancel should find the session")
	}
	if st.Status != AutonomousCancelled {
		t.Errorf("expected status %q, got %q", AutonomousCancelled, st.Status)
	}
	if ctx.Err() == nil {
		t.Error("context should be cancelled after Cancel()")
	}
}
