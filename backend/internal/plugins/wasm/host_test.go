// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package wasm_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/plugins/wasm"
)

// loadEchoWasm reads the pre-built testdata/echo_plugin.wasm. If
// missing, the test SKIPs with a clear instruction. The Makefile's
// `make wasm-testdata` target rebuilds it. Agents editing the
// plugin source must re-run that target before committing.
func loadEchoWasm(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/echo_plugin.wasm")
	if err != nil {
		t.Skipf("testdata/echo_plugin.wasm missing; run `make wasm-testdata`: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("testdata/echo_plugin.wasm is empty")
	}
	return data
}

// TestHost_HappyPath is the smoke test: load, invoke, get a JSON
// reply back. If this breaks every other test in the file is also
// broken, so keep it as the FIRST test for fast-fail.
func TestHost_HappyPath(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	payload := []byte(`{"message":"hello from host"}`)
	res, err := host.Invoke(ctx, "echo", payload)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("guest error: %v (exit=%d stderr=%q)",
			res.Err, res.ExitCode, res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d, want 0", res.ExitCode)
	}

	var reply map[string]any
	if err := json.Unmarshal(res.Stdout, &reply); err != nil {
		t.Fatalf("parse reply: %v (stdout=%q)", err, res.Stdout)
	}
	if reply["echoed"] != "hello from host" {
		t.Fatalf("reply[echoed]=%v, want 'hello from host'", reply["echoed"])
	}
	if reply["from_wasm"] != true {
		t.Fatalf("reply[from_wasm]=%v, want true (guest didn't execute)", reply["from_wasm"])
	}
}

// TestHost_Timeout — a plugin that spins forever must be killed at
// InvokeTimeout, not eat a CPU core indefinitely. The security
// boundary depends on this.
func TestHost_Timeout(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{
		InvokeTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	start := time.Now()
	res, err := host.Invoke(ctx, "echo", []byte(`{"spin":true}`))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Err == nil {
		t.Fatalf("expected timeout error; got clean return")
	}
	if !strings.Contains(res.Err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %q", res.Err)
	}
	// Reasonable upper bound: 5× the configured timeout. If we
	// blew past that, the context cancellation didn't land.
	if elapsed > time.Second {
		t.Fatalf("timeout did not cut in: elapsed=%v (configured 200ms)", elapsed)
	}
}

// TestHost_GuestExitsNonZero — the guest explicitly calls exit(1)
// after writing to stderr. The host surfaces this as a structured
// error without trapping.
func TestHost_GuestExitsNonZero(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	res, err := host.Invoke(ctx, "echo", []byte(`{"fail_with":"banana"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Err == nil {
		t.Fatalf("expected guest error; got clean return. stdout=%q", res.Stdout)
	}
	if res.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", res.ExitCode)
	}
	if !strings.Contains(string(res.Stderr), "banana") {
		t.Fatalf("stderr did not carry guest diagnostic: %q", res.Stderr)
	}
}

// TestHost_StdoutTruncation — a plugin that writes more than
// MaxStdoutBytes must be cut off, not allowed to OOM the host.
func TestHost_StdoutTruncation(t *testing.T) {
	ctx := context.Background()
	// Keep the cap below the guest's 128 KiB big_reply so the
	// limitedBuffer trips its truncation path.
	host, err := wasm.NewHost(ctx, wasm.Config{
		MaxStdoutBytes: 16 * 1024, // 16 KiB
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	res, err := host.Invoke(ctx, "echo", []byte(`{"message":"hi","big_reply":true}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// Guest succeeds from its POV — it wrote bytes, got no error.
	// Host caps at 16 KiB; anything beyond is silently dropped.
	if len(res.Stdout) > 16*1024 {
		t.Fatalf("stdout not truncated: got %d bytes, cap 16 KiB", len(res.Stdout))
	}
}

// TestHost_InputTooLarge — payload > MaxStdinBytes MUST be refused
// by the host, never reaching the guest.
func TestHost_InputTooLarge(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{
		MaxStdinBytes: 1024, // 1 KiB
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// Construct a ~10 KiB JSON blob.
	big := map[string]string{"junk": strings.Repeat("x", 10*1024)}
	payload, _ := json.Marshal(big)

	_, err = host.Invoke(ctx, "echo", payload)
	if err == nil {
		t.Fatalf("expected payload-size error")
	}
	if !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("wrong error: %v", err)
	}
}

// TestHost_PluginNotLoaded — Invoke on an unknown name fails cleanly.
func TestHost_PluginNotLoaded(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	_, err = host.Invoke(ctx, "nonexistent", []byte(`{}`))
	if err == nil {
		t.Fatalf("expected not-loaded error")
	}
	if !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("wrong error: %v", err)
	}
}

// TestHost_NoNetworkAccess is the security tripwire. A guest that
// tries to import a non-WASI module (e.g. a wasi-sockets draft) must
// fail at LoadPlugin — the host advertises ONLY wasi_snapshot_preview1.
//
// We can't easily synthesize a Wasm module importing a forbidden
// module from Go source (the Go wasip1 target only imports
// wasi_snapshot_preview1). Instead we assert the negative
// structural property: the echo plugin's imports are ALL within
// wasi_snapshot_preview1. A future refactor that loosens host
// capabilities would require editing this test too.
//
// Re-checking via the runtime: we read the compiled module's
// ImportedFunctions list.
func TestHost_OnlyWASIImportsAllowed(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	// Inject a malformed plugin that declares an import from a
	// module name we don't provide. Use a minimal hand-assembled
	// Wasm binary that imports (env "net_connect") — this is the
	// shape of a sockets-capable or OS-escape module.
	//
	// Wasm binary breakdown:
	//   \x00asm  — magic
	//   \x01\x00\x00\x00 — version
	//   section 1 (type):  one type, () -> ()
	//   section 2 (import): "env"."net_connect" of type 0
	rogue := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // magic+version
		// Type section: id=1, size=4, 1 type, func (no params, no results)
		0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
		// Import section: id=2, body length = 1(count)+1(modlen)+3(mod)+
		// 1(fieldlen)+11(field)+1(kind)+1(typeidx) = 19 bytes.
		0x02, 0x13, 0x01,
		0x03, 'e', 'n', 'v',
		0x0b, 'n', 'e', 't', '_', 'c', 'o', 'n', 'n', 'e', 'c', 't',
		0x00, 0x00,
	}

	err = host.LoadPlugin(ctx, "rogue", rogue)
	if err == nil {
		t.Fatalf("LoadPlugin accepted a module with a forbidden import — sandbox is BROKEN")
	}
	if !strings.Contains(err.Error(), "disallowed") {
		t.Fatalf("error does not mention disallowed import: %v", err)
	}
}

// TestHost_EmptyPayloadIsHostAccepted — the HOST does not reject
// empty payloads at the json.Valid gate; an empty stdin is a
// legitimate input for plugins that take no arguments. Whether the
// guest accepts it is guest-specific (our echo sample rejects because
// json.Unmarshal("") errors — that's a sample-plugin choice, not a
// host contract). The invariant we care about: Invoke itself does
// not error on nil payload.
func TestHost_EmptyPayloadIsHostAccepted(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// nil payload => host must NOT return a top-level error.
	// (The guest will exit(1) because json.Unmarshal("") fails; that's
	// expected guest behavior, not a host bug.)
	_, err = host.Invoke(ctx, "echo", nil)
	if err != nil {
		t.Fatalf("Invoke(empty): host returned top-level error, "+
			"but empty payload is allowed by the host contract: %v", err)
	}
}

// TestBridgeTool — end-to-end via the tools.Tool adapter. Proves a
// Wasm plugin plugs straight into the existing tool runner shape
// without any Wasm-specific handling at the call site.
func TestBridgeTool(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	if err := host.LoadPlugin(ctx, "echo", loadEchoWasm(t)); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	bridge := wasm.NewBridgeTool(host, "echo", wasm.ToolDescriptor{
		Name:        "wasm_echo",
		Description: "echoes the message field",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]string{"type": "string"},
			},
		},
	})
	if bridge.Name() != "wasm_echo" {
		t.Fatalf("bridge.Name()=%q", bridge.Name())
	}

	result := bridge.Execute(ctx, map[string]any{"message": "bridge"})
	if result.IsError {
		t.Fatalf("bridge Execute errored: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, `"echoed":"bridge"`) {
		t.Fatalf("bridge result did not carry echo: %s", result.ForLLM)
	}
}

// TestHost_ReloadReplacesPlugin — loading a plugin under an existing
// name closes the old compilation and replaces it. Prevents zombie
// modules from piling up in long-running gateways.
func TestHost_ReloadReplacesPlugin(t *testing.T) {
	ctx := context.Background()
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	bin := loadEchoWasm(t)
	if err := host.LoadPlugin(ctx, "dup", bin); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if err := host.LoadPlugin(ctx, "dup", bin); err != nil {
		t.Fatalf("reload: %v", err)
	}
	// Only one entry in List.
	found := 0
	for _, n := range host.List() {
		if n == "dup" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("dup appears %d times in List, want 1", found)
	}
}
