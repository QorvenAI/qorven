// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package sessioncancel

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancel_ActuallyStopsGoroutine(t *testing.T) {
	r := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	release := r.Register("sess-1", cancel, time.Now().Add(time.Minute))
	defer release()

	done := make(chan struct{})
	var exited atomic.Bool
	go func() {
		<-ctx.Done()
		exited.Store(true)
		close(done)
	}()

	// Cancel and wait with a generous budget for the scheduler.
	if !r.Cancel("sess-1", CodeUserAbort, "user-42", "clicked stop") {
		t.Fatalf("cancel returned false for active session")
	}
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("goroutine did not exit within 500ms")
	}
	if !exited.Load() {
		t.Fatalf("goroutine claimed done but did not set exited flag")
	}
}

func TestCancel_IsIdempotent(t *testing.T) {
	r := NewRegistry()
	_, cancel := context.WithCancel(context.Background())
	release := r.Register("sess-1", cancel, time.Now().Add(time.Minute))
	defer release()

	if !r.Cancel("sess-1", CodeUserAbort, "u1", "") {
		t.Fatalf("first cancel must return true")
	}
	if r.Cancel("sess-1", CodeUserAbort, "u1", "") {
		t.Fatalf("second cancel must return false")
	}
}

func TestCancel_OfMissingSession(t *testing.T) {
	r := NewRegistry()
	if r.Cancel("nope", CodeUserAbort, "u1", "") {
		t.Fatalf("Cancel of missing session must return false")
	}
}

func TestRegister_CancelAndReplace(t *testing.T) {
	r := NewRegistry()
	ctxA, cancelA := context.WithCancel(context.Background())
	releaseA := r.Register("sess-1", cancelA, time.Now().Add(time.Minute))
	defer releaseA()

	// Second Register should preempt.
	ctxB, cancelB := context.WithCancel(context.Background())
	releaseB := r.Register("sess-1", cancelB, time.Now().Add(time.Minute))
	defer releaseB()

	select {
	case <-ctxA.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("first context was not cancelled by replacement")
	}
	select {
	case <-ctxB.Done():
		t.Fatalf("second context must still be live")
	default:
	}
}

func TestRelease_DoesNotEvictReplacement(t *testing.T) {
	r := NewRegistry()
	_, cancelA := context.WithCancel(context.Background())
	releaseA := r.Register("sess-1", cancelA, time.Now().Add(time.Minute))

	_, cancelB := context.WithCancel(context.Background())
	releaseB := r.Register("sess-1", cancelB, time.Now().Add(time.Minute))

	// releaseA must NOT delete the entry — B is the current one.
	releaseA()
	if r.Active() != 1 {
		t.Fatalf("expected 1 active, got %d", r.Active())
	}
	releaseB()
	if r.Active() != 0 {
		t.Fatalf("expected 0 active after second release, got %d", r.Active())
	}
}

func TestCancelAll(t *testing.T) {
	r := NewRegistry()
	const n = 10
	ctxs := make([]context.Context, n)
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		r.Register(fmt.Sprintf("s-%d", i), cancel, time.Now().Add(time.Minute))
	}
	count := r.CancelAll("admin")
	if count != n {
		t.Fatalf("expected %d cancelled, got %d", n, count)
	}
	for i, c := range ctxs {
		select {
		case <-c.Done():
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("ctx %d not cancelled", i)
		}
	}
}

// TestConcurrentAbort_NoGoroutineLeak is the benchmark the ruling asks for:
// 100 concurrent submits, 100 concurrent aborts, assert all goroutines exit
// within 500 ms and the runtime goroutine count returns to baseline.
func TestConcurrentAbort_NoGoroutineLeak(t *testing.T) {
	// Settle the runtime before we measure.
	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	baseline := runtime.NumGoroutine()

	r := NewRegistry()
	const n = 100

	// Launch N "agent turns" — each blocks on ctx.Done.
	var wg sync.WaitGroup
	releases := make([]func(), n)
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		releases[i] = r.Register(fmt.Sprintf("s-%d", i), cancel, time.Now().Add(time.Minute))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
		}()
	}

	// Fire 100 concurrent aborts.
	var aborters sync.WaitGroup
	for i := 0; i < n; i++ {
		aborters.Add(1)
		go func(i int) {
			defer aborters.Done()
			if !r.Cancel(fmt.Sprintf("s-%d", i), CodeUserAbort, "u1", "test") {
				t.Errorf("abort %d returned false", i)
			}
		}(i)
	}

	aborters.Wait()

	// All goroutines must exit within 500ms.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("goroutines did not exit within 500ms")
	}

	// Clean up entries.
	for _, rel := range releases {
		rel()
	}

	// Ensure the registry is empty.
	if r.Active() != 0 {
		t.Fatalf("registry still has %d active entries", r.Active())
	}

	// Goroutine count should be back within a reasonable delta (some test
	// runtime goroutines may fluctuate); allow +5 slack.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	cur := runtime.NumGoroutine()
	if cur > baseline+5 {
		t.Fatalf("goroutine leak: baseline=%d current=%d", baseline, cur)
	}
}

func TestLookup_DoesNotLeakCancelFunc(t *testing.T) {
	r := NewRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Register("s1", cancel, time.Now().Add(time.Minute))

	e, ok := r.Lookup("s1")
	if !ok {
		t.Fatalf("lookup returned false")
	}
	if e.Cancel != nil {
		t.Fatalf("Lookup leaked CancelFunc — caller could cancel out-of-band")
	}
}

func TestZeroValue_Works(t *testing.T) {
	// A zero Registry should work without NewRegistry.
	var r Registry
	_, cancel := context.WithCancel(context.Background())
	release := r.Register("s1", cancel, time.Now().Add(time.Minute))
	defer release()
	if r.Active() != 1 {
		t.Fatalf("zero value not usable")
	}
}
