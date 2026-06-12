// Copyright 2026 Qorven AI. All rights reserved.
package social

import "testing"

func TestApprovalDecision(t *testing.T) {
	cases := map[string]bool{"none": false, "supervisor": true, "user": true, "both": true, "": true}
	for mode, want := range cases {
		if NeedsApproval(mode) != want {
			t.Errorf("NeedsApproval(%q) = %v, want %v", mode, NeedsApproval(mode), want)
		}
	}
}
