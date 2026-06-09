// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"testing"
	"time"
)

func TestUsageWindow_ExhaustedBlocks(t *testing.T) {
	future := time.Now().Add(time.Hour)
	w := &UsageWindow{LimitCount: 100, UsedCount: 100, ResetsAt: &future}
	if w.Available(time.Now()) {
		t.Fatal("exhausted window (used>=limit, not yet reset) must be unavailable")
	}
}

func TestUsageWindow_UnderLimitAvailable(t *testing.T) {
	future := time.Now().Add(time.Hour)
	w := &UsageWindow{LimitCount: 100, UsedCount: 40, ResetsAt: &future}
	if !w.Available(time.Now()) {
		t.Fatal("under-limit window must be available")
	}
}

func TestUsageWindow_PastResetIsAvailable(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	w := &UsageWindow{LimitCount: 100, UsedCount: 100, ResetsAt: &past}
	if !w.Available(time.Now()) {
		t.Fatal("window past its reset must be available again")
	}
}

func TestUsageWindow_NilIsAlwaysAvailable(t *testing.T) {
	var w *UsageWindow
	if !w.Available(time.Now()) {
		t.Fatal("a key with no declared window must be available")
	}
}
