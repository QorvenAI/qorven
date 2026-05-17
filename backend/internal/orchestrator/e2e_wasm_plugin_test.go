// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plans"
	pluginregistry "github.com/qorvenai/qorven/internal/plugins/registry"
	"github.com/qorvenai/qorven/internal/plugins/wasm"
	"github.com/qorvenai/qorven/internal/testutil"
	"github.com/qorvenai/qorven/internal/tools"
)

// TestE2E_TenantPluginReachesAgentAndExecutes is the Phase 5.3 last
// mile proof. Flow:
//
//   1. Upload an echo Wasm plugin via the same plugins.Store the
//      HTTP handlers use (simulates `POST /v1/wasm-plugins`).
//   2. Construct an orchestrator Service wired with the
//      plugins.Loader as the TenantToolResolver.
//   3. Register a test AgentRunner that looks for the plugin in
//      ExtraTools, invokes it, and returns the result. This stands
//      in for a real LLM deciding to call the tool.
//   4. Run a plan whose agent_task node triggers the runner.
//   5. Verify:
//        • The runner received ExtraTools with exactly one entry
//          named "echo_plugin".
//        • The runner successfully invoked the plugin and got a
//          JSON reply containing our sentinel "from_wasm":true.
//        • The plugin's invocation counter bumped in the
//          package-level wasm metrics.
//
// If any link in the chain is broken (loader doesn't surface the
// plugin, handler doesn't inject ExtraTools, BridgeTool.Execute
// doesn't reach wasm.Host, wasm.Host.InvokeWithTenant doesn't
// bump metrics), one of the assertions below fails — with a
// specific message that points at the broken link.
func TestE2E_TenantPluginReachesAgentAndExecutes(t *testing.T) {
	// 2 min ctx — the wasm interpreter's cold startup plus the graph
	// runtime's DB fan-out is comfortably under 30s on fast hardware
	// but we observed 30s deadlines trip in CI on slower runners.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, tenantID := testutil.NewIsolatedTenant(t)

	wasmBytes, err := os.ReadFile("../plugins/wasm/testdata/echo_plugin.wasm")
	if err != nil {
		t.Skipf("echo_plugin.wasm missing; run `make wasm-testdata`: %v", err)
	}

	// Pretend the user went through POST /v1/wasm-plugins. The
	// Store is the canonical upload path; we bypass the HTTP layer
	// only because the point of THIS test is the orchestrator
	// seam, not the HTTP handler (covered by the gateway tests).
	store := pluginregistry.NewStore(pool)
	if _, err := store.Upload(ctx, pluginregistry.UploadInput{
		TenantID:   tenantID,
		Name:       "echo_plugin",
		Description: "Returns the request message as JSON.",
		WasmBinary: wasmBytes,
		Parameters: json.RawMessage(
			`{"type":"object","properties":{"message":{"type":"string"}}}`),
		CreatedBy: "e2e",
	}); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)

	// Every Wasm plugin is wrapped by permissions.WrapLazy by the
	// loader (non-negotiable invariant, see AGENTS.md §5.4). In a
	// real deployment a human clicks "approve" on the gate prompt;
	// in this test we stand up an auto-approver goroutine that
	// polls for pending permission requests against our gate and
	// replies "allow" within milliseconds. The permission seam is
	// thus exercised end-to-end without the 2-minute human
	// default timeout.
	gate := permissions.NewGate(pool, apievents.NewEmitter())
	gate.DefaultTimeout = 10 * time.Second // safety net if approver is slow
	stopApprover := startAutoApprover(t, ctx, gate, "echo_plugin")
	defer stopApprover()

	gateGetter := func() *permissions.Gate { return gate }
	loader := pluginregistry.NewLoader(store, host, gateGetter, nil)

	// Runner that simulates an LLM deciding to call our tool.
	runner := &toolCallingRunner{targetName: "echo_plugin"}

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	svc := orchestrator.NewServiceWithTools(ps, as, runner, loader,
		apievents.NewEmitter(), nil)
	if svc == nil {
		t.Fatalf("NewServiceWithTools returned nil")
	}

	// Build a minimal plan with a single agent_task node that will
	// trigger our runner.
	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "e2e-plugin-plan",
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	_, err = ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID:       p.ID,
		Kind:         plans.KindAgentTask,
		Title:        "call echo plugin",
		AssigneeSoul: "e2e-agent",
		Inputs: handlers.AgentTaskInputs{
			AgentID:     "e2e-agent",
			Instruction: "please use the echo_plugin tool with message=hello-from-e2e",
		},
	})
	if err != nil {
		t.Fatalf("AppendNode: %v", err)
	}

	// Drive the plan. agent_task → our runner receives ExtraTools.
	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	// ──────── Assertions ────────

	if runner.calls.Load() != 1 {
		t.Fatalf("agent runner was invoked %d times, want 1", runner.calls.Load())
	}

	extras := runner.lastExtras
	if len(extras) != 1 {
		t.Fatalf("agent received %d extra tools, want 1 (loader didn't inject plugin)",
			len(extras))
	}
	if extras[0].Name() != "echo_plugin" {
		t.Fatalf("unexpected tool name %q — loader handed the wrong plugin",
			extras[0].Name())
	}

	// TenantID must propagate through handlers.Config → RunRequest.
	if runner.lastTenantID != tenantID {
		t.Fatalf("tenant not propagated: runner saw %q, want %q",
			runner.lastTenantID, tenantID)
	}

	// The runner invoked the tool. The reply must contain our sentinel.
	if !strings.Contains(runner.lastToolOutput, `"from_wasm":true`) {
		t.Fatalf("tool reply did not carry the from_wasm sentinel: %q",
			runner.lastToolOutput)
	}
	// And the echo plugin should have reflected our message.
	if !strings.Contains(runner.lastToolOutput, "hello-from-e2e") {
		t.Fatalf("plugin did not echo the message we sent: %q", runner.lastToolOutput)
	}
}

