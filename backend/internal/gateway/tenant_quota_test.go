// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/byom"
)

// TestTenantQuota_EmptyTenantBypasses — endpoints that run under
// an anonymous caller (setup, health) must pass through. The quota
// layer is tenant-keyed; no tenant → no gate.
func TestTenantQuota_EmptyTenantBypasses(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      1,
		TenantRateLimitPerSecond: 1,
	})
	defer restore()

	q := NewTenantQuota(context.Background())
	for i := 0; i < 5; i++ {
		ok, _ := q.Acquire("")
		if !ok {
			t.Fatalf("empty tenant should always be admitted; got 429 on attempt %d", i)
		}
	}
}

// TestTenantQuota_EmitsPrometheusMetrics — Phase 8 Gap #6 closure.
// Each denial path bumps tenant_quota_denials_total with the right
// reason label. Operators depend on this metric to spot noisy
// neighbors before they turn into a page.
func TestTenantQuota_EmitsPrometheusMetrics(t *testing.T) {
	resetQuotaMetricsForTests()

	// Scenario A: only concurrency is tight. Rate is generous so
	// rate denials cannot fire; any denial MUST be concurrency.
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      1,
		TenantRateLimitPerSecond: 1000,
	})
	defer restore()

	q := NewTenantQuota(context.Background())

	if ok, _ := q.Acquire("A"); !ok {
		t.Fatalf("A first acquire should succeed")
	}
	// Hold the slot; next Acquire denies on concurrency.
	if ok, _ := q.Acquire("A"); ok {
		t.Fatalf("A second acquire should deny on concurrency")
	}
	q.Release("A")

	// Scenario B: rate is tight, concurrency is generous. Burn the
	// rate bucket via acquire+release, then re-attempt — denial
	// fires on rate.
	restore2 := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      1000,
		TenantRateLimitPerSecond: 1,
	})
	defer restore2()

	// NewTenantQuota snapshots byom at construction, so we need a
	// fresh instance after flipping the knobs.
	q2 := NewTenantQuota(context.Background())
	if ok, _ := q2.Acquire("B"); !ok {
		t.Fatalf("B first acquire should succeed")
	}
	q2.Release("B")
	if ok, _ := q2.Acquire("B"); ok {
		t.Fatalf("B second acquire should deny on rate (bucket drained)")
	}

	var buf bytes.Buffer
	writeQuotaMetrics(&buf)
	out := buf.String()

	wantLines := []string{
		`tenant_quota_denials_total{tenant="A",reason="concurrency"} 1`,
		`tenant_quota_denials_total{tenant="B",reason="rate"} 1`,
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("metrics missing %q. full:\n%s", want, out)
		}
	}
}

// TestTenantQuota_ConcurrencyCap — a tenant at its cap gets 429
// on the next request. The test uses a cap of 2 and deliberately
// holds slots without releasing.
func TestTenantQuota_ConcurrencyCap(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      2,
		TenantRateLimitPerSecond: 1000, // rate-limiter effectively off
	})
	defer restore()

	q := NewTenantQuota(context.Background())
	tenant := "t-conc"

	// First two acquires succeed.
	for i := 0; i < 2; i++ {
		if ok, _ := q.Acquire(tenant); !ok {
			t.Fatalf("acquire %d should succeed under cap 2", i)
		}
	}
	// Third hits the cap — 429 + Retry-After.
	ok, retry := q.Acquire(tenant)
	if ok {
		t.Fatalf("third acquire should fail; tenant already holds 2 slots")
	}
	if retry <= 0 {
		t.Fatalf("retry-after must be positive; got %s", retry)
	}

	// Release one, next acquire succeeds.
	q.Release(tenant)
	if ok, _ := q.Acquire(tenant); !ok {
		t.Fatalf("after release, next acquire should succeed")
	}
}

// TestTenantQuota_RateLimit — token bucket drains; 429 when empty;
// a short sleep refills.
func TestTenantQuota_RateLimit(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      1000, // cap effectively off
		TenantRateLimitPerSecond: 2,    // 2 rps
	})
	defer restore()

	q := NewTenantQuota(context.Background())
	tenant := "t-rate"
	// Release immediately to keep concurrency slots free.
	acquireAndRelease := func() (bool, time.Duration) {
		ok, retry := q.Acquire(tenant)
		if ok {
			q.Release(tenant)
		}
		return ok, retry
	}

	// Burst capacity is 2. First two succeed, third fails.
	for i := 0; i < 2; i++ {
		if ok, _ := acquireAndRelease(); !ok {
			t.Fatalf("burst %d should succeed under rate=2", i)
		}
	}
	ok, retry := acquireAndRelease()
	if ok {
		t.Fatalf("third acquire should exhaust the bucket")
	}
	if retry < time.Second {
		t.Fatalf("retry-after should be >= 1s; got %s", retry)
	}

	// Wait for the bucket to refill.
	time.Sleep(1100 * time.Millisecond)
	if ok, _ := acquireAndRelease(); !ok {
		t.Fatalf("after refill, acquire should succeed")
	}
}

// TestTenantQuota_TenantsIsolated — one noisy tenant at its cap
// must not affect a different tenant.
func TestTenantQuota_TenantsIsolated(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      1,
		TenantRateLimitPerSecond: 1,
	})
	defer restore()

	q := NewTenantQuota(context.Background())

	// Tenant A saturates.
	if ok, _ := q.Acquire("A"); !ok {
		t.Fatalf("A first acquire")
	}
	if ok, _ := q.Acquire("A"); ok {
		t.Fatalf("A second acquire should fail")
	}

	// Tenant B unaffected — independent bucket.
	if ok, _ := q.Acquire("B"); !ok {
		t.Fatalf("B must be admitted despite A being saturated")
	}
}

// TestTenantQuota_DisabledByZero — both limits set to 0 means
// unlimited. The middleware becomes a pass-through.
func TestTenantQuota_DisabledByZero(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      0,
		TenantRateLimitPerSecond: 0,
	})
	defer restore()

	q := NewTenantQuota(context.Background())
	for i := 0; i < 100; i++ {
		if ok, _ := q.Acquire("any"); !ok {
			t.Fatalf("limiter disabled by 0 must admit every request; failed on %d", i)
		}
	}
}

// TestTenantQuota_ConcurrentCallersRaceSafe — many goroutines
// hammering Acquire/Release don't corrupt state. Mostly a -race
// detector target.
func TestTenantQuota_ConcurrentCallersRaceSafe(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		TenantMaxConcurrent:      4,
		TenantRateLimitPerSecond: 1000,
	})
	defer restore()

	q := NewTenantQuota(context.Background())
	tenant := "t-race"
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				ok, _ := q.Acquire(tenant)
				if ok {
					q.Release(tenant)
				}
			}
		}()
	}
	wg.Wait()
	// If we reach here without deadlock or panic, the locks are
	// sound. Final state: bucket exists; concurrency slot count
	// back to 0 (len of channel == 0).
	b := q.bucket(tenant)
	if b.sem != nil && len(b.sem) != 0 {
		t.Fatalf("leaked concurrency slots: len(sem)=%d", len(b.sem))
	}
}
