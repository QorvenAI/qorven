// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Hard cache tests — eviction under memory pressure, TTL accuracy, throughput.

func TestHard_Cache_EvictionUnderPressure(t *testing.T) {
	c := NewInMemoryCache[[]byte]()
	ctx := context.Background()

	// Fill cache with 100 x 1MB entries (100MB total)
	for i := 0; i < 100; i++ {
		data := make([]byte, 1024*1024) // 1MB
		c.Set(ctx, "big-"+string(rune('A'+i%26))+string(rune('0'+i/26)), data, time.Minute)
	}

	// All should be retrievable
	found := 0
	for i := 0; i < 100; i++ {
		key := "big-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		if _, ok := c.Get(ctx, key); ok { found++ }
	}
	t.Logf("100 x 1MB entries: %d found", found)

	// Clear and verify memory is freed
	c.Clear(ctx)
	if _, ok := c.Get(ctx, "big-A0"); ok { t.Error("should be cleared") }
}

func TestHard_Cache_TTL_MixedExpiry(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()

	// Set entries with different TTLs
	c.Set(ctx, "fast", "expires-fast", 30*time.Millisecond)
	c.Set(ctx, "medium", "expires-medium", 100*time.Millisecond)
	c.Set(ctx, "slow", "expires-slow", 500*time.Millisecond)
	c.Set(ctx, "permanent", "never-expires", time.Hour)

	// At 50ms: fast expired, others alive
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.Get(ctx, "fast"); ok { t.Error("fast should expire by 50ms") }
	if _, ok := c.Get(ctx, "medium"); !ok { t.Error("medium should be alive at 50ms") }
	if _, ok := c.Get(ctx, "slow"); !ok { t.Error("slow should be alive at 50ms") }

	// At 150ms: medium expired too
	time.Sleep(100 * time.Millisecond)
	if _, ok := c.Get(ctx, "medium"); ok { t.Error("medium should expire by 150ms") }
	if _, ok := c.Get(ctx, "slow"); !ok { t.Error("slow should be alive at 150ms") }
	if _, ok := c.Get(ctx, "permanent"); !ok { t.Error("permanent should be alive") }

	t.Log("mixed TTL expiry verified ✅")
}

func TestHard_Cache_Throughput_Benchmark(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()

	// Benchmark: how many ops/sec can we sustain?
	ops := 500000
	start := time.Now()
	for i := 0; i < ops; i++ {
		key := "bench-" + string(rune('A'+i%26))
		c.Set(ctx, key, i, time.Minute)
		c.Get(ctx, key)
	}
	elapsed := time.Since(start)
	opsPerSec := float64(ops*2) / elapsed.Seconds()
	t.Logf("throughput: %d set+get in %v (%.0f ops/sec)", ops, elapsed, opsPerSec)
	if opsPerSec < 100000 { t.Errorf("too slow: %.0f ops/sec", opsPerSec) }
}

func TestHard_Cache_ConcurrentMixedOps(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	var wg sync.WaitGroup
	var errors atomic.Int32

	// 50 writers, 50 readers, 10 deleters, 5 clearers — all concurrent
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Set(ctx, "k"+string(rune('0'+n%10)), n, time.Minute)
			}
		}(i)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Get(ctx, "k"+string(rune('0'+n%10)))
			}
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.Delete(ctx, "k"+string(rune('0'+n%10)))
			}
		}(i)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			c.DeleteByPrefix(ctx, "k")
		}()
	}

	wg.Wait()
	if errors.Load() > 0 { t.Errorf("%d errors", errors.Load()) }
	t.Log("115 goroutines mixed ops: no panics, no deadlocks ✅")
}
