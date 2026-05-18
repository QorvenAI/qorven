// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/tools"
)

// fakeTool is a minimal tools.Tool for wrap tests.
type fakeTool struct {
	name    string
	ran     bool
	lastCtx context.Context
	resp    *tools.Result
}

func (f *fakeTool) Name() string                                { return f.name }
func (f *fakeTool) Description() string                         { return "fake" }
func (f *fakeTool) Parameters() map[string]any                  { return map[string]any{} }
func (f *fakeTool) Execute(ctx context.Context, _ map[string]any) *tools.Result {
	f.ran = true
	f.lastCtx = ctx
	if f.resp != nil {
		return f.resp
	}
	return tools.TextResult("ok")
}

func TestWrap_NilGateReturnsError(t *testing.T) {
	inner := &fakeTool{name: "gh_push_file"}
	gated := Wrap(nil, inner, GatedToolOptions{})
	r := gated.Execute(context.Background(), map[string]any{})
	if r == nil || !r.IsError {
		t.Fatalf("expected error result")
	}
	if !strings.Contains(r.ForUser, "gate not configured") {
		t.Fatalf("message: %s", r.ForUser)
	}
	if inner.ran {
		t.Fatalf("inner tool must not run when gate is nil")
	}
}

func TestWrap_PreservesNameAndParameters(t *testing.T) {
	inner := &fakeTool{name: "gh_push_file"}
	gated := Wrap(nil, inner, GatedToolOptions{})
	if gated.Name() != "gh_push_file" {
		t.Fatalf("name: %s", gated.Name())
	}
	if gated.Description() != "fake" {
		t.Fatalf("desc: %s", gated.Description())
	}
	if _, ok := gated.Parameters()["type"]; ok {
		t.Fatalf("parameters should be whatever the inner tool returned")
	}
}

func TestDefaultSessionIDFromArgs(t *testing.T) {
	if got := defaultSessionIDFromArgs(nil); got != "" {
		t.Fatalf("nil args → %q", got)
	}
	if got := defaultSessionIDFromArgs(map[string]any{"session_id": "s1"}); got != "s1" {
		t.Fatalf("got %q", got)
	}
	if got := defaultSessionIDFromArgs(map[string]any{"session_id": 42}); got != "" {
		t.Fatalf("non-string session_id must be ignored, got %q", got)
	}
}
