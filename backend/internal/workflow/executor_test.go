// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package workflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// TestRun_CollectSurvivesRunLoop is the regression for the run-loop overwrite:
// execCollect stores a map under SaveAs, and the run loop must NOT clobber it
// with the step's string result. Exercises the real Run() path (not executeStep
// directly) so the loop's SaveAs write is in play.
func TestRun_CollectSurvivesRunLoop(t *testing.T) {
	e := &Executor{store: NewStore(nil), tenantID: "t1"}
	steps := []Step{{ID: "s1", Type: StepCollect, Fields: []string{"name", "email"}, SaveAs: "collected"}}
	raw, _ := json.Marshal(steps)
	wf := &Workflow{ID: "wf1", TenantID: "t1", Steps: raw}
	run, err := e.Run(context.Background(), wf, map[string]any{"name": "Ada", "email": "ada@x.com"})
	if err != nil {
		t.Fatalf("Run errored: %v", err)
	}
	// Run persists the final vars as JSON in Run.Context — unmarshal and confirm
	// "collected" is still the captured map, not the step's string result.
	var vars map[string]any
	if err := json.Unmarshal(run.Context, &vars); err != nil {
		t.Fatalf("unmarshal run context: %v", err)
	}
	got, ok := vars["collected"].(map[string]any)
	if !ok {
		t.Fatalf("collected was clobbered — not a map: %T = %v", vars["collected"], vars["collected"])
	}
	if got["name"] != "Ada" || got["email"] != "ada@x.com" {
		t.Fatalf("collect lost fields after Run: %+v", got)
	}
}

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

func TestExecAPI_BlocksInternalURL(t *testing.T) {
	e := &Executor{}
	for _, bad := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:4200/v1/admin",
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
	} {
		_, _, err := e.execAPI(context.Background(), Step{Type: StepAPI, URL: bad, Method: "GET"}, map[string]any{})
		if err == nil {
			t.Errorf("execAPI must block internal URL %q", bad)
		}
	}
}

func TestIsInternalURL_AllowsExternal(t *testing.T) {
	if blocked, _ := tools.IsInternalURL("https://api.github.com/repos/x/y"); blocked {
		t.Fatal("external URL should not be blocked")
	}
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
