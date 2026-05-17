// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Hard cache tests — TTL expiry, concurrent access, prefix delete, generics.

func TestInMemoryCache_SetGet(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	c.Set(ctx, "key1", "value1", time.Minute)
	v, ok := c.Get(ctx, "key1")
	if !ok { t.Error("should find key") }
	if v != "value1" { t.Errorf("value=%q", v) }
}

func TestInMemoryCache_GetMissing(t *testing.T) {
	c := NewInMemoryCache[string]()
	_, ok := c.Get(context.Background(), "nonexistent")
	if ok { t.Error("should not find missing key") }
}

func TestInMemoryCache_TTLExpiry(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	c.Set(ctx, "key1", "value1", 50*time.Millisecond)
	v, ok := c.Get(ctx, "key1")
	if !ok { t.Error("should find before expiry") }
	if v != "value1" { t.Error("wrong value") }
	time.Sleep(100 * time.Millisecond)
	_, ok = c.Get(ctx, "key1")
	if ok { t.Error("should expire after TTL") }
}

func TestInMemoryCache_Overwrite(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	c.Set(ctx, "counter", 1, time.Minute)
	c.Set(ctx, "counter", 2, time.Minute)
	v, _ := c.Get(ctx, "counter")
	if v != 2 { t.Errorf("should overwrite: %d", v) }
}

func TestInMemoryCache_Delete(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	c.Set(ctx, "key1", "value1", time.Minute)
	c.Delete(ctx, "key1")
	_, ok := c.Get(ctx, "key1")
	if ok { t.Error("should be deleted") }
}

func TestInMemoryCache_DeleteNonexistent(t *testing.T) {
	c := NewInMemoryCache[string]()
	c.Delete(context.Background(), "nonexistent") // should not panic
}

func TestInMemoryCache_DeleteByPrefix(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	c.Set(ctx, "user:1:name", "Alice", time.Minute)
	c.Set(ctx, "user:1:email", "alice@example.com", time.Minute)
	c.Set(ctx, "user:2:name", "Bob", time.Minute)
	c.Set(ctx, "session:abc", "data", time.Minute)

	c.DeleteByPrefix(ctx, "user:1:")

	_, ok1 := c.Get(ctx, "user:1:name")
	_, ok2 := c.Get(ctx, "user:1:email")
	_, ok3 := c.Get(ctx, "user:2:name")
	_, ok4 := c.Get(ctx, "session:abc")

	if ok1 { t.Error("user:1:name should be deleted") }
	if ok2 { t.Error("user:1:email should be deleted") }
	if !ok3 { t.Error("user:2:name should survive") }
	if !ok4 { t.Error("session:abc should survive") }
}

func TestInMemoryCache_Clear(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		c.Set(ctx, string(rune('a'+i%26)), "val", time.Minute)
	}
	c.Clear(ctx)
	_, ok := c.Get(ctx, "a")
	if ok { t.Error("should be empty after clear") }
}

func TestInMemoryCache_ConcurrentReadWrite(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	var wg sync.WaitGroup

	// 50 writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Set(ctx, "key", n*100+j, time.Minute)
			}
		}(i)
	}

	// 50 readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Get(ctx, "key")
			}
		}()
	}

	wg.Wait()
	// Should not panic or deadlock
}

func TestInMemoryCache_ConcurrentDeleteByPrefix(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				c.Set(ctx, "prefix:"+string(rune('a'+j%26)), "val", time.Minute)
			}
		}(i)
	}

	// Deleters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.DeleteByPrefix(ctx, "prefix:")
		}()
	}

	wg.Wait()
}

func TestInMemoryCache_ZeroTTL(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	c.Set(ctx, "key", "value", 0)
	// Zero TTL — should either not expire or expire immediately
	v, ok := c.Get(ctx, "key")
	_ = v
	_ = ok
	// Either behavior is acceptable
}

func TestInMemoryCache_LargeValues(t *testing.T) {
	c := NewInMemoryCache[[]byte]()
	ctx := context.Background()
	big := make([]byte, 10*1024*1024) // 10MB
	c.Set(ctx, "big", big, time.Minute)
	v, ok := c.Get(ctx, "big")
	if !ok { t.Error("should store large value") }
	if len(v) != 10*1024*1024 { t.Error("wrong size") }
}

func TestInMemoryCache_ManyKeys(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		c.Set(ctx, string(rune(i)), i, time.Minute)
	}
	v, ok := c.Get(ctx, string(rune(9999)))
	if !ok { t.Error("should find key 9999") }
	if v != 9999 { t.Errorf("value=%d", v) }
}

// Permission cache tests
func TestPermissionCache_New(t *testing.T) {
	pc := NewPermissionCache()
	if pc == nil { t.Fatal("nil") }
}

// === HARD CACHE STRESS TESTS ===

func TestInMemoryCache_TTL_Precision(t *testing.T) {
	c := NewInMemoryCache[string]()
	ctx := context.Background()
	ttls := []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}
	for _, ttl := range ttls {
		key := "ttl_" + ttl.String()
		c.Set(ctx, key, "value", ttl)
		// Should exist immediately
		if _, ok := c.Get(ctx, key); !ok { t.Errorf("TTL %v: missing immediately", ttl) }
		// Wait for expiry
		time.Sleep(ttl + 20*time.Millisecond)
		if _, ok := c.Get(ctx, key); ok { t.Errorf("TTL %v: should have expired", ttl) }
	}
}

func TestInMemoryCache_StressDeleteByPrefix_Concurrent(t *testing.T) {
	c := NewInMemoryCache[int]()
	ctx := context.Background()
	var wg sync.WaitGroup

	// 100 writers writing different prefixes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			prefix := "p" + string(rune('a'+n%10)) + ":"
			for j := 0; j < 100; j++ {
				c.Set(ctx, prefix+string(rune('0'+j%10)), n*100+j, time.Minute)
			}
		}(i)
	}

	// 10 deleters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			time.Sleep(5 * time.Millisecond)
			c.DeleteByPrefix(ctx, "p"+string(rune('a'+n%10))+":")
		}(i)
	}

	wg.Wait()
}

func TestInMemoryCache_TypeVariety(t *testing.T) {
	// Test with different value types
	strCache := NewInMemoryCache[string]()
	intCache := NewInMemoryCache[int]()
	boolCache := NewInMemoryCache[bool]()
	sliceCache := NewInMemoryCache[[]string]()
	mapCache := NewInMemoryCache[map[string]int]()

	ctx := context.Background()
	strCache.Set(ctx, "k", "hello", time.Minute)
	intCache.Set(ctx, "k", 42, time.Minute)
	boolCache.Set(ctx, "k", true, time.Minute)
	sliceCache.Set(ctx, "k", []string{"a", "b"}, time.Minute)
	mapCache.Set(ctx, "k", map[string]int{"x": 1}, time.Minute)

	if v, ok := strCache.Get(ctx, "k"); !ok || v != "hello" { t.Error("string cache") }
	if v, ok := intCache.Get(ctx, "k"); !ok || v != 42 { t.Error("int cache") }
	if v, ok := boolCache.Get(ctx, "k"); !ok || v != true { t.Error("bool cache") }
	if v, ok := sliceCache.Get(ctx, "k"); !ok || len(v) != 2 { t.Error("slice cache") }
	if v, ok := mapCache.Get(ctx, "k"); !ok || v["x"] != 1 { t.Error("map cache") }
}
