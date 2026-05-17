// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// QorvenProvider runs tasks in-process via agent.Loop.Run — no SSE, no CLI.
// Unlike Kiro/ClaudeCode (SSE-pull), we actually execute the task here.
package providers

import (
	"context"
	"fmt"
	"sync"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/daemon"
)

// QorvenProvider implements daemon.AgentProvider by dispatching tasks directly
// to the in-process agent.Loop.  Each Dispatch call spawns a goroutine, holds a
// cancel func, and reports progress / completion via the registry callbacks.
type QorvenProvider struct {
	loop     *agent.Loop
	reg      *daemon.Registry
	agentID  string
	tenantID string

	mu      sync.Mutex
	cancels map[string]context.CancelFunc // taskID → cancel
}

// NewQorvenProvider wires up the in-process provider.
// agentID is the daemon AgentInstance.ID that was registered for this provider;
// it is used to attach progress events to the correct instance.
func NewQorvenProvider(loop *agent.Loop, reg *daemon.Registry, agentID, tenantID string) *QorvenProvider {
	return &QorvenProvider{
		loop:     loop,
		reg:      reg,
		agentID:  agentID,
		tenantID: tenantID,
		cancels:  make(map[string]context.CancelFunc),
	}
}

// ProviderID satisfies daemon.AgentProvider.
func (q *QorvenProvider) ProviderID() string { return "qorven_internal" }

// Name satisfies daemon.AgentProvider.
func (q *QorvenProvider) Name() string { return "Qorven (in-process)" }

// Capabilities returns the full task-type set supported by the internal agent.
func (q *QorvenProvider) Capabilities() []string {
	return []string{"code", "backend", "frontend", "review", "plan", "research", "test", "write"}
}

// Dispatch executes the task in a background goroutine.
// It constructs a RunRequest from the task, streams progress events back to
// the registry, and calls Complete or Fail when the run finishes.
func (q *QorvenProvider) Dispatch(ctx context.Context, task *daemon.Task) error {
	runCtx, cancel := context.WithCancel(ctx)

	q.mu.Lock()
	q.cancels[task.ID] = cancel
	q.mu.Unlock()

	go func() {
		defer func() {
			q.mu.Lock()
			delete(q.cancels, task.ID)
			q.mu.Unlock()
			cancel()
		}()

		req := agent.RunRequest{
			AgentID:     q.agentID,
			UserMessage: buildPrompt(task),
			TenantID:    q.tenantID,
		}

		var (
			lastPct int
			files   []string
		)
		result, err := q.loop.Run(runCtx, req, func(evt agent.StreamEvent) {
			switch evt.Type {
			case "tool_result":
				// file_write / file_edit events carry a file path in ToolID
				if evt.Tool == "file_write" || evt.Tool == "file_edit" || evt.Tool == "str_replace_editor" {
					if evt.ToolID != "" {
						files = appendUnique(files, evt.ToolID)
						q.reg.Progress(daemon.TaskProgress{
							TaskID:   task.ID,
							AgentID:  q.agentID,
							Message:  "wrote " + evt.ToolID,
							FilePath: evt.ToolID,
							Action:   "modified",
						})
					}
				}
			default:
				if evt.Content == "" {
					return
				}
				lastPct = clampPct(lastPct + 5)
				q.reg.Progress(daemon.TaskProgress{
					TaskID:  task.ID,
					AgentID: q.agentID,
					Message: truncate(evt.Content, 120),
					Percent: lastPct,
				})
			}
		})

		if err != nil {
			retryable := !isContextErr(err)
			q.reg.Fail(task.ID, err.Error(), retryable)
			return
		}

		summary := ""
		if result != nil {
			summary = result.Content
		}
		q.reg.Complete(task.ID, summary, files)
	}()

	return nil
}

// Cancel stops an in-progress task by cancelling its context.
func (q *QorvenProvider) Cancel(_ context.Context, taskID string) error {
	q.mu.Lock()
	cancel, ok := q.cancels[taskID]
	q.mu.Unlock()

	if !ok {
		return fmt.Errorf("qorven: task %s not running", taskID)
	}
	cancel()
	return nil
}

// Ping is always nil — in-process means always alive.
func (q *QorvenProvider) Ping(_ context.Context) error { return nil }

// ─── helpers ──────────────────────────────────────────────────────────────────

func buildPrompt(task *daemon.Task) string {
	if task.Description != "" {
		return fmt.Sprintf("%s\n\n%s", task.Title, task.Description)
	}
	return task.Title
}

func clampPct(n int) int {
	if n > 95 {
		return 95
	}
	return n
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func isContextErr(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

func appendUnique(ss []string, s string) []string {
	for _, v := range ss {
		if v == s {
			return ss
		}
	}
	return append(ss, s)
}

// compile-time interface check
var _ daemon.AgentProvider = (*QorvenProvider)(nil)
