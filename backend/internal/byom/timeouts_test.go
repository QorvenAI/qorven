// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package byom

import (
	"testing"
	"time"
)

// TestLoad_Defaults asserts that an unconfigured process lands on the
// documented cloud-deployment defaults. Flags regressions where a
// silent default change (e.g. someone lowers StalePlanAfter) would
// misclassify legitimately-slow BYOM inference as stuck.
func TestLoad_Defaults(t *testing.T) {
	// Drop any cached state from earlier tests AND from env vars a
	// developer may have set in their shell — unset the full knob set.
	for _, name := range []string{
		EnvSubmitHardTimeout,
		EnvPermissionTimeout,
		EnvStalePlanAfter,
		EnvSweeperTick,
		EnvGraphMaxHops,
	} {
		t.Setenv(name, "")
	}
	ResetForTests()

	got := Load()
	if got.SubmitHardTimeout != DefaultSubmitHardTimeout {
		t.Errorf("SubmitHardTimeout: got %v, want %v", got.SubmitHardTimeout, DefaultSubmitHardTimeout)
	}
	if got.PermissionTimeout != DefaultPermissionTimeout {
		t.Errorf("PermissionTimeout: got %v, want %v", got.PermissionTimeout, DefaultPermissionTimeout)
	}
	if got.StalePlanAfter != DefaultStalePlanAfter {
		t.Errorf("StalePlanAfter: got %v, want %v", got.StalePlanAfter, DefaultStalePlanAfter)
	}
	if got.SweeperTick != DefaultSweeperTick {
		t.Errorf("SweeperTick: got %v, want %v", got.SweeperTick, DefaultSweeperTick)
	}
	if got.GraphMaxHops != DefaultGraphMaxHops {
		t.Errorf("GraphMaxHops: got %v, want %v", got.GraphMaxHops, DefaultGraphMaxHops)
	}
}

// TestLoad_EnvOverrides proves BYOM users can raise the slow-inference
// knobs via env vars without patching code — the open-source contract
// the package exists to enforce.
func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv(EnvSubmitHardTimeout, "45m")
	t.Setenv(EnvPermissionTimeout, "10m")
	t.Setenv(EnvStalePlanAfter, "1h")
	t.Setenv(EnvSweeperTick, "2m")
	t.Setenv(EnvGraphMaxHops, "1024")
	ResetForTests()

	got := Load()
	if got.SubmitHardTimeout != 45*time.Minute {
		t.Errorf("SubmitHardTimeout override failed: got %v", got.SubmitHardTimeout)
	}
	if got.PermissionTimeout != 10*time.Minute {
		t.Errorf("PermissionTimeout override failed: got %v", got.PermissionTimeout)
	}
	if got.StalePlanAfter != time.Hour {
		t.Errorf("StalePlanAfter override failed: got %v", got.StalePlanAfter)
	}
	if got.SweeperTick != 2*time.Minute {
		t.Errorf("SweeperTick override failed: got %v", got.SweeperTick)
	}
	if got.GraphMaxHops != 1024 {
		t.Errorf("GraphMaxHops override failed: got %d", got.GraphMaxHops)
	}
}

// TestLoad_InvalidValuesFallBack asserts that a misconfigured env var
// does NOT crash the process — operators see a warning in the startup
// log and the default takes over. Silent corruption (e.g. accepting
// -1m as zero) would be worse than a loud reject.
func TestLoad_InvalidValuesFallBack(t *testing.T) {
	t.Setenv(EnvSubmitHardTimeout, "not-a-duration")
	t.Setenv(EnvPermissionTimeout, "-5m")
	t.Setenv(EnvGraphMaxHops, "-1")
	ResetForTests()

	got := Load()
	if got.SubmitHardTimeout != DefaultSubmitHardTimeout {
		t.Errorf("invalid SubmitHardTimeout should fall back: got %v", got.SubmitHardTimeout)
	}
	if got.PermissionTimeout != DefaultPermissionTimeout {
		t.Errorf("negative PermissionTimeout should fall back: got %v", got.PermissionTimeout)
	}
	if got.GraphMaxHops != DefaultGraphMaxHops {
		t.Errorf("negative GraphMaxHops should fall back: got %d", got.GraphMaxHops)
	}
}

// TestSetForTests_RoundTrip asserts the test-only override mechanism
// works as intended and the restore function actually restores.
func TestSetForTests_RoundTrip(t *testing.T) {
	ResetForTests()
	original := Load()

	restore := SetForTests(Timeouts{
		SubmitHardTimeout: 1 * time.Second,
		PermissionTimeout: 2 * time.Second,
		StalePlanAfter:    3 * time.Second,
		SweeperTick:       4 * time.Second,
		GraphMaxHops:      99,
	})
	overridden := Load()
	if overridden.SubmitHardTimeout != 1*time.Second {
		t.Errorf("SetForTests did not take effect: got %v", overridden.SubmitHardTimeout)
	}
	if overridden.GraphMaxHops != 99 {
		t.Errorf("SetForTests did not take effect for int field: got %d", overridden.GraphMaxHops)
	}

	restore()
	after := Load()
	if after != original {
		t.Errorf("restore did not recover original Timeouts: got %+v, want %+v", after, original)
	}
}
