// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package wasm

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMetrics_RecordsOutcomeLabels verifies that each branch of the
// Invoke state machine bumps the right `outcome` label. If the
// classifier in host.go drifts, this test catches it — operators
// rely on these labels to draw dashboards.
func TestMetrics_RecordsOutcomeLabels(t *testing.T) {
	resetMetricsForTests()

	ctx := context.Background()
	host, err := NewHost(ctx, Config{
		// 2s invoke timeout for the happy path (same as the default
		// — the echo plugin spends most of its time in Go runtime
		// startup under the Wasm interpreter, ~600ms each). Short
		// enough that the `spin` test still trips it within the test's
		// 120s budget.
		InvokeTimeout: 2 * time.Second,
		// Small stdout cap so the `big_reply` plugin input truncates.
		MaxStdoutBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	data, err := os.ReadFile("testdata/echo_plugin.wasm")
	if err != nil {
		t.Skipf("testdata/echo_plugin.wasm missing: %v", err)
	}
	if err := host.LoadPlugin(ctx, "echo", data); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	// 1. ok: trivial happy path.
	if _, err := host.InvokeWithTenant(ctx, "echo", "t-A", []byte(`{"message":"hi"}`)); err != nil {
		t.Fatalf("ok invoke: %v", err)
	}

	// 2. exit_nonzero: plugin exit(1) after writing to stderr.
	if _, err := host.InvokeWithTenant(ctx, "echo", "t-A", []byte(`{"fail_with":"x"}`)); err != nil {
		t.Fatalf("exit invoke: %v", err)
	}

	// 3. timeout: spinning plugin.
	if _, err := host.InvokeWithTenant(ctx, "echo", "t-A", []byte(`{"spin":true}`)); err != nil {
		t.Fatalf("timeout invoke: %v", err)
	}

	// 4. ok + truncated_stdout: big_reply overflows 32 KiB cap. The
	// primary outcome stays "ok" because the guest exits cleanly;
	// truncated_stdout is a SECOND counter bump, not a replacement.
	if _, err := host.InvokeWithTenant(ctx, "echo", "t-A",
		[]byte(`{"message":"x","big_reply":true}`)); err != nil {
		t.Fatalf("truncate invoke: %v", err)
	}

	// Force a load error by passing an invalid module — exercises
	// the LoadPlugin → recordLoadError path.
	if err := host.LoadPlugin(ctx, "garbage", []byte{0x00, 0x00}); err == nil {
		t.Fatalf("expected LoadPlugin to reject malformed bytes")
	}

	var buf bytes.Buffer
	WriteMetrics(&buf)
	out := buf.String()

	// Spot-check every label/outcome combination we care about.
	expect := []string{
		`plugins_wasm_invocations_total{plugin="echo",tenant="t-A",outcome="ok"} 2`,
		`plugins_wasm_invocations_total{plugin="echo",tenant="t-A",outcome="exit_nonzero"} 1`,
		`plugins_wasm_invocations_total{plugin="echo",tenant="t-A",outcome="timeout"} 1`,
		`plugins_wasm_invocations_total{plugin="echo",tenant="t-A",outcome="truncated_stdout"} 1`,
		`plugins_wasm_load_errors_total 1`,
		`plugins_wasm_duration_ms_count{plugin="echo",tenant="t-A"} 4`,
	}
	for _, e := range expect {
		if !strings.Contains(out, e) {
			t.Errorf("metrics output missing %q. full output:\n%s", e, out)
		}
	}
}

// TestMetrics_SeparateTenantsSeparateCounters confirms the tenant
// label is actually distinguishing. A regression that let all
// invocations collide under tenant="" would silently merge dashboards.
func TestMetrics_SeparateTenantsSeparateCounters(t *testing.T) {
	resetMetricsForTests()
	ctx := context.Background()
	host, err := NewHost(ctx, Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	data, err := os.ReadFile("testdata/echo_plugin.wasm")
	if err != nil {
		t.Skipf("testdata/echo_plugin.wasm missing: %v", err)
	}
	if err := host.LoadPlugin(ctx, "echo", data); err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	for _, tenant := range []string{"tA", "tA", "tB"} {
		if _, err := host.InvokeWithTenant(ctx, "echo", tenant, []byte(`{"message":"x"}`)); err != nil {
			t.Fatalf("%s: %v", tenant, err)
		}
	}

	var buf bytes.Buffer
	WriteMetrics(&buf)
	out := buf.String()
	if !strings.Contains(out, `tenant="tA",outcome="ok"} 2`) {
		t.Errorf("expected tA=2: %s", out)
	}
	if !strings.Contains(out, `tenant="tB",outcome="ok"} 1`) {
		t.Errorf("expected tB=1: %s", out)
	}
}
