//go:build unit

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestCachedScriptValid(t *testing.T) {
	body := []byte("console.log('react')")
	sum := sha256.Sum256(body)
	good := hex.EncodeToString(sum[:])

	cases := []struct {
		name    string
		body    []byte
		wantSHA string
		valid   bool
	}{
		{"no pin configured trusts cache", body, "", true},
		{"matching pin is valid", body, good, true},
		{"mismatched pin is rejected", body, "deadbeef", false},
		{"tampered body under a pin is rejected", []byte("console.log('evil')"), good, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cachedScriptValid(tc.body, tc.wantSHA); got != tc.valid {
				t.Errorf("cachedScriptValid(%q, %q) = %v, want %v", tc.body, tc.wantSHA, got, tc.valid)
			}
		})
	}
}
