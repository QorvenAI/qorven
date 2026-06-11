// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import "testing"

func TestFixDedupKey(t *testing.T) {
	if fixDedupKey("ci", "42") != fixDedupKey("ci", "42") {
		t.Error("stable")
	}
	if fixDedupKey("ci", "42") == fixDedupKey("deploy", "42") {
		t.Error("source distinguishes")
	}
}

func TestFixAttemptExceeded(t *testing.T) {
	if !fixAttemptExceeded(3, 3) {
		t.Error("at cap → exceeded")
	}
	if fixAttemptExceeded(2, 3) {
		t.Error("under cap → not exceeded")
	}
	if fixAttemptExceeded(1, 0) {
		t.Error("cap 0 → never exceeded (disabled)")
	}
}