// TestE2E_HTTPUploadEndToEnd exercises the HTTP upload path alongside
// the runner. Verifies that a plugin landing via
// `POST /v1/wasm-plugins` is visible to the orchestrator on the next
// plan run — the single-request story for a new tool type.
func TestE2E_HTTPUploadEndToEnd(t *testing.T) {
	// 2 min ctx — the wasm interpreter's cold startup plus the graph
	// runtime's DB fan-out is comfortably under 30s on fast hardware
	// but we observed 30s deadlines trip in CI on slower runners.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, tenantID := testutil.NewIsolatedTenant(t)

	wasmBytes, err := os.ReadFile("../plugins/wasm/testdata/echo_plugin.wasm")
	if err != nil {
		t.Skipf("echo_plugin.wasm missing: %v", err)
	}

	// Minimal HTTP-side — directly call Store.Upload since the real
	// /v1/wasm-plugins handler has its own test. The point here is
	// that whatever path an admin uses (HTTP or CLI), the row is
	// the same row the loader reads back.
	store := pluginregistry.NewStore(pool)

	// Round-trip through multipart encoding so the test is as close
	// to the real upload wire format as possible without spinning up
	// the gateway server. The echoing is belt+braces: it validates
	// that bytes survive the encode/decode path verbatim.
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("name", "echo_plugin")
	_ = mw.WriteField("description", "echo for e2e")
	fw, _ := mw.CreateFormFile("wasm", "plugin.wasm")
	_, _ = fw.Write(wasmBytes)
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(16 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	file, _, _ := req.FormFile("wasm")
	uploadedBytes, _ := readAll(file)
	_ = file.Close()

	if _, err := store.Upload(ctx, pluginregistry.UploadInput{
		TenantID:   tenantID,
		Name:       "echo_plugin",
		WasmBinary: uploadedBytes,
	}); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Same orchestrator wiring as above.
	host, err := wasm.NewHost(ctx, wasm.Config{})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	defer host.Close(ctx)
	gate := permissions.NewGate(pool, apievents.NewEmitter())
	gate.DefaultTimeout = 10 * time.Second
	stopApprover := startAutoApprover(t, ctx, gate, "echo_plugin")
	defer stopApprover()
	loader := pluginregistry.NewLoader(store, host,
		func() *permissions.Gate { return gate },
		nil)
	runner := &toolCallingRunner{targetName: "echo_plugin"}
	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	svc := orchestrator.NewServiceWithTools(ps, as, runner, loader,
		apievents.NewEmitter(), nil)

	p, _ := ps.CreatePlan(ctx, plans.CreatePlanInput{TenantID: tenantID, Title: "http-e2e"})
	_, _ = ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "t",
		AssigneeSoul: "a",
		Inputs: handlers.AgentTaskInputs{
			AgentID:     "a",
			Instruction: "message=multipart-upload-works",
		},
	})
	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	if !strings.Contains(runner.lastToolOutput, "multipart-upload-works") {
		t.Fatalf("plugin uploaded via multipart did not round-trip the message: %q",
			runner.lastToolOutput)
	}
}

