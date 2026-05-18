// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDeniedError_IsDenied verifies the helper sentinel.
func TestDeniedError_IsDenied(t *testing.T) {
	e := &DeniedError{RequestID: "r1", Note: "nope"}
	if !IsDenied(e) {
		t.Fatalf("IsDenied must match")
	}
	if IsDenied(errors.New("x")) {
		t.Fatalf("IsDenied false positive on plain error")
	}
	if got := e.Error(); got != "permission denied (request r1): nope" {
		t.Fatalf("message: %q", got)
	}
	e2 := &DeniedError{RequestID: "r2"}
	if got := e2.Error(); got != "permission denied (request r2)" {
		t.Fatalf("no-note message: %q", got)
	}
}

func TestExpiredError_IsExpired(t *testing.T) {
	e := &ExpiredError{RequestID: "r1", After: 2 * time.Second}
	if !IsExpired(e) {
		t.Fatalf("IsExpired must match")
	}
	if IsExpired(errors.New("x")) {
		t.Fatalf("IsExpired false positive")
	}
	if got := e.Error(); got != "permission request r1 expired after 2s" {
		t.Fatalf("message: %q", got)
	}
}

func TestVerdict_Allowed(t *testing.T) {
	v := &Verdict{Decision: DecisionAllow}
	if !v.Allowed() {
		t.Fatalf("allow should permit")
	}
	v.Expired = true
	if v.Allowed() {
		t.Fatalf("expired allow must not permit")
	}
	v = &Verdict{Decision: DecisionDeny}
	if v.Allowed() {
		t.Fatalf("deny must not permit")
	}
}

func TestRequest_NilGate(t *testing.T) {
	var g *Gate // nil
	_ = g
	// A nil pool inside Gate triggers the explicit check.
	g2 := &Gate{}
	_, err := g2.Request(context.Background(), RequestInput{Tool: "x"})
	if err == nil || !contains(err.Error(), "gate not configured") {
		t.Fatalf("expected nil-pool error, got %v", err)
	}
}

func TestReply_InvalidDecision(t *testing.T) {
	g := &Gate{}
	if _, err := g.Reply(context.Background(), "r1", ReplyInput{Decision: "maybe"}); err == nil {
		t.Fatalf("expected invalid-decision error")
	}
}

func TestReply_EmptyID(t *testing.T) {
	g := &Gate{}
	if _, err := g.Reply(context.Background(), "", ReplyInput{Decision: DecisionAllow}); err == nil {
		t.Fatalf("expected empty-id error")
	}
}

func TestDecodeArgsMap(t *testing.T) {
	if got := decodeArgsMap(nil); got != nil {
		t.Fatalf("nil → %v", got)
	}
	if got := decodeArgsMap([]byte(`{"x":1}`)); got["x"] != float64(1) {
		t.Fatalf("map decode: %+v", got)
	}
	if got := decodeArgsMap([]byte(`garbage`)); got != nil {
		t.Fatalf("garbage must yield nil, got %+v", got)
	}
}

func contains(s, sub string) bool {
	return indexOf(s, sub) >= 0
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
