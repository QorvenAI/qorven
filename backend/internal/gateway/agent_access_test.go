// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import "testing"

func TestAgentChattable(t *testing.T) {
	cases := []struct {
		level string
		want  bool
	}{
		{"l1", true},
		{"l2", true},
		{"L1", true},   // case-insensitive
		{" l2 ", true}, // trimmed
		{"l3", false},
		{"customer_facing", false},
		{"", false}, // blank defaults to non-chattable (worker)
		{"random", false},
	}
	for _, c := range cases {
		if got := agentChattable(c.level); got != c.want {
			t.Errorf("agentChattable(%q) = %v, want %v", c.level, got, c.want)
		}
	}
}
