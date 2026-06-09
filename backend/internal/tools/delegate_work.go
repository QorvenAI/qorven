// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import "context"

// OnDelegateWork is wired by the gateway to the rooms delegation orchestrator.
// It returns a short human-readable confirmation (or an error) the head sees.
var OnDelegateWork func(ctx context.Context, headID, worker, task string) (string, error)

// DelegateWorkTool lets a head, while working in a room, assign a piece of work
// to one of its direct reports. The worker does the work and reports back in the
// same room; a summary rolls up to the company hub.
type DelegateWorkTool struct{}

func NewDelegateWorkTool() *DelegateWorkTool { return &DelegateWorkTool{} }

func (t *DelegateWorkTool) Name() string { return "delegate_work" }

func (t *DelegateWorkTool) Description() string {
	return "Assign a piece of work to someone on your team (one of your direct reports). They will do the work and report back in this room. Use this for real work; answer trivial questions yourself. Args: worker (their role or name, e.g. \"engineer\"), task (the full instruction)."
}

func (t *DelegateWorkTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"worker": map[string]any{"type": "string", "description": "The team member to assign to — their role (e.g. \"devops\") or name."},
			"task":   map[string]any{"type": "string", "description": "The full instruction for the work to be done."},
		},
		"required": []string{"worker", "task"},
	}
}

func (t *DelegateWorkTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnDelegateWork == nil {
		return ErrorResult("delegation is not available right now")
	}
	worker, _ := args["worker"].(string)
	task, _ := args["task"].(string)
	if worker == "" || task == "" {
		return ErrorResult("both 'worker' and 'task' are required")
	}
	headID := AgentIDFromCtx(ctx)
	msg, err := OnDelegateWork(ctx, headID, worker, task)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return SuccessResult(msg)
}
