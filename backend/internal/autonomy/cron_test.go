// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package autonomy

import (
	"testing"
	"time"
)

func TestComputeNextRunFrom_AtSchedule_Before(t *testing.T) {
	s := NewCronScheduler(nil)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 06:00 Shanghai — before 08:00
	now := time.Date(2026, 5, 7, 6, 0, 0, 0, loc)
	got := s.computeNextRunFrom("at:08:00+Asia/Shanghai", "", now)
	want := time.Date(2026, 5, 7, 8, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("before 08:00: got %v, want %v", got, want)
	}
}

func TestComputeNextRunFrom_AtSchedule_After(t *testing.T) {
	s := NewCronScheduler(nil)
	loc, _ := time.LoadLocation("Asia/Shanghai")
	// 09:00 Shanghai — after 08:00, should schedule next day
	now := time.Date(2026, 5, 7, 9, 0, 0, 0, loc)
	got := s.computeNextRunFrom("at:08:00+Asia/Shanghai", "", now)
	want := time.Date(2026, 5, 8, 8, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("after 08:00: got %v, want %v", got, want)
	}
}

func TestComputeNextRunFrom_EveryInterval(t *testing.T) {
	s := NewCronScheduler(nil)
	now := time.Now()
	got := s.computeNextRunFrom("every:30m", "", now)
	diff := got.Sub(now)
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Errorf("expected ~30m interval, got %v", diff)
	}
}
