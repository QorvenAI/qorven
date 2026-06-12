// Copyright 2026 Qorven AI. All rights reserved.
package calendar

import (
	"strings"
	"testing"
	"time"
)

func TestTimeline_MethodSet(t *testing.T) {
	_ = (*Store).Timeline
}

func TestKindFor(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	if got := kindFor(future, "scheduled", now); got != "future" {
		t.Errorf("future scheduled = %q, want future", got)
	}
	if got := kindFor(past, "ok", now); got != "past" {
		t.Errorf("past ok = %q, want past", got)
	}
	if got := kindFor(future, "running", now); got != "past" {
		t.Errorf("running = %q, want past", got)
	}
}

func TestSnippet(t *testing.T) {
	if got := snippet("hello world this is long", 5); got != "hello" {
		t.Errorf("snippet = %q", got)
	}
	if got := snippet("hi", 5); got != "hi" {
		t.Errorf("snippet short = %q", got)
	}
	if strings.Contains(snippet("", 5), " ") {
		t.Errorf("empty snippet should be empty")
	}
}
