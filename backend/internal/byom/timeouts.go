// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package byom captures Qorven's "Bring Your Own Model" operational
// reality and exposes the knobs it creates.
//
// Users deploy Qorven on everything from production Kubernetes clusters
// running Opus-class inference to home labs running Llama 3.1 8B on CPU.
// Sensible defaults for one deployment are absurd for the other:
//
//   - A 7-shot tool-call that returns in 800 ms on a hosted Opus model
//     can take 5 minutes on a local 8B Llama via Ollama. Stale-plan
//     detection tuned for the former will mark the latter as "stuck"
//     during legitimate inference and spuriously recover it.
//   - A permission-gate timeout of 2 minutes is generous for a human
//     at a terminal; on a user who stepped away to grab coffee, it
//     auto-denies — fine. For an agent-initiated gate prompt with a
//     slow local model behind it, 2 minutes is a coin flip.
//   - A 30-second sweeper tick is correct in a multi-plan cluster; on
//     a solo home-lab deployment with one in-flight plan it's wasted
//     DB load.
//
// Every timeout that the orchestrator, sweeper, permission gate, or
// submit handler consults MUST come from this package. Hardcoded
// time.Minute literals in those paths are a BYOM regression.
//
// All values are loaded from environment variables at process start.
// Users override via systemd drop-ins, docker-compose env, or their
// shell. Defaults match a conservative cloud deployment; BYOM guides
// will document how to tune for local inference.
package byom

import (
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
)

// Defaults — production cloud-appropriate. BYOM users override per
// deployment.
const (
	// DefaultSubmitHardTimeout caps how long a single submit_prompt
	// agent turn may run. Raised in BYOM deployments with slow local
	// models; lowered to tighten SLOs on hosted inference.
	DefaultSubmitHardTimeout = 10 * time.Minute

	// DefaultPermissionTimeout is how long a permission.requested event
	// waits for a reply before auto-denying. Tuned for a human at a
	// keyboard; BYOM users automating approval via webhooks may set
	// this lower (or higher if gates feed back into a slow LLM agent).
	DefaultPermissionTimeout = 2 * time.Minute

	// DefaultStalePlanAfter is how long an orchestrator plan can sit in
	// status=running with no activity before the sweeper treats it as
	// abandoned. MUST exceed the slowest expected single-node runtime
	// plus a safety margin, otherwise the sweeper hijacks healthy
	// plans waiting on legitimately-slow inference.
	DefaultStalePlanAfter = 15 * time.Minute

	// DefaultSweeperTick is how often the sweeper scans for recovery
	// candidates. Lower = faster recovery after a crash; higher =
	// less DB load on idle installs.
	DefaultSweeperTick = 30 * time.Second

	// DefaultGraphMaxHops bounds how many node transitions a single
	// graph.Run call can make before suspecting a cycle. Phase 2
	// hardcoded 256; moved here so cluster-scale plans can raise it
	// without recompiling.
	DefaultGraphMaxHops = 256

	// per-tenant resource quotas. These cap the blast
	// radius a single tenant can impose on the gateway process so
	// one greedy tenant cannot starve others.
	//
	// DefaultTenantMaxConcurrent is how many plan executions +
	// wasm-plugin mutating requests a single tenant can have
	// in-flight simultaneously. Cheap to raise; dropping below 1
	// disables the limiter. Default 4 is generous for interactive
	// use; batch-heavy tenants should raise via env.
	DefaultTenantMaxConcurrent = 4

	// DefaultTenantRateLimitPerSecond caps sustained request rate
	// per tenant across /v1/commands + /v1/wasm-plugins. Token
	// bucket with equal burst. 0 disables. Default 30/sec matches
	// the existing IP-level NewIPRateLimit(10,20) shape — slightly
	// higher because tenants typically front many human users.
	DefaultTenantRateLimitPerSecond = 30
)

