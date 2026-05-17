// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/providers"
)

// SubagentManager spawns and tracks child agent runs.
// Subagents get isolated context (no parent history), blocked dangerous tools,
// and depth limiting to prevent infinite recursion.
//
// From Qorven's subagent system and Qorven's delegate tool.
type SubagentManager struct {
	tasks     map[string]*SubagentTask
	mu        sync.Mutex
	maxDepth  int
	maxParallel int
	activeCount atomic.Int32

	// BlockedTools are tools subagents cannot use (prevent recursion/escalation)
	BlockedTools []string

	// RunFunc executes a subagent. Injected by the parent loop.
	RunFunc func(ctx context.Context, req SubagentRunRequest) (*SubagentResult, error)
}

// SubagentTask tracks a spawned subagent.
type SubagentTask struct {
	ID          string
	ParentID    string
	Task        string
	Status      string // "running", "completed", "failed", "cancelled"
	Result      string
	Depth       int
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// SubagentRunRequest is the input for spawning a subagent.
type SubagentRunRequest struct {
	Task        string
	AgentID     string // which agent persona to use
	SessionID   string // isolated session
	Depth       int
	ParentModel string
	Context     []providers.Message // optional seed context
}

// SubagentResult is the output of a subagent run.
type SubagentResult struct {
	Content    string
	Iterations int
	ToolsUsed  []string
}

// NewSubagentManager creates a manager with sensible defaults.
func NewSubagentManager() *SubagentManager {
	return &SubagentManager{
		tasks:       make(map[string]*SubagentTask),
		maxDepth:    3,
		maxParallel: 5,
		BlockedTools: []string{
			"spawn", "subagent", "team_tasks", "cron",
			"sessions_send", "message", "gateway",
		},
	}
}

// Spawn creates and runs a subagent synchronously. Returns the result.
func (sm *SubagentManager) Spawn(ctx context.Context, parentID string, depth int, task string) (string, error) {
	if depth >= sm.maxDepth {
		return "", fmt.Errorf("max subagent depth %d reached — cannot spawn deeper", sm.maxDepth)
	}
	if int(sm.activeCount.Load()) >= sm.maxParallel {
		return "", fmt.Errorf("max parallel subagents %d reached — wait for existing to complete", sm.maxParallel)
	}
	if sm.RunFunc == nil {
		return "", fmt.Errorf("subagent runner not configured")
	}

	id := uuid.New().String()[:8]
	st := &SubagentTask{
		ID:        id,
		ParentID:  parentID,
		Task:      task,
		Status:    "running",
		Depth:     depth + 1,
		CreatedAt: time.Now(),
	}

	sm.mu.Lock()
	sm.tasks[id] = st
	sm.mu.Unlock()
	sm.activeCount.Add(1)

	slog.Info("subagent.spawn", "id", id, "parent", parentID, "depth", st.Depth, "task", truncateSA(task, 80))

	result, err := sm.RunFunc(ctx, SubagentRunRequest{
		Task:      task,
		SessionID: "subagent-" + id,
		Depth:     st.Depth,
	})

	sm.activeCount.Add(-1)
	now := time.Now()

	sm.mu.Lock()
	if err != nil {
		st.Status = "failed"
		st.Result = err.Error()
	} else {
		st.Status = "completed"
		st.Result = result.Content
	}
	st.CompletedAt = &now
	sm.mu.Unlock()

	if err != nil {
		slog.Warn("subagent.failed", "id", id, "error", err)
		return "", fmt.Errorf("subagent failed: %w", err)
	}

	slog.Info("subagent.completed", "id", id, "iterations", result.Iterations, "result_len", len(result.Content))
	return result.Content, nil
}

// SpawnBatch runs multiple subagents in parallel and collects results.
func (sm *SubagentManager) SpawnBatch(ctx context.Context, parentID string, depth int, tasks []string) ([]string, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	if len(tasks) > sm.maxParallel {
		return nil, fmt.Errorf("batch size %d exceeds max parallel %d", len(tasks), sm.maxParallel)
	}

	results := make([]string, len(tasks))
	errors := make([]error, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()
			result, err := sm.Spawn(ctx, parentID, depth, t)
			results[idx] = result
			errors[idx] = err
		}(i, task)
	}

	wg.Wait()

	// Collect errors
	var errMsgs []string
	for i, err := range errors {
		if err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("task %d: %s", i, err))
		}
	}
	if len(errMsgs) > 0 {
		return results, fmt.Errorf("batch errors: %s", strings.Join(errMsgs, "; "))
	}
	return results, nil
}

// IsToolBlocked checks if a tool is blocked for subagents.
func (sm *SubagentManager) IsToolBlocked(toolName string) bool {
	for _, blocked := range sm.BlockedTools {
		if toolName == blocked {
			return true
		}
	}
	return false
}

// Active returns the count of currently running subagents.
func (sm *SubagentManager) Active() int {
	return int(sm.activeCount.Load())
}

func truncateSA(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
