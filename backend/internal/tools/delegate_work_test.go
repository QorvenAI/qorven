package tools

import (
	"context"
	"testing"
)

func TestDelegateWorkTool_Schema(t *testing.T) {
	tl := NewDelegateWorkTool()
	if tl.Name() != "delegate_work" {
		t.Errorf("name: got %q", tl.Name())
	}
	params := tl.Parameters()
	props, _ := params["properties"].(map[string]any)
	if _, ok := props["worker"]; !ok {
		t.Errorf("missing 'worker' param")
	}
	if _, ok := props["task"]; !ok {
		t.Errorf("missing 'task' param")
	}
}

func TestDelegateWorkTool_Execute_CallsCallback(t *testing.T) {
	var gotHead, gotWorker, gotTask string
	OnDelegateWork = func(ctx context.Context, headID, worker, task string) (string, error) {
		gotHead, gotWorker, gotTask = headID, worker, task
		return "assigned to @eng (work item wi-1)", nil
	}
	defer func() { OnDelegateWork = nil }()

	ctx := WithAgentID(context.Background(), "h1")
	tl := NewDelegateWorkTool()
	res := tl.Execute(ctx, map[string]any{"worker": "eng", "task": "rebuild onboarding"})
	if res == nil || res.IsError {
		t.Fatalf("expected success result, got %+v", res)
	}
	if gotHead != "h1" || gotWorker != "eng" || gotTask != "rebuild onboarding" {
		t.Errorf("callback args: head=%q worker=%q task=%q", gotHead, gotWorker, gotTask)
	}
}

func TestDelegateWorkTool_Execute_NoCallback(t *testing.T) {
	OnDelegateWork = nil
	tl := NewDelegateWorkTool()
	res := tl.Execute(context.Background(), map[string]any{"worker": "eng", "task": "x"})
	if res == nil || !res.IsError {
		t.Errorf("with no callback wired, expect an error result")
	}
}
