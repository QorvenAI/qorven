// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import "testing"

func TestLevelAllowsChannel(t *testing.T) {
	cases := []struct {
		level string
		want  bool
	}{
		{"l1", true},
		{"l2", true},
		{"l3", false},
		{"", false},
		{"customer_facing", false},
		{"L1", true},
		{"L2", true},
	}
	for _, c := range cases {
		if got := levelAllowsChannel(c.level); got != c.want {
			t.Errorf("levelAllowsChannel(%q) = %v, want %v", c.level, got, c.want)
		}
	}
}
