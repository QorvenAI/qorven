// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package rules

import (
	"testing"
	"time"
)

// ─── cron parser tests ────────────────────────────────────────────────────────

func TestNextCronTime_EveryMinute(t *testing.T) {
	from := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	next, err := nextCronTime("* * * * *", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 15, 10, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, next)
	}
}

func TestNextCronTime_DailySundayMidnight(t *testing.T) {
	// "0 0 * * 0" = every Sunday at midnight UTC
	// 2026-01-15 is a Thursday — next Sunday is 2026-01-18
	from := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	next, err := nextCronTime("0 0 * * 0", from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 18, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("want %v, got %v", want, next)
	}
}

func TestNextCronTime_Every2am(t *testing.T) {
	// "0 2 * * 0" = every Sunday at 2am
	from := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	next, err := nextCronTime("0 2 * * 0", from)
	if err != nil {
		t.Fatal(err)
	}
	if next.Weekday() != time.Sunday {
		t.Errorf("want Sunday, got %v", next.Weekday())
	}
	if next.Hour() != 2 || next.Minute() != 0 {
		t.Errorf("want 02:00, got %02d:%02d", next.Hour(), next.Minute())
	}
}

func TestNextCronTime_Step(t *testing.T) {
	// "*/15 * * * *" = every 15 minutes (0, 15, 30, 45)
	from := time.Date(2026, 1, 15, 10, 7, 0, 0, time.UTC)
	next, err := nextCronTime("*/15 * * * *", from)
	if err != nil {
		t.Fatal(err)
	}
	if next.Minute() != 15 {
		t.Errorf("want minute=15, got %d", next.Minute())
	}
}

func TestNextCronTime_InvalidField(t *testing.T) {
	_, err := nextCronTime("not-valid", time.Now())
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

// ─── matchField tests ─────────────────────────────────────────────────────────

func TestMatchField_Wildcard(t *testing.T) {
	if !matchField("*", 5, 0, 59) {
		t.Error("wildcard should match")
	}
}

func TestMatchField_Exact(t *testing.T) {
	if !matchField("30", 30, 0, 59) {
		t.Error("exact match should work")
	}
	if matchField("30", 31, 0, 59) {
		t.Error("non-match should fail")
	}
}

func TestMatchField_Range(t *testing.T) {
	if !matchField("8-17", 12, 0, 23) {
		t.Error("in-range should match")
	}
	if matchField("8-17", 5, 0, 23) {
		t.Error("out-of-range should not match")
	}
}

func TestMatchField_List(t *testing.T) {
	if !matchField("1,3,5", 3, 0, 7) {
		t.Error("list member should match")
	}
	if matchField("1,3,5", 2, 0, 7) {
		t.Error("non-member should not match")
	}
}

// ─── threshold matching tests ─────────────────────────────────────────────────

func TestMatchesThreshold(t *testing.T) {
	e := &Engine{}
	r := rule{
		triggerType: "threshold",
		triggerSpec: []byte(`{"metric":"cpu","operator":"gt","value":90}`),
	}

	if !e.matchesThreshold(r, map[string]any{"cpu": 95.0}) {
		t.Error("95 > 90 should match")
	}
	if e.matchesThreshold(r, map[string]any{"cpu": 85.0}) {
		t.Error("85 > 90 should not match")
	}
	if e.matchesThreshold(r, map[string]any{"ram": 95.0}) {
		t.Error("wrong metric should not match")
	}
}

// ─── event matching tests ─────────────────────────────────────────────────────

func TestMatchesEvent(t *testing.T) {
	e := &Engine{}
	r := rule{
		triggerType: "event",
		triggerSpec: []byte(`{"event":"invoice.received"}`),
	}

	if !e.matchesEvent(r, "invoice.received") {
		t.Error("exact match should fire")
	}
	if !e.matchesEvent(r, "Invoice.Received") {
		t.Error("case-insensitive match should fire")
	}
	if e.matchesEvent(r, "invoice.sent") {
		t.Error("different event should not fire")
	}
}

// ─── interpolation test ───────────────────────────────────────────────────────

func TestInterpolateData(t *testing.T) {
	result := interpolateData("CPU is {cpu}% on host {host}", map[string]any{
		"cpu":  95,
		"host": "server-1",
	})
	want := "CPU is 95% on host server-1"
	if result != want {
		t.Errorf("want %q, got %q", want, result)
	}
}
