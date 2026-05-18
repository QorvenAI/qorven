// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tasks

import (
	"testing"
)

func TestPausedTransitions(t *testing.T) {
	// in_progress → paused is valid
	if !isValidTransition(StatusInProgress, StatusPaused) {
		t.Error("expected in_progress→paused to be valid")
	}
	// paused → in_progress is valid (resume)
	if !isValidTransition(StatusPaused, StatusInProgress) {
		t.Error("expected paused→in_progress to be valid")
	}
	// paused → cancelled is valid
	if !isValidTransition(StatusPaused, StatusCancelled) {
		t.Error("expected paused→cancelled to be valid")
	}
	// paused → done is NOT valid (must resume first)
	if isValidTransition(StatusPaused, StatusDone) {
		t.Error("expected paused→done to be invalid")
	}
}