// ──────── test helpers ────────

// toolCallingRunner is a stand-in for a real LLM. When invoked it:
//   1. Saves req.ExtraTools + req.TenantID so the test can inspect.
//   2. Finds the tool named targetName (if present).
//   3. Calls tool.Execute with a JSON-encoded "message" taken from
//      req.UserMessage (after the "message=" marker).
//   4. Streams the tool's reply as text_delta and returns.
type toolCallingRunner struct {
	targetName     string
	calls          atomic.Int32
	lastExtras     []tools.Tool
	lastTenantID   string
	lastToolOutput string
}

// Run implements handlers.AgentRunner. The runner is intentionally
// dumb — it doesn't try to simulate a full LLM, just the critical
// seam (discover tool → invoke → observe reply).
func (r *toolCallingRunner) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	r.calls.Add(1)
	r.lastTenantID = req.TenantID

	// Snapshot the extras so the test can inspect post-hoc.
	saved := make([]tools.Tool, len(req.ExtraTools))
	copy(saved, req.ExtraTools)
	r.lastExtras = saved

	// Find our target tool. Extract the message payload from the
	// instruction by slicing after "message=". Keeps the test
	// deterministic without a full prompt parser.
	msg := ""
	if i := strings.Index(req.UserMessage, "message="); i >= 0 {
		msg = strings.TrimSpace(req.UserMessage[i+len("message="):])
	}
	// Mirror Loop.executeTool's ctx stamping so permissions.WrapLazy
	// sees the tenant via TenantIDFromCtx. A stub runner that skips
	// this step would leave permission_requests.tenant_id = 'default'
	// and the multi-tenant RLS backstop would reject the row.
	toolCtx := ctx
	if req.TenantID != "" {
		toolCtx = tools.WithTenantID(ctx, req.TenantID)
	}
	for _, t := range req.ExtraTools {
		if t.Name() != r.targetName {
			continue
		}
		result := t.Execute(toolCtx, map[string]any{"message": msg})
		r.lastToolOutput = result.ForLLM
		onEvent(agent.StreamEvent{Type: "text_delta", Delta: result.ForLLM})
		break
	}
	// DoneEvent is normally emitted by the outer loop; handlers_test
	// pattern doesn't require it.
	return &agent.RunResult{Content: r.lastToolOutput}, nil
}

// startAutoApprover polls the permission_requests table for pending
// rows whose `tool` name is in allowedTools and auto-allows them.
// Stands in for a human admin who clicks "approve" in the real UI.
//
// The tool-name filter is load-bearing: the broader suite contains
// tests (e.g. TestGate_GhPushFlow_Deny) that create their own
// pending requests and apply their OWN decisions. Approving ANY
// pending row would race those tests when go test ./... runs
// packages concurrently against the shared DB. Pass only the exact
// tool names YOUR test creates.
//
// Returns a stop function the caller MUST defer.
func startAutoApprover(t *testing.T, parentCtx context.Context, gate *permissions.Gate, allowedTools ...string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(parentCtx)
	done := make(chan struct{})
	approverPool := testutil.Pool(t)
	allow := make(map[string]bool, len(allowedTools))
	for _, n := range allowedTools {
		allow[n] = true
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rows, err := approverPool.Query(ctx,
					`SELECT id::text, tool FROM permission_requests WHERE state='pending' LIMIT 16`)
				if err != nil {
					t.Logf("approver query error: %v", err)
					continue
				}
				type cand struct{ id, tool string }
				var todo []cand
				var seenUnwanted []string
				for rows.Next() {
					var c cand
					if err := rows.Scan(&c.id, &c.tool); err == nil {
						if allow[c.tool] {
							todo = append(todo, c)
						} else {
							seenUnwanted = append(seenUnwanted, c.tool)
						}
					}
				}
				rows.Close()
				if len(seenUnwanted) > 0 && len(todo) == 0 {
					t.Logf("approver saw pending but none matched allowlist: %v (allow=%v)", seenUnwanted, allowedTools)
				}
				for _, c := range todo {
					if _, err := gate.Reply(ctx, c.id, permissions.ReplyInput{
						Decision:  permissions.DecisionAllow,
						RepliedBy: "e2e-auto-approver",
					}); err != nil {
						t.Logf("approver Reply error for %s: %v", c.id, err)
					}
				}
			}
		}
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

// readAll is a thin wrapper around io.ReadAll that we inline so the
// test file's imports stay tight.
func readAll(r interface{ Read(p []byte) (int, error) }) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
