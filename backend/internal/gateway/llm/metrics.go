// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"fmt"
	"io"
	"sync/atomic"
)

// GatewayMetrics holds atomic counters for the AI Gateway pipeline.
// All fields are safe for concurrent access without locks.
// Use the package-level Metrics variable; do not allocate additional instances.
type GatewayMetrics struct {
	// Request counts
	RequestsTotal    atomic.Int64 // all requests entering the pipeline
	RequestsErrors   atomic.Int64 // requests that returned an error
	CacheHits        atomic.Int64 // requests served from cache (exact or semantic)
	CacheMisses      atomic.Int64 // requests that missed the cache
	BudgetDenials    atomic.Int64 // requests rejected by the budget engine

	// Token volumes (input / output / thinking / cache-write / cache-read)
	TokensIn         atomic.Int64
	TokensOut        atomic.Int64
	TokensThinking   atomic.Int64
	TokensCacheWrite atomic.Int64
	TokensCacheRead  atomic.Int64

	// Cost in µUSD (1 USD = 1,000,000 µUSD)
	CostTotalUUSD    atomic.Int64

	// Circuit breaker trips (key went open)
	CircuitTrips     atomic.Int64

	// Unknown-pricing events (tokens billed at $0)
	PricingMissingCalls atomic.Int64
}

// Metrics is the singleton for the gateway LLM pipeline.
var Metrics = &GatewayMetrics{}

// recordRequest is called by the pipeline on every completed request.
func recordRequest(resp *GatewayResponse, cacheHit bool, err error) {
	Metrics.RequestsTotal.Add(1)
	if err != nil {
		Metrics.RequestsErrors.Add(1)
		return
	}
	if cacheHit {
		Metrics.CacheHits.Add(1)
	} else {
		Metrics.CacheMisses.Add(1)
	}
	if resp == nil {
		return
	}
	Metrics.TokensIn.Add(int64(resp.Cost.TokensIn))
	Metrics.TokensOut.Add(int64(resp.Cost.TokensOut))
	Metrics.TokensThinking.Add(int64(resp.Cost.TokensThinking))
	Metrics.TokensCacheWrite.Add(int64(resp.Cost.TokensCacheWrite))
	Metrics.TokensCacheRead.Add(int64(resp.Cost.TokensCacheRead))
	Metrics.CostTotalUUSD.Add(resp.Cost.TotalUUSD)
	if resp.Cost.PricingMissing {
		Metrics.PricingMissingCalls.Add(1)
	}
}

// WriteMetrics emits Prometheus text-format metrics for the gateway pipeline.
// Call this from the /metrics handler so operators get all gateway stats in one scrape.
func WriteMetrics(w io.Writer) {
	m := Metrics

	lines := []struct{ help, typ, name string; val int64 }{
		{"Total requests through the AI Gateway pipeline", "counter", "gateway_requests_total", m.RequestsTotal.Load()},
		{"Requests that returned an error", "counter", "gateway_request_errors_total", m.RequestsErrors.Load()},
		{"Requests served from cache (exact or semantic)", "counter", "gateway_cache_hits_total", m.CacheHits.Load()},
		{"Requests that missed the cache", "counter", "gateway_cache_misses_total", m.CacheMisses.Load()},
		{"Requests rejected by the per-agent budget engine", "counter", "gateway_budget_denials_total", m.BudgetDenials.Load()},
		{"Input tokens sent to LLM providers", "counter", "gateway_tokens_input_total", m.TokensIn.Load()},
		{"Output tokens received from LLM providers", "counter", "gateway_tokens_output_total", m.TokensOut.Load()},
		{"Thinking (reasoning) tokens", "counter", "gateway_tokens_thinking_total", m.TokensThinking.Load()},
		{"Cache-write tokens (Anthropic prompt caching)", "counter", "gateway_tokens_cache_write_total", m.TokensCacheWrite.Load()},
		{"Cache-read tokens (Anthropic prompt caching)", "counter", "gateway_tokens_cache_read_total", m.TokensCacheRead.Load()},
		{"Total spend in micro-USD (divide by 1,000,000 for USD)", "counter", "gateway_cost_uusd_total", m.CostTotalUUSD.Load()},
		{"Times a circuit breaker opened for a provider key", "counter", "gateway_circuit_trips_total", m.CircuitTrips.Load()},
		{"LLM calls where model pricing was unknown (billed at $0)", "counter", "gateway_pricing_missing_calls_total", m.PricingMissingCalls.Load()},
	}

	for _, l := range lines {
		fmt.Fprintf(w, "# HELP %s %s\n", l.name, l.help)
		fmt.Fprintf(w, "# TYPE %s %s\n", l.name, l.typ)
		fmt.Fprintf(w, "%s %d\n", l.name, l.val)
	}

	// Cache hit rate gauge (computed, not stored atomically)
	hits := m.CacheHits.Load()
	total := m.RequestsTotal.Load()
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	fmt.Fprintf(w, "# HELP gateway_cache_hit_rate Fraction of requests served from cache (0.0–1.0)\n")
	fmt.Fprintf(w, "# TYPE gateway_cache_hit_rate gauge\n")
	fmt.Fprintf(w, "gateway_cache_hit_rate %g\n", hitRate)
}
