// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/llm"
)

const (
	MaxDepth    = 3
	MaxFanOut   = 5
	MaxTreeSize = 15
	Timeout     = 5 * time.Minute
)

type Task struct {
	ID          string `json:"id"`
	Brief       string `json:"brief"`
	Tools       []string `json:"tools"`
	ParentID    string `json:"parent_id,omitempty"`
	Depth       int    `json:"depth"`
}

type Result struct {
	TaskID  string          `json:"task_id"`
	Content string          `json:"content"`
	Score   int             `json:"score"` // 1-10 quality gate
	Status  string          `json:"status"` // success, retry, failed
	Data    json.RawMessage `json:"data,omitempty"`
}

type Orchestrator struct {
	llm       llm.Provider
	mu        sync.Mutex
	active    map[string]context.CancelFunc
	treeSize  int
}

func NewOrchestrator(provider llm.Provider) *Orchestrator {
	return &Orchestrator{llm: provider, active: make(map[string]context.CancelFunc)}
}

// Spawn creates and runs a sub-agent with isolated context.
func (o *Orchestrator) Spawn(ctx context.Context, task Task) (*Result, error) {
	if task.Depth > MaxDepth {
		return nil, fmt.Errorf("max depth %d exceeded", MaxDepth)
	}
	o.mu.Lock()
	if o.treeSize >= MaxTreeSize {
		o.mu.Unlock()
		return nil, fmt.Errorf("max tree size %d exceeded", MaxTreeSize)
	}
	o.treeSize++
	taskCtx, cancel := context.WithTimeout(ctx, Timeout)
	task.ID = uuid.New().String()[:8]
	o.active[task.ID] = cancel
	o.mu.Unlock()

	defer func() {
		cancel()
		o.mu.Lock()
		delete(o.active, task.ID)
		o.treeSize--
		o.mu.Unlock()
	}()

	slog.Info("sub-agent spawned", "task", task.ID, "depth", task.Depth, "brief", task.Brief[:min(50, len(task.Brief))])

	// Execute with isolated context — sub-agent only sees its task brief
	resp, err := o.llm.Chat(taskCtx, llm.ChatRequest{
		Model: "balanced",
		Messages: []llm.Message{
			{Role: "system", Content: "You are a focused sub-agent. Complete ONLY the assigned task. Be concise and structured."},
			{Role: "user", Content: task.Brief},
		},
	})
	if err != nil {
		return &Result{TaskID: task.ID, Status: "failed", Content: err.Error()}, nil
	}

	result := &Result{TaskID: task.ID, Content: resp.Content, Status: "success"}

	// Quality gate — score the result
	result.Score = o.evaluateQuality(taskCtx, task.Brief, resp.Content)
	if result.Score < 4 {
		result.Status = "failed"
		slog.Warn("sub-agent quality gate failed", "task", task.ID, "score", result.Score)
	} else if result.Score < 7 {
		result.Status = "retry"
		slog.Info("sub-agent quality marginal", "task", task.ID, "score", result.Score)
	}

	return result, nil
}

// SpawnParallel runs multiple sub-agents in parallel (max MaxFanOut).
func (o *Orchestrator) SpawnParallel(ctx context.Context, tasks []Task) []*Result {
	if len(tasks) > MaxFanOut {
		tasks = tasks[:MaxFanOut]
	}
	results := make([]*Result, len(tasks))
	var wg sync.WaitGroup
	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t Task) {
			defer wg.Done()
			r, err := o.Spawn(ctx, t)
			if err != nil {
				results[idx] = &Result{TaskID: t.ID, Status: "failed", Content: err.Error()}
			} else {
				results[idx] = r
			}
		}(i, task)
	}
	wg.Wait()
	return results
}

// Decompose uses LLM to break a complex task into sub-tasks.
func (o *Orchestrator) Decompose(ctx context.Context, task string, maxSubtasks int) ([]Task, error) {
	if maxSubtasks > MaxFanOut {
		maxSubtasks = MaxFanOut
	}
	prompt := fmt.Sprintf(`Break this task into %d focused sub-tasks. Return JSON array of objects with "brief" field only.

Task: %s

Return ONLY valid JSON array, no explanation.`, maxSubtasks, task)

	resp, err := o.llm.Chat(ctx, llm.ChatRequest{
		Model:    "balanced",
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return nil, err
	}

	var subtasks []struct{ Brief string `json:"brief"` }
	if err := json.Unmarshal([]byte(resp.Content), &subtasks); err != nil {
		// Try extracting JSON from response
		return []Task{{Brief: task, Depth: 1}}, nil
	}

	tasks := []Task{}
	for _, st := range subtasks {
		tasks = append(tasks, Task{Brief: st.Brief, Depth: 1})
	}
	return tasks, nil
}

// KillAll terminates all active sub-agents.
func (o *Orchestrator) KillAll() {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, cancel := range o.active {
		cancel()
		slog.Info("sub-agent killed", "task", id)
	}
}

// evaluateQuality scores a result 1-10 using LLM self-review.
func (o *Orchestrator) evaluateQuality(ctx context.Context, brief, result string) int {
	evalCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := o.llm.Chat(evalCtx, llm.ChatRequest{
		Model: "balanced",
		Messages: []llm.Message{{Role: "user", Content: fmt.Sprintf(
			`Score this result 1-10 for the given task. Return ONLY a number.
Task: %s
Result: %s`, brief[:min(200, len(brief))], result[:min(500, len(result))])}},
	})
	if err != nil {
		return 5 // Default to marginal on eval failure
	}
	score := 5
	fmt.Sscanf(resp.Content, "%d", &score)
	if score < 1 {
		score = 1
	}
	if score > 10 {
		score = 10
	}
	return score
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
