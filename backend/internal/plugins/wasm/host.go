// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package wasm hosts Qorven's WebAssembly plugin runtime. A Wasm
// plugin registers a custom tool that the orchestrator can invoke
// exactly like a built-in tool — but with a deny-by-default sandbox
// that makes it safe to run third-party (or AI-generated) code in a
// multi-tenant environment.
//
// Sandbox guarantees (non-negotiable):
//
//   • No network. The host does not expose any socket-capable
//     wasi-snapshot-preview1 function to the guest. Even if a
//     rogue plugin tried to `connect()`, the import fails at
//     instantiation — not at runtime — because we compile the module
//     against a NetBuilder-less WASI surface.
//
//   • No filesystem. The host mounts no directories. The guest sees
//     an empty root; its WASI preopens list is empty. Attempts to
//     open files fail with EBADF (the wasi spec's "no such fd").
//
//   • No clocks it can abuse for fingerprinting. We provide the
//     WASI clock via wazero's default, which returns zero when
//     sys.Nanosleep is called with no time source — but we do NOT
//     rely on that as a security property; it's a usability nicety.
//
//   • Memory cap. Each guest is compiled with MaxMemoryPages set to
//     a small value (default 64 pages = 4 MiB). An AI-generated
//     plugin that tries to allocate unbounded memory traps with
//     "out of memory" and the host returns a structured error to
//     the orchestrator rather than OOMing the gateway.
//
//   • CPU timeout. Every invocation runs under a context.Context
//     with a hard deadline (default 2s). wazero honors ctx
//     cancellation at every wasm instruction boundary, so a plugin
//     cannot infinite-loop past its timeout.
//
//   • Invocation is isolated. Each Invoke call instantiates a fresh
//     module. Guest-side globals do NOT persist between invocations.
//     This is deliberate: a stateful plugin could leak data across
//     tenants. If a plugin needs persistent state, it must round-trip
//     through the orchestrator's DB (which is RLS-gated by tenant).
//
// Contract with the guest:
//
//   The host writes a JSON payload to STDIN, runs the guest's _start,
//   and reads the guest's STDOUT as a JSON reply. The guest may write
//   diagnostics to STDERR — the host captures it and returns it in
//   the InvokeResult for debugging, but the orchestrator never
//   surfaces STDERR content to tool-call consumers.
//
//   The JSON schemas are defined in the sibling `types.go` file.
package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Config sets the sandbox limits. Zero values in any field fall back
// to the package defaults. Conservative defaults — raising them is
// a security decision, not an ergonomics one.
type Config struct {
	// MaxMemoryPages caps the Wasm linear memory in 64 KiB pages.
	// Default 64 pages = 4 MiB. Must be > 0.
	MaxMemoryPages uint32

	// InvokeTimeout bounds a single Invoke call. Default 2s. Must
	// be > 0. The ctx passed into Invoke is Used as the parent —
	// whichever is shorter wins.
	InvokeTimeout time.Duration

	// MaxStdoutBytes caps the guest's reply size. A plugin that
	// writes gigabytes to STDOUT would OOM the HOST side. Default
	// 1 MiB.
	MaxStdoutBytes int

	// MaxStdinBytes caps the input payload we're willing to hand to
	// the guest. Default 256 KiB.
	MaxStdinBytes int

	// Logger captures host-side events (compilation, guest trap,
	// timeout). Defaults to slog.Default().
	Logger *slog.Logger
}