// Env var names — kept as constants so tests and docs reference one
// source.
const (
	EnvSubmitHardTimeout         = "QORVEN_SUBMIT_TIMEOUT"
	EnvPermissionTimeout         = "QORVEN_PERMISSION_TIMEOUT"
	EnvStalePlanAfter            = "QORVEN_STALE_PLAN_AFTER"
	EnvSweeperTick               = "QORVEN_SWEEPER_TICK"
	EnvGraphMaxHops              = "QORVEN_GRAPH_MAX_HOPS"
	EnvTenantMaxConcurrent       = "QORVEN_TENANT_MAX_CONCURRENT"
	EnvTenantRateLimitPerSecond  = "QORVEN_TENANT_RATE_LIMIT"

	// EnvCacheBackend selects the shared cache implementation.
	// "memory" (default) — in-process; safe for single-replica deployments.
	// "redis"            — Redis-backed; required when QORVEN_REPLICAS > 1.
	// REDIS_URL must be set when using the redis backend.
	EnvCacheBackend = "QORVEN_CACHE_BACKEND"
)

// Timeouts is the resolved set of BYOM-tunable operational defaults.
// Construct once via Load() at process start; handlers consult fields
// by value. Not meant for hot-reload — changing a timeout mid-flight
// while a submit is in-flight is asking for trouble. Operators
// restart to apply.
type Timeouts struct {
	SubmitHardTimeout         time.Duration
	PermissionTimeout         time.Duration
	StalePlanAfter            time.Duration
	SweeperTick               time.Duration
	GraphMaxHops              int
	TenantMaxConcurrent       int
	TenantRateLimitPerSecond  int
	CacheBackend              string // "memory" or "redis"
}

// loadedMu + loaded guard a lazy single load. Tests can override the
// resolved set via SetForTests.
var (
	loadedMu sync.Mutex
	loaded   *Timeouts
)

// Load reads env vars and returns the resolved Timeouts. Subsequent
// calls return the same cached value. Thread-safe.
func Load() Timeouts {
	loadedMu.Lock()
	defer loadedMu.Unlock()
	if loaded != nil {
		return *loaded
	}
	t := Timeouts{
		SubmitHardTimeout:        durationEnv(EnvSubmitHardTimeout, DefaultSubmitHardTimeout),
		PermissionTimeout:        durationEnv(EnvPermissionTimeout, DefaultPermissionTimeout),
		StalePlanAfter:           durationEnv(EnvStalePlanAfter, DefaultStalePlanAfter),
		SweeperTick:              durationEnv(EnvSweeperTick, DefaultSweeperTick),
		GraphMaxHops:             intEnv(EnvGraphMaxHops, DefaultGraphMaxHops),
		TenantMaxConcurrent:      intEnvAllowZero(EnvTenantMaxConcurrent, DefaultTenantMaxConcurrent),
		TenantRateLimitPerSecond: intEnvAllowZero(EnvTenantRateLimitPerSecond, DefaultTenantRateLimitPerSecond),
		CacheBackend:             stringEnv(EnvCacheBackend, "memory"),
	}
	loaded = &t
	return t
}

// SetForTests overrides the cached Timeouts and returns a restore
// function the caller MUST defer. Name documents the contract — only
// tests should call this.
func SetForTests(t Timeouts) func() {
	loadedMu.Lock()
	prev := loaded
	loaded = &t
	loadedMu.Unlock()
	return func() {
		loadedMu.Lock()
		loaded = prev
		loadedMu.Unlock()
	}
}

// ResetForTests drops the cached Timeouts so the next Load() re-reads
// env vars. Paired with t.Setenv in tests that exercise env parsing.
func ResetForTests() {
	loadedMu.Lock()
	loaded = nil
	loadedMu.Unlock()
}

// durationEnv parses a duration env var with a default.
// Invalid values fall back to the default and log a warning —
// operators see the mistake in the startup log rather than running
// with a silently-wrong value.
func durationEnv(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		slog.Warn("byom: invalid duration env var; using default",
			"env", name, "value", raw, "default", def, "err", err)
		return def
	}
	if v <= 0 {
		slog.Warn("byom: non-positive duration rejected; using default",
			"env", name, "value", v, "default", def)
		return def
	}
	return v
}

// intEnvAllowZero is like intEnv but accepts 0 as a valid value.
// Used for quotas where 0 means "feature disabled" — rate limit 0
// turns the limiter into a no-op, concurrency 0 lets every request
// through uncapped. Negative is still rejected.
func intEnvAllowZero(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		slog.Warn("byom: invalid int env var; using default",
			"env", name, "value", raw, "default", def, "err", err)
		return def
	}
	return v
}

func intEnv(name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		slog.Warn("byom: invalid int env var; using default",
			"env", name, "value", raw, "default", def, "err", err)
		return def
	}
	return v
}

// stringEnv returns the env var value, or def when unset or empty.
func stringEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
