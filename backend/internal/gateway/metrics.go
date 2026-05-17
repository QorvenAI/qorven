// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/qorvenai/qorven/internal/plugins/wasm"
)

// Metrics tracks request counts and latencies for Prometheus.
type Metrics struct {
	RequestCount   atomic.Int64
	ErrorCount     atomic.Int64
	ActiveAgents   atomic.Int64
	LLMCallCount   atomic.Int64
	LLMErrorCount  atomic.Int64
	TotalLatencyMs atomic.Int64
}

var GlobalMetrics = &Metrics{}

// MetricsMiddleware records request metrics.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		GlobalMetrics.RequestCount.Add(1)
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		ms := time.Since(start).Milliseconds()
		GlobalMetrics.TotalLatencyMs.Add(ms)
		if rw.status >= 500 {
			GlobalMetrics.ErrorCount.Add(1)
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher — required for SSE streaming
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker — required for WebSocket upgrades
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
}

// HandleMetrics serves Prometheus-compatible metrics.
func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	m := GlobalMetrics
	reqCount := m.RequestCount.Load()
	avgMs := int64(0)
	if reqCount > 0 {
		avgMs = m.TotalLatencyMs.Load() / reqCount
	}
	fmt.Fprintf(w, "# HELP qorven_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE qorven_requests_total counter\n")
	fmt.Fprintf(w, "qorven_requests_total %d\n", reqCount)
	fmt.Fprintf(w, "# HELP qorven_errors_total Total HTTP 5xx errors\n")
	fmt.Fprintf(w, "# TYPE qorven_errors_total counter\n")
	fmt.Fprintf(w, "qorven_errors_total %d\n", m.ErrorCount.Load())
	fmt.Fprintf(w, "# HELP qorven_llm_calls_total Total LLM API calls\n")
	fmt.Fprintf(w, "# TYPE qorven_llm_calls_total counter\n")
	fmt.Fprintf(w, "qorven_llm_calls_total %d\n", m.LLMCallCount.Load())
	fmt.Fprintf(w, "# HELP qorven_llm_errors_total Total LLM API errors\n")
	fmt.Fprintf(w, "# TYPE qorven_llm_errors_total counter\n")
	fmt.Fprintf(w, "qorven_llm_errors_total %d\n", m.LLMErrorCount.Load())
	fmt.Fprintf(w, "# HELP qorven_avg_latency_ms Average request latency\n")
	fmt.Fprintf(w, "# TYPE qorven_avg_latency_ms gauge\n")
	fmt.Fprintf(w, "qorven_avg_latency_ms %d\n", avgMs)
	fmt.Fprintf(w, "# HELP qorven_active_agents Number of active agents\n")
	fmt.Fprintf(w, "# TYPE qorven_active_agents gauge\n")
	fmt.Fprintf(w, "qorven_active_agents %d\n", m.ActiveAgents.Load())

	// Wasm plugin metrics (Phase 5.2 Gap #6 closure). The wasm
	// package owns its counters; we just delegate the exposition so
	// operators have a single scrape endpoint.
	wasm.WriteMetrics(w)

	// tenant quota denials. Lives in the gateway package
	// so it can share the atomic.Int64 counter with the Acquire()
	// hot path without a cross-package import.
	writeQuotaMetrics(w)
}