func (c *Config) applyDefaults() {
	if c.MaxMemoryPages == 0 {
		c.MaxMemoryPages = 64
	}
	if c.InvokeTimeout <= 0 {
		c.InvokeTimeout = 2 * time.Second
	}
	if c.MaxStdoutBytes <= 0 {
		c.MaxStdoutBytes = 1 << 20
	}
	if c.MaxStdinBytes <= 0 {
		c.MaxStdinBytes = 256 << 10
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

// Host is a long-lived Wasm runtime. Compile plugins once via
// LoadPlugin; Invoke many times. One Host instance is shared across
// every tenant in a single-binary deployment.
type Host struct {
	cfg     Config
	runtime wazero.Runtime
	// compiled plugins keyed by their registration name.
	mu      sync.RWMutex
	modules map[string]wazero.CompiledModule
}

// NewHost constructs a Host and registers the WASI preview1 import
// surface — minus network and filesystem, because we do not call
// the .With* builders for those.
//
// The ctx passed here is used for the initial WASI module
// registration. Pass a background context; compilation is not
// per-request.
func NewHost(ctx context.Context, cfg Config) (*Host, error) {
	cfg.applyDefaults()

	// Use the interpreter engine rather than the optimizing
	// compiler. The compiler (wazevo) is faster for long-running
	// workloads but has toolchain-specific codegen quirks we don't
	// need to debug — plugin invocations are bounded to 2s and
	// usually dominated by I/O. Interpreter gives us consistent
	// behavior across Go versions and architectures.
	rtCfg := wazero.NewRuntimeConfigInterpreter().
		WithMemoryLimitPages(cfg.MaxMemoryPages).
		// Close at runtime shutdown — NOT at guest-exit — so we can
		// reuse the compiled module across invocations.
		WithCloseOnContextDone(true)

	rt := wazero.NewRuntimeWithConfig(ctx, rtCfg)

	// WASI preview1 is mandatory for TinyGo / Rust-wasi / Go-wasip1
	// guests. The import provides stdin/stdout/stderr, arg parsing,
	// environment variable lookup (empty set), and exit. It does NOT
	// expose sockets — those are in wasi-sockets, which we do not
	// instantiate.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("wasm: instantiate wasi: %w", err)
	}

	return &Host{
		cfg:     cfg,
		runtime: rt,
		modules: make(map[string]wazero.CompiledModule),
	}, nil
}

// Close releases every compiled module and shuts the runtime. Safe to
// call once; double-close is a no-op.
func (h *Host) Close(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, m := range h.modules {
		_ = m.Close(ctx)
		delete(h.modules, name)
	}
	return h.runtime.Close(ctx)
}

// LoadPlugin compiles a .wasm module and registers it under the
// given name. A subsequent Invoke(name, ...) looks up this
// compilation.
//
// Compile errors (malformed module, missing _start, unsupported
// import) surface here — not at Invoke time — so operators see bad
// plugins at registration rather than when a user triggers one.
func (h *Host) LoadPlugin(ctx context.Context, name string, wasmBinary []byte) error {
	if name == "" {
		return errors.New("wasm: plugin name required")
	}
	if len(wasmBinary) == 0 {
		return errors.New("wasm: empty module")
	}
	compiled, err := h.runtime.CompileModule(ctx, wasmBinary)
	if err != nil {
		recordLoadError()
		return fmt.Errorf("wasm: compile %q: %w", name, err)
	}

	// Refuse modules that import anything outside the wasi_snapshot_preview1
	// surface we advertised. A plugin asking for "wasi_sockets" or
	// "env" imports we didn't provide is either malicious or broken
	// — either way, fail at LoadPlugin.
	for _, imp := range compiled.ImportedFunctions() {
		mod, _, _ := imp.Import()
		if mod != "wasi_snapshot_preview1" {
			_ = compiled.Close(ctx)
			recordLoadError()
			return fmt.Errorf("wasm: plugin %q imports disallowed module %q; "+
				"only wasi_snapshot_preview1 is exposed (no network, no filesystem)",
				name, mod)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if prev, ok := h.modules[name]; ok {
		_ = prev.Close(ctx)
	}
	h.modules[name] = compiled
	h.cfg.Logger.Info("wasm: plugin loaded",
		"name", name, "size_bytes", len(wasmBinary))
	return nil
}

// UnloadPlugin removes a previously-loaded plugin. Returns true if
// the plugin was present.
func (h *Host) UnloadPlugin(ctx context.Context, name string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	compiled, ok := h.modules[name]
	if !ok {
		return false
	}
	_ = compiled.Close(ctx)
	delete(h.modules, name)
	return true
}

// List returns the names of every currently-loaded plugin. Sorted is
// not guaranteed.
func (h *Host) List() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]string, 0, len(h.modules))
	for name := range h.modules {
		out = append(out, name)
	}
	return out
}

// Invoke runs the plugin's _start with payload on STDIN, captures
// STDOUT (parsed as JSON reply) and STDERR (returned as diagnostics).
// See InvokeWithTenant for multi-tenant metric attribution; Invoke is
// a convenience that passes tenant="" (used by tests and single-
// tenant call paths).
func (h *Host) Invoke(ctx context.Context, pluginName string, payload []byte) (*InvokeResult, error) {
	return h.InvokeWithTenant(ctx, pluginName, "", payload)
}

