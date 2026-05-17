// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Deep cache tests — race conditions, TTL precision, eviction under pressure.

func TestDeep_Cache_RaceCondition_SetGet(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	var wg sync.WaitGroup

	// 100 writers setting same key
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Set(ctx, "race", n, time.Minute)
		}(i)
	}

	// 100 readers reading same key
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Get(ctx, "race")
		}()
	}

	wg.Wait()
	// Should not panic or deadlock
	v, ok := c.Get(ctx, "race")
	if !ok { t.Error("key should exist after writes") }
	t.Logf("final value: %d", v)
}

func TestDeep_Cache_RaceCondition_DeleteWhileReading(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	var wg sync.WaitGroup

	// Pre-populate
	for i := 0; i < 100; i++ {
		c.Set(ctx, "key"+string(rune('0'+i%10)), "value", time.Minute)
	}

	// Concurrent reads and deletes
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			c.Get(ctx, "key"+string(rune('0'+n%10)))
		}(i)
		go func(n int) {
			defer wg.Done()
			c.Delete(ctx, "key"+string(rune('0'+n%10)))
		}(i)
	}
	wg.Wait()
}

func TestDeep_Cache_TTL_Precision_Tight(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()

	// Set with 50ms TTL
	c.Set(ctx, "precise", "value", 50*time.Millisecond)

	// Should exist at 20ms
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get(ctx, "precise"); !ok { t.Error("should exist at 20ms") }

	// Should expire by 80ms
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.Get(ctx, "precise"); ok { t.Error("should expire by 80ms") }
}

func TestDeep_Cache_HighThroughput(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	start := time.Now()

	// 100K operations
	for i := 0; i < 100000; i++ {
		key := "k" + string(rune(i%1000))
		c.Set(ctx, key, i, time.Minute)
		c.Get(ctx, key)
	}

	elapsed := time.Since(start)
	t.Logf("100K set+get operations: %v (%.0f ops/sec)", elapsed, 200000/elapsed.Seconds())
	if elapsed > 5*time.Second { t.Error("too slow for 100K ops") }
}

func TestDeep_Cache_PrefixDelete_Correctness(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()

	// Set keys with different prefixes
	prefixes := []string{"user:", "session:", "agent:", "memory:"}
	for _, p := range prefixes {
		for i := 0; i < 10; i++ {
			c.Set(ctx, p+string(rune('0'+i)), "val", time.Minute)
		}
	}

	// Delete one prefix
	c.DeleteByPrefix(ctx, "session:")

	// Verify only session: keys are gone
	for _, p := range prefixes {
		for i := 0; i < 10; i++ {
			key := p + string(rune('0'+i))
			_, ok := c.Get(ctx, key)
			if p == "session:" && ok { t.Errorf("session key %q should be deleted", key) }
			if p != "session:" && !ok { t.Errorf("non-session key %q should survive", key) }
		}
	}
}

func TestDeep_Cache_ConcurrentPrefixDelete(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	var wg sync.WaitGroup
	var errors atomic.Int32

	// Writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Set(ctx, "p"+string(rune('a'+n%5))+":"+string(rune('0'+j%10)), n, time.Minute)
			}
		}(i)
	}

	// Deleters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			time.Sleep(time.Duration(n) * time.Millisecond)
			c.DeleteByPrefix(ctx, "p"+string(rune('a'+n%5))+":")
		}(i)
	}

	// Readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Get(ctx, "p"+string(rune('a'+n%5))+":0")
		}(i)
	}

	wg.Wait()
	if errors.Load() > 0 { t.Errorf("%d errors", errors.Load()) }
	t.Log("concurrent prefix delete: no panics, no deadlocks")
}
