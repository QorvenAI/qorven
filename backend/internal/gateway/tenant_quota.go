// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qorvenai/qorven/internal/byom"
)

// ──────────────────── Metrics (Gap #6 closure) ────────────────────
//
// Process-wide counter of quota denials, labeled by tenant + reason.
// Two reasons map to the two gates Acquire enforces:
//
//   reason="rate"        — token bucket empty (sustained over-rate)
//   reason="concurrency" — concurrency semaphore full
//
// Cardinality is bounded by the distinct tenant set that actually hits
// the limits; tenants well under their caps never create a series. The
// idle-bucket sweeper does NOT prune metric entries — operators care
// about historical noisy-neighbor events even after that tenant quiets
// down. If cardinality ever becomes a concern (millions of tenants
// hitting caps) we'd add a TTL sweep here too.

type quotaDenialKey struct {
	tenant string
	reason string // "rate" | "concurrency"
}

var (
	quotaDenialMu sync.Mutex
	quotaDenials  = map[quotaDenialKey]*atomic.Int64{}
)

func recordQuotaDenial(tenant, reason string) {
	k := quotaDenialKey{tenant: tenant, reason: reason}
	quotaDenialMu.Lock()
	c, ok := quotaDenials[k]
	if !ok {
		c = &atomic.Int64{}
		quotaDenials[k] = c
	}
	quotaDenialMu.Unlock()
	c.Add(1)
}

// writeQuotaMetrics emits the tenant_quota_denials_total counter in
// Prometheus text exposition. Called from HandleMetrics alongside
// the other hand-written counters. Deterministic output order so
// scrape diffs are legible.
func writeQuotaMetrics(w io.Writer) {
	quotaDenialMu.Lock()
	keys := make([]quotaDenialKey, 0, len(quotaDenials))
	vals := make(map[quotaDenialKey]int64, len(quotaDenials))
	for k, c := range quotaDenials {
		keys = append(keys, k)
		vals[k] = c.Load()
	}
	quotaDenialMu.Unlock()
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.tenant != b.tenant {
			return a.tenant < b.tenant
		}
		return a.reason < b.reason
	})

	fmt.Fprintln(w, "# HELP tenant_quota_denials_total Tenant quota denials by reason.")
	fmt.Fprintln(w, "# TYPE tenant_quota_denials_total counter")
	for _, k := range keys {
		fmt.Fprintf(w,
			"tenant_quota_denials_total{tenant=%q,reason=%q} %d\n",
			k.tenant, k.reason, vals[k])
	}
}

// resetQuotaMetricsForTests drops counters. Tests only.
func resetQuotaMetricsForTests() {
	quotaDenialMu.Lock()
	quotaDenials = map[quotaDenialKey]*atomic.Int64{}
	quotaDenialMu.Unlock()
}

// TenantQuota is the Phase 7 per-tenant resource limiter. Two
// dimensions enforced simultaneously:
//
//   1. Concurrent requests. Each tenant holds at most MaxConcurrent
//      in-flight requests through the gated endpoints; the
//      MaxConcurrent+1 request gets 429 with Retry-After: 1.
//
//   2. Sustained rate. A token bucket with capacity == refill rate
//      per second. Drains one token per request; 429 when empty.
//
// Process-local (no Redis). A multi-process gateway deployment would
// scale the effective limits by the process count, which is fine for
// our single-binary target. Switching to Redis is a future refactor
// when we ship a horizontal deployment story; until then, a single
// process is the contract AGENTS.md §1 already documents.
//
// ## Why not the existing NewRateLimiter?
//
// gateway/rate_limit.go has an IP-keyed limiter. IP limiting catches
// shared-IP spam (office behind one NAT) but doesn't protect against
// a tenant with many client hosts. Tenant limiting is the boundary
// that actually maps to our billing + SaaS resource model.
//
// ## Limiter lifetime
//
// Per-tenant state is lazily created on first request and reclaimed
// after an idle period (we sweep every 5 minutes, drop buckets
// unused for 10). A hot tenant keeps its bucket; a tenant that
// stops sending requests releases memory. 5k idle tenants =
// ~500 KB, acceptable.
type TenantQuota struct {
	// maxConcurrent is the simultaneous-requests cap. 0 disables.
	maxConcurrent int
	// ratePerSecond is the token bucket refill rate. 0 disables.
	ratePerSecond int

	mu      sync.Mutex
	buckets map[string]*tenantBucket
}

type tenantBucket struct {
	// semaphore for concurrency cap. Buffered channel of size
	// maxConcurrent; send acquires, recv releases.
	sem chan struct{}

	// Token bucket for rate limit.
	tokens   float64
	capacity float64
	lastFill time.Time
	rateMu   sync.Mutex

	// lastUsed tracks the bucket's last acquire/release for idle
	// sweep eligibility.
	lastUsedNano int64
	lastUsedMu   sync.Mutex
}

// NewTenantQuota reads the limits from byom.Load() and starts the
// idle-bucket sweeper. Pass background ctx — sweeper stops when it
// cancels (i.e. process shutdown).
func NewTenantQuota(ctx context.Context) *TenantQuota {
	t := byom.Load()
	q := &TenantQuota{
		maxConcurrent: t.TenantMaxConcurrent,
		ratePerSecond: t.TenantRateLimitPerSecond,
		buckets:       make(map[string]*tenantBucket),
	}
	// Sweep idle buckets so a short-lived tenant's state doesn't
	// leak forever. No-op when both limiters are disabled.
	if q.maxConcurrent > 0 || q.ratePerSecond > 0 {
		go q.sweepIdle(ctx, 5*time.Minute, 10*time.Minute)
	}
	return q
}

