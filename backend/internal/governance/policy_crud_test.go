// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package governance

import "testing"

// No DB pool harness exists in this package; integration coverage is provided
// by the live smoke test. This file covers the pure-logic helper.

func TestValidPolicyAction(t *testing.T) {
	for _, a := range []string{"allow", "deny", "warn", "require_approval", "throttle", "log", "escalate"} {
		if !ValidPolicyAction(a) {
			t.Errorf("%q should be valid", a)
		}
	}
	if ValidPolicyAction("bogus") {
		t.Error("bogus should be invalid")
	}
}
