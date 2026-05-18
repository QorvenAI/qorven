// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package wasm

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// metrics is the in-process counters for Wasm plugin runtime
// observability. Mirrors the pattern in internal/gateway/metrics.go:
// hand-written Prometheus text exposition, no client library —
// Qorven's metrics surface is a single /metrics handler and we want
// this package to drop in next to it without adding a dependency.
//
// Exposed series:
//
//   plugins_wasm_invocations_total{plugin, tenant, outcome}
//       counter — one per Invoke call.
//       outcome ∈ ok | timeout | trap | exit_nonzero | truncated_stdout
//       "truncated_stdout" fires alongside the primary outcome so a
//       single invoke can bump two series.
//
//   plugins_wasm_duration_ms_sum{plugin, tenant}
//   plugins_wasm_duration_ms_count{plugin, tenant}
//       two counters that together form a simple average. Not a
//       full Prometheus histogram — those require bucket config a
//       lightweight package shouldn't own. Operators who want p95/p99
//       run a scraper that converts sum+count into the native
//       histogram shape. Documented in AGENTS.md.
//
//   plugins_wasm_load_errors_total
//       global counter for LoadPlugin failures — unlabeled because
//       load errors happen before tenant context is known.
//
// The label set is small and bounded: we only keep (plugin, tenant,
// outcome). Unbounded labels (e.g. "error_code") would make this a
// cardinality bomb.

type counterKey struct {
	plugin  string
	tenant  string
	outcome string
}

type metrics struct {
	mu            sync.Mutex
	invocations   map[counterKey]*atomic.Int64
	durationSum   map[counterKey]*atomic.Int64 // ms
	durationCount map[counterKey]*atomic.Int64
	loadErrors    atomic.Int64
}

// globalMetrics is shared across all Host instances. Unlike Host
// itself (potentially multiple per process for tests), metrics form
// a single observable view of all plugin activity in the binary.
var globalMetrics = &metrics{
	invocations:   map[counterKey]*atomic.Int64{},
	durationSum:   map[counterKey]*atomic.Int64{},
	durationCount: map[counterKey]*atomic.Int64{},
}

// recordInvocation updates counters for one Invoke call. outcome is
// the primary classification; if the stdout was truncated, pass
// truncated=true and a second counter bumps under outcome=truncated_stdout.
func recordInvocation(plugin, tenant, outcome string, elapsed time.Duration, truncated bool) {
	k := counterKey{plugin: plugin, tenant: tenant, outcome: outcome}
	globalMetrics.mu.Lock()
	if _, ok := globalMetrics.invocations[k]; !ok {
		globalMetrics.invocations[k] = &atomic.Int64{}
	}
	globalMetrics.invocations[k].Add(1)

	dkey := counterKey{plugin: plugin, tenant: tenant}
	if _, ok := globalMetrics.durationSum[dkey]; !ok {
		globalMetrics.durationSum[dkey] = &atomic.Int64{}
		globalMetrics.durationCount[dkey] = &atomic.Int64{}
	}
	globalMetrics.durationSum[dkey].Add(elapsed.Milliseconds())
	globalMetrics.durationCount[dkey].Add(1)

	if truncated {
		tk := counterKey{plugin: plugin, tenant: tenant, outcome: "truncated_stdout"}
		if _, ok := globalMetrics.invocations[tk]; !ok {
			globalMetrics.invocations[tk] = &atomic.Int64{}
		}
		globalMetrics.invocations[tk].Add(1)
	}
	globalMetrics.mu.Unlock()
}

func recordLoadError() { globalMetrics.loadErrors.Add(1) }

// WriteMetrics emits the Prometheus text exposition for every Wasm
// plugin counter. The caller (gateway/metrics.go:HandleMetrics) calls
// this inside the /metrics handler.
func WriteMetrics(w io.Writer) {
	globalMetrics.mu.Lock()
	// Copy keys out so we can release the lock before Fprintf.
	invKeys := make([]counterKey, 0, len(globalMetrics.invocations))
	invVals := make(map[counterKey]int64, len(globalMetrics.invocations))
	for k, v := range globalMetrics.invocations {
		invKeys = append(invKeys, k)
		invVals[k] = v.Load()
	}
	durKeys := make([]counterKey, 0, len(globalMetrics.durationSum))
	durSum := make(map[counterKey]int64, len(globalMetrics.durationSum))
	durCnt := make(map[counterKey]int64, len(globalMetrics.durationCount))
	for k, v := range globalMetrics.durationSum {
		durKeys = append(durKeys, k)
		durSum[k] = v.Load()
		if c, ok := globalMetrics.durationCount[k]; ok {
			durCnt[k] = c.Load()
		}
	}
	loadErrors := globalMetrics.loadErrors.Load()
	globalMetrics.mu.Unlock()

	// Deterministic output order helps operators diff scrapes.
	sort.Slice(invKeys, func(i, j int) bool {
		a, b := invKeys[i], invKeys[j]
		if a.plugin != b.plugin {
			return a.plugin < b.plugin
		}
		if a.tenant != b.tenant {
			return a.tenant < b.tenant
		}
		return a.outcome < b.outcome
	})
	sort.Slice(durKeys, func(i, j int) bool {
		a, b := durKeys[i], durKeys[j]
		if a.plugin != b.plugin {
			return a.plugin < b.plugin
		}
		return a.tenant < b.tenant
	})

	fmt.Fprintln(w, "# HELP plugins_wasm_invocations_total Total Wasm plugin invocations by outcome.")
	fmt.Fprintln(w, "# TYPE plugins_wasm_invocations_total counter")
	for _, k := range invKeys {
		fmt.Fprintf(w,
			"plugins_wasm_invocations_total{plugin=%q,tenant=%q,outcome=%q} %d\n",
			k.plugin, k.tenant, k.outcome, invVals[k])
	}

	fmt.Fprintln(w, "# HELP plugins_wasm_duration_ms_sum Sum of Wasm plugin invocation durations in milliseconds.")
	fmt.Fprintln(w, "# TYPE plugins_wasm_duration_ms_sum counter")
	for _, k := range durKeys {
		fmt.Fprintf(w,
			"plugins_wasm_duration_ms_sum{plugin=%q,tenant=%q} %d\n",
			k.plugin, k.tenant, durSum[k])
	}
	fmt.Fprintln(w, "# HELP plugins_wasm_duration_ms_count Count of Wasm plugin invocations recorded against the duration sum.")
	fmt.Fprintln(w, "# TYPE plugins_wasm_duration_ms_count counter")
	for _, k := range durKeys {
		fmt.Fprintf(w,
			"plugins_wasm_duration_ms_count{plugin=%q,tenant=%q} %d\n",
			k.plugin, k.tenant, durCnt[k])
	}

	fmt.Fprintln(w, "# HELP plugins_wasm_load_errors_total Total LoadPlugin failures.")
	fmt.Fprintln(w, "# TYPE plugins_wasm_load_errors_total counter")
	fmt.Fprintf(w, "plugins_wasm_load_errors_total %d\n", loadErrors)
}

// resetMetricsForTests wipes all state. Used only from _test.go.
func resetMetricsForTests() {
	globalMetrics.mu.Lock()
	globalMetrics.invocations = map[counterKey]*atomic.Int64{}
	globalMetrics.durationSum = map[counterKey]*atomic.Int64{}
	globalMetrics.durationCount = map[counterKey]*atomic.Int64{}
	globalMetrics.loadErrors.Store(0)
	globalMetrics.mu.Unlock()
}
