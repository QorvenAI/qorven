// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/testutil"
	"github.com/qorvenai/qorven/internal/tools"
)

// trackingTool is a real tools.Tool we wrap with the gate. We count
// Execute calls so we can prove: allow → 1 call; deny → 0 calls.
type trackingTool struct {
	name  string
	calls atomic.Int32
}

func (t *trackingTool) Name() string                                 { return t.name }
func (t *trackingTool) Description() string                          { return "tracking" }
func (t *trackingTool) Parameters() map[string]any                   { return map[string]any{} }
func (t *trackingTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	t.calls.Add(1)
	return tools.TextResult("executed")
}

// TestGate_GhPushFlow_Allow simulates the production flow: a goroutine
// enters the wrapped tool, blocks inside gate.Request, a second
// goroutine plays the user and calls Reply(Allow), the inner tool's
// Execute runs exactly once.
//
// Phase 3 (FU-030): uses NewIsolatedTenant + a per-test session id so
// waitForPending's SQL scope never sees other tests' rows. Zero
// quiesce patterns needed.
func TestGate_GhPushFlow_Allow(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	sessionID := seedIsolatedSession(t, ctx, pool, tenantID)

	gate := permissions.NewGate(pool, nil)
	gate.DefaultTimeout = 30 * time.Second

	inner := &trackingTool{name: "gh_push_file_test"}
	wrapped := permissions.WrapLazy(
		func() *permissions.Gate { return gate },
		inner,
		permissions.GatedToolOptions{
			Reason:      "test",
			RequestedBy: "agent",
			Timeout:     30 * time.Second,
		},
	)

	done := make(chan *tools.Result, 1)
	go func() {
		done <- wrapped.Execute(ctx, map[string]any{
			"session_id": sessionID,
			"owner":      "qorven-ai",
			"repo":       "test",
			"path":       "hello.txt",
			"content":    "hi",
			"branch":     "main",
			"message":    "test",
		})
	}()

	pendingID := waitForPendingBySession(t, ctx, pool, sessionID, 2*time.Second)

	if _, err := gate.Reply(ctx, pendingID, permissions.ReplyInput{
		Decision: permissions.DecisionAllow, RepliedBy: "reviewer",
	}); err != nil {
		t.Fatalf("Reply allow: %v", err)
	}

	select {
	case res := <-done:
		if res == nil || res.IsError {
			t.Fatalf("expected success, got %+v", res)
		}
		if res.ForLLM != "executed" {
			t.Fatalf("inner tool output: %q", res.ForLLM)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("wrapped tool did not return after allow")
	}
	if inner.calls.Load() != 1 {
		t.Fatalf("inner tool expected 1 call, got %d", inner.calls.Load())
	}
}

// TestGate_GhPushFlow_Deny proves the inner tool is NOT called when
// the user denies the gate request.
func TestGate_GhPushFlow_Deny(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	sessionID := seedIsolatedSession(t, ctx, pool, tenantID)

	gate := permissions.NewGate(pool, nil)
	gate.DefaultTimeout = 30 * time.Second

	inner := &trackingTool{name: "gh_push_file_test"}
	wrapped := permissions.WrapLazy(
		func() *permissions.Gate { return gate },
		inner,
		permissions.GatedToolOptions{Timeout: 30 * time.Second},
	)

	done := make(chan *tools.Result, 1)
	go func() {
		done <- wrapped.Execute(ctx, map[string]any{"session_id": sessionID, "path": "x"})
	}()

	pendingID := waitForPendingBySession(t, ctx, pool, sessionID, 2*time.Second)
	if _, err := gate.Reply(ctx, pendingID, permissions.ReplyInput{
		Decision: permissions.DecisionDeny, RepliedBy: "reviewer", Note: "wrong repo",
	}); err != nil {
		t.Fatalf("Reply deny: %v", err)
	}

	select {
	case res := <-done:
		if res == nil || !res.IsError {
			t.Fatalf("expected error result on deny, got %+v", res)
		}
		if !strings.Contains(res.ForLLM, "denied") {
			t.Fatalf("expected denial message, got %q", res.ForLLM)
		}
		if !strings.Contains(res.ForLLM, "wrong repo") {
			t.Fatalf("expected deny note passthrough, got %q", res.ForLLM)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("wrapped tool did not return after deny")
	}
	if inner.calls.Load() != 0 {
		t.Fatalf("inner tool must NOT be called on deny (got %d calls)", inner.calls.Load())
	}
}

// TestGate_GhPushFlow_Timeout proves the gate auto-denies on timeout
// and the inner tool is not called.
func TestGate_GhPushFlow_Timeout(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	sessionID := seedIsolatedSession(t, ctx, pool, tenantID)

	gate := permissions.NewGate(pool, nil)
	gate.DefaultTimeout = 150 * time.Millisecond

	inner := &trackingTool{name: "gh_push_file_test"}
	wrapped := permissions.WrapLazy(
		func() *permissions.Gate { return gate },
		inner,
		permissions.GatedToolOptions{Timeout: 150 * time.Millisecond},
	)

	start := time.Now()
	res := wrapped.Execute(ctx, map[string]any{"session_id": sessionID})
	elapsed := time.Since(start)

	if res == nil || !res.IsError {
		t.Fatalf("expected error on timeout, got %+v", res)
	}
	if !strings.Contains(res.ForLLM, "expired") {
		t.Fatalf("expected expired message, got %q", res.ForLLM)
	}
	if inner.calls.Load() != 0 {
		t.Fatalf("inner tool must NOT run on timeout (got %d)", inner.calls.Load())
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("Execute returned too fast (%s) — timeout not honored", elapsed)
	}
}

// TestGate_GhPushFlow_NilGate proves wrap fails closed when the gate
// is never constructed (misconfiguration).
func TestGate_GhPushFlow_NilGate(t *testing.T) {
	inner := &trackingTool{name: "gh_push_file_test"}
	wrapped := permissions.WrapLazy(
		func() *permissions.Gate { return nil },
		inner,
		permissions.GatedToolOptions{},
	)
	res := wrapped.Execute(context.Background(), map[string]any{})
	if res == nil || !res.IsError {
		t.Fatalf("expected error with nil gate, got %+v", res)
	}
	if inner.calls.Load() != 0 {
		t.Fatalf("inner tool ran with nil gate — fail-open regression")
	}
	if !strings.Contains(res.ForLLM, "gate not configured") {
		t.Fatalf("expected explicit error, got %q", res.ForLLM)
	}
}

// seedIsolatedSession creates a sessions row under the given tenant so
// we can thread a real UUID as session_id through the wrapped tool.
// The permission_requests table's session_id column is a UUID FK-less
// reference; we still use a real row to exercise the production path.
// Returns the session id.
func seedIsolatedSession(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()

	// Minimal agent row the sessions FK needs.
	var agentID string
	if err := pool.QueryRow(ctx, `
        INSERT INTO agents (tenant_id, agent_key, display_name, model)
        VALUES ($1, $2, 'perms-test', 'test-model')
        RETURNING id
    `, tenantID, "perms-"+testutil.TempID("a")).Scan(&agentID); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	// Minimal session row (no session_key uniqueness issues: tenant is isolated).
	var sessionID string
	if err := pool.QueryRow(ctx, `
        INSERT INTO sessions (tenant_id, agent_id, session_key, user_id, messages, channel, status)
        VALUES ($1, $2, $3, 'operator', '[]', 'test', 'active')
        RETURNING id
    `, tenantID, agentID, "perms-"+testutil.TempID("s")).Scan(&sessionID); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Phase 7 test hygiene — delete permission_requests tied to this
	// session before the tenant cleanup runs. Rows inserted by the
	// gate land with tenant_id='default' (the column default) when
	// the caller doesn't set RequestInput.TenantID — which these
	// legacy tests intentionally don't — so the testutil tenant
	// cleanup can't find them. Scope by session_id instead.
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx,
			`DELETE FROM permission_requests WHERE session_id = $1::uuid`, sessionID)
	})
	return sessionID
}

// waitForPendingBySession polls permission_requests for a pending row
// scoped to the given session_id and returns its id. Using a
// session-scoped filter (instead of "newest pending globally") makes
// concurrent test runs safe — each test owns its own session UUID so
// there is no cross-test ambiguity.
func waitForPendingBySession(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string, budget time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		var id string
		err := pool.QueryRow(ctx, `
            SELECT id FROM permission_requests
            WHERE state = 'pending' AND session_id = $1::uuid
            ORDER BY created_at DESC LIMIT 1
        `, sessionID).Scan(&id)
		if err == nil && id != "" {
			return id
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no pending permission request for session %s appeared within %s", sessionID, budget)
	return ""
}
