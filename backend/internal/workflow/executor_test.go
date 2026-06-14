// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package workflow

import (
	"context"
	"testing"
	"time"
)

func TestExecCollect_CapturesFields(t *testing.T) {
	e := &Executor{}
	vars := map[string]any{"name": "Ada", "email": "ada@x.com", "extra": "ignored"}
	step := Step{Type: StepCollect, Fields: []string{"name", "email", "missing"}, SaveAs: "collected"}
	out, _, err := e.executeStep(context.Background(), step, vars)
	if err != nil {
		t.Fatalf("collect errored: %v", err)
	}
	got, ok := vars["collected"].(map[string]any)
	if !ok {
		t.Fatalf("collected not a map: %T", vars["collected"])
	}
	if got["name"] != "Ada" || got["email"] != "ada@x.com" {
		t.Fatalf("collect lost fields: %+v", got)
	}
	if _, has := got["missing"]; !has {
		t.Fatal("missing field should be present (empty), not absent")
	}
	_ = out
}

func TestExecWait_DelaysAndCaps(t *testing.T) {
	e := &Executor{}
	step := Step{Type: StepWait, Args: map[string]any{"seconds": 0.05}}
	start := time.Now()
	if _, _, err := e.executeStep(context.Background(), step, map[string]any{}); err != nil {
		t.Fatalf("wait errored: %v", err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("wait did not actually pause")
	}
}

func TestExecWait_RespectsContextCancel(t *testing.T) {
	e := &Executor{}
	step := Step{Type: StepWait, Args: map[string]any{"seconds": 30.0}}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	start := time.Now()
	_, _, _ = e.executeStep(ctx, step, map[string]any{})
	if time.Since(start) > 5*time.Second {
		t.Fatal("wait ignored ctx cancel")
	}
}