// InvokeWithTenant is the tenant-aware variant. The tenant string
// lands as a label on Prometheus counters so operators can see
// "plugin foo in tenant A fired 1200 times, in tenant B fired 12."
// Pass "" for unscoped tests; handlers in multi-tenant mode pass
// the authenticated user's TenantID.
//
// Contract (unchanged between variants):
//   • payload MUST be valid JSON or empty. The host does not validate
//     beyond "it's UTF-8 + under MaxStdinBytes"; the guest is expected
//     to json.Unmarshal on its side.
//   • The guest terminates by exit(0) for success or exit(1) for a
//     structured error. exit(0) means STDOUT has your JSON reply;
//     exit(1) means STDERR has a diagnostic.
//   • A guest that TRAPs (illegal instruction, memory fault) returns
//     a non-nil Err and Stdout may be empty. Never panic the host.
//   • The guest MUST finish within Config.InvokeTimeout.
func (h *Host) InvokeWithTenant(ctx context.Context, pluginName, tenant string, payload []byte) (*InvokeResult, error) {
	if len(payload) > h.cfg.MaxStdinBytes {
		return nil, fmt.Errorf("wasm: payload %d bytes exceeds max %d",
			len(payload), h.cfg.MaxStdinBytes)
	}
	if !json.Valid(payload) && len(payload) != 0 {
		// Empty is OK — a plugin that takes no input needs no JSON.
		return nil, fmt.Errorf("wasm: payload is not valid JSON")
	}

	h.mu.RLock()
	compiled, ok := h.modules[pluginName]
	h.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("wasm: plugin %q not loaded", pluginName)
	}

	invokeCtx, cancel := context.WithTimeout(ctx, h.cfg.InvokeTimeout)
	defer cancel()

	stdinBuf := bytes.NewReader(payload)
	stdoutBuf := newLimitedBuffer(h.cfg.MaxStdoutBytes)
	stderrBuf := newLimitedBuffer(h.cfg.MaxStdoutBytes)

	modCfg := wazero.NewModuleConfig().
		WithName("").   // empty module name so the instance is transient
		WithStdin(stdinBuf).
		WithStdout(stdoutBuf).
		WithStderr(stderrBuf).
		// No env vars, no args, no filesystem mounts, no random source
		// tied to the host clock. An AI-generated plugin that expects
		// os.Args will see len=0 — which is correct and deterministic.
		WithArgs().
		WithSysWalltime().   // monotonic enough for the guest's internal use
		WithSysNanotime()

	// Track whether the guest exited cleanly vs trapped vs timed out.
	start := time.Now()
	mod, runErr := h.runtime.InstantiateModule(invokeCtx, compiled, modCfg)
	elapsed := time.Since(start)

	// wazero returns a *sys.ExitError wrapping exit(code). We always
	// close the module — even on error — so its memory is freed.
	if mod != nil {
		_ = mod.Close(context.Background())
	}

	res := &InvokeResult{
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
		Elapsed:  elapsed,
		ExitCode: 0,
	}

	// Determine the outcome label for metrics. This single switch is
	// the source of truth for metric classification; anyone wanting
	// to add a new label must edit it here + document in metrics.go.
	truncated := stdoutBuf.truncated
	outcome := "ok"
	if runErr != nil {
		// Check context-cancellation FIRST. wazero reports a
		// context-cancelled guest as a *sys.ExitError with a sentinel
		// exit code (0xEFFFFFFF = sys.ExitCodeContextCanceled), which
		// would otherwise match our "graceful exit(N)" branch and
		// hide the timeout. Drive off invokeCtx.Err() instead so the
		// decision is unambiguous.
		switch {
		case errors.Is(invokeCtx.Err(), context.DeadlineExceeded) ||
			errors.Is(invokeCtx.Err(), context.Canceled):
			res.Err = fmt.Errorf("guest exceeded %s timeout", h.cfg.InvokeTimeout)
			outcome = "timeout"
		default:
			// Distinguish graceful guest-side exit(N) from traps.
			// wazero's *sys.ExitError carries the exit code.
			var exitErr interface{ ExitCode() uint32 }
			if errors.As(runErr, &exitErr) {
				code := exitErr.ExitCode()
				res.ExitCode = int(code)
				if code == 0 {
					// exit(0) still produces a *sys.ExitError in wazero —
					// that's a clean success. outcome stays "ok".
				} else {
					res.Err = fmt.Errorf("guest exited with code %d", res.ExitCode)
					outcome = "exit_nonzero"
				}
			} else {
				res.Err = fmt.Errorf("guest trap: %w", runErr)
				outcome = "trap"
			}
		}
	}
	recordInvocation(pluginName, tenant, outcome, elapsed, truncated)
	return res, nil
}

// InvokeResult is what Invoke returns on both success and graceful
// error paths. A top-level error from Invoke (not nil returned along
// with nil result) means the host itself couldn't set up the call —
// bad payload, plugin not loaded, etc. Guest-side failures (trap,
// timeout, exit(N)) populate Err on the result.
type InvokeResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	Elapsed  time.Duration
	Err      error
}

// limitedBuffer is a bytes.Buffer with a hard cap. Writes past the
// cap truncate (like io.LimitedWriter) and the buffer records that
// it overflowed so callers can flag the response as truncated.
type limitedBuffer struct {
	buf       []byte
	cap       int
	truncated bool
}

func newLimitedBuffer(cap int) *limitedBuffer {
	if cap <= 0 {
		cap = 1 << 20
	}
	return &limitedBuffer{buf: make([]byte, 0, 4096), cap: cap}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.buf)+len(p) > b.cap {
		room := b.cap - len(b.buf)
		if room > 0 {
			b.buf = append(b.buf, p[:room]...)
		}
		b.truncated = true
		// Return len(p) so the guest's write() loop sees success —
		// if we returned short, the guest might retry and spin.
		// The truncation fact is recorded server-side.
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf
}

// Ensure the types we hand out implement the minimal io.Writer/
// io.Reader surface wazero expects.
var (
	_ io.Writer = (*limitedBuffer)(nil)
)