// bucket returns (creating if needed) the per-tenant state.
// Safe for concurrent callers.
func (q *TenantQuota) bucket(tenantID string) *tenantBucket {
	q.mu.Lock()
	defer q.mu.Unlock()
	if b, ok := q.buckets[tenantID]; ok {
		b.markUsed()
		return b
	}
	b := &tenantBucket{
		lastFill: time.Now(),
		capacity: float64(q.ratePerSecond),
		tokens:   float64(q.ratePerSecond),
	}
	if q.maxConcurrent > 0 {
		b.sem = make(chan struct{}, q.maxConcurrent)
	}
	b.markUsed()
	q.buckets[tenantID] = b
	return b
}

func (b *tenantBucket) markUsed() {
	b.lastUsedMu.Lock()
	b.lastUsedNano = time.Now().UnixNano()
	b.lastUsedMu.Unlock()
}

func (b *tenantBucket) idleFor() time.Duration {
	b.lastUsedMu.Lock()
	defer b.lastUsedMu.Unlock()
	return time.Since(time.Unix(0, b.lastUsedNano))
}

// Acquire attempts to reserve a concurrency slot + consume a rate
// token for the tenant. Returns (ok, retryAfter). ok=true means
// the caller proceeds; ok=false means the caller MUST return 429
// with the retryAfter duration in the header. On ok=true the caller
// MUST defer q.Release(tenantID) so the slot is returned.
//
// tenantID=="" bypasses the limiter — the endpoint wasn't
// tenant-scoped (setup, auth, legacy unauth paths).
func (q *TenantQuota) Acquire(tenantID string) (ok bool, retryAfter time.Duration) {
	if tenantID == "" {
		return true, 0
	}
	if q.maxConcurrent <= 0 && q.ratePerSecond <= 0 {
		return true, 0
	}
	b := q.bucket(tenantID)

	// 1. Rate gate. Refill tokens, then try to consume one.
	if q.ratePerSecond > 0 {
		b.rateMu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.lastFill).Seconds()
		b.tokens = minFloat(b.capacity, b.tokens+elapsed*float64(q.ratePerSecond))
		b.lastFill = now
		if b.tokens < 1 {
			// How long until we have 1 token?
			need := 1 - b.tokens
			retry := time.Duration(need / float64(q.ratePerSecond) * float64(time.Second))
			if retry < time.Second {
				retry = time.Second
			}
			b.rateMu.Unlock()
			recordQuotaDenial(tenantID, "rate")
			return false, retry
		}
		b.tokens--
		b.rateMu.Unlock()
	}

	// 2. Concurrency gate. Non-blocking — a full semaphore means the
	// tenant is already at its ceiling; return fast rather than
	// queue. Queueing is what the caller's client-side retry is for.
	if q.maxConcurrent > 0 {
		select {
		case b.sem <- struct{}{}:
			// acquired
		default:
			// refund the rate token we took — we aren't running.
			if q.ratePerSecond > 0 {
				b.rateMu.Lock()
				b.tokens = minFloat(b.capacity, b.tokens+1)
				b.rateMu.Unlock()
			}
			recordQuotaDenial(tenantID, "concurrency")
			return false, time.Second
		}
	}
	return true, 0
}

// Release returns a concurrency slot to the tenant. Safe to call
// even when Acquire returned false (no-op).
func (q *TenantQuota) Release(tenantID string) {
	if tenantID == "" || q.maxConcurrent <= 0 {
		return
	}
	q.mu.Lock()
	b, ok := q.buckets[tenantID]
	q.mu.Unlock()
	if !ok || b.sem == nil {
		return
	}
	select {
	case <-b.sem:
	default:
		// Unbalanced release — log + ignore. Indicates a caller
		// that skipped Acquire or called Release twice; safer to
		// return than panic.
	}
	b.markUsed()
}

// sweepIdle runs until ctx is done, periodically dropping buckets
// that have been idle longer than maxIdle. Minimizes memory for
// deployments with churny tenant sets.
func (q *TenantQuota) sweepIdle(ctx context.Context, interval, maxIdle time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			q.mu.Lock()
			for id, b := range q.buckets {
				// Skip buckets with active concurrency holders.
				if len(b.sem) > 0 {
					continue
				}
				if b.idleFor() > maxIdle {
					delete(q.buckets, id)
				}
			}
			q.mu.Unlock()
		}
	}
}

// TenantQuotaMiddleware is a per-request wrapper that calls
// Acquire/Release around the handler. On quota exhaustion it writes
// 429 with a Retry-After header and a structured JSON body so
// admin UIs + AI agents can surface the specific limit.
//
// Placement: install AFTER AuthMiddlewareV2 (so user.TenantID is
// resolved) and BEFORE TenantScopeMiddleware (so we don't burn a
// DB transaction on a request about to 429).
func (gw *Gateway) TenantQuotaMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gw.tenantQuota == nil {
			next.ServeHTTP(w, r)
			return
		}
		user := userFromContext(r.Context())
		tenantID := ""
		if user != nil {
			tenantID = user.TenantID
		}
		ok, retry := gw.tenantQuota.Acquire(tenantID)
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error":       "tenant quota exhausted",
				"code":        "tenant_quota",
				"retry_after": retry.Seconds(),
				"tenant_id":   tenantID,
				"detail":      fmt.Sprintf("tenant exceeded its concurrent-request or rate limit; retry in %s", retry),
			})
			return
		}
		defer gw.tenantQuota.Release(tenantID)
		next.ServeHTTP(w, r)
	})
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
