// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/qorvenai/qorven/internal/providers"
)

// SubTask is a single decomposed unit of work.
type SubTask struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	AssignTo    string `json:"assign_to"` // Soul key
	DependsOn   []int  `json:"depends_on,omitempty"`
	Result      string `json:"result,omitempty"`
	Status      string `json:"status"` // pending, running, done, failed
}

// Plan is a decomposed set of subtasks for a complex request.
type Plan struct {
	Goal     string    `json:"goal"`
	SubTasks []SubTask `json:"subtasks"`
}

// RunSoulFunc executes a Soul and returns the result.
type RunSoulFunc func(ctx context.Context, soulKey, task string) (string, error)

// Planner decomposes complex requests into subtasks and orchestrates execution.
type Planner struct {
	provider providers.Provider
	model    string
}

func NewPlanner(provider providers.Provider, model string) *Planner {
	return &Planner{provider: provider, model: model}
}

// Decompose uses LLM to break a complex request into subtasks.
func (p *Planner) Decompose(ctx context.Context, request string, availableSouls []string) (*Plan, error) {
	soulsJSON, _ := json.Marshal(availableSouls)
	prompt := fmt.Sprintf(`Decompose this request into subtasks. Assign each to the most appropriate Soul.

Available Souls: %s

Request: %s

Respond with JSON:
{"goal": "...", "subtasks": [{"id": 1, "description": "...", "assign_to": "soul_key", "depends_on": []}]}

Rules:
- Use 2-5 subtasks max
- Only use available Soul keys
- Use depends_on when a task needs another's output
- If a task can be done by any Soul, use "qorven"`, soulsJSON, request)

	resp, err := p.provider.Chat(ctx, providers.ChatRequest{
		Model:    p.model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options: map[string]any{
			"temperature":     0,
			"response_format": map[string]string{"type": "json_object"},
		},
	})
	if err != nil {
		return nil, err
	}

	var plan Plan
	if err := json.Unmarshal([]byte(resp.Content), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan: %w", err)
	}
	for i := range plan.SubTasks {
		plan.SubTasks[i].Status = "pending"
	}
	return &plan, nil
}

// Execute runs all subtasks, respecting dependencies, with parallel execution where possible.
func (p *Planner) Execute(ctx context.Context, plan *Plan, runSoul RunSoulFunc) error {
	results := make(map[int]string)
	var mu sync.Mutex

	// Build dependency graph
	for {
		// Find tasks that are pending and have all dependencies met
		ready := []int{}
		allDone := true
		for i, st := range plan.SubTasks {
			if st.Status == "pending" {
				allDone = false
				depsOK := true
				for _, dep := range st.DependsOn {
					mu.Lock()
					_, done := results[dep]
					mu.Unlock()
					if !done {
						depsOK = false
						break
					}
				}
				if depsOK {
					ready = append(ready, i)
				}
			} else if st.Status == "running" {
				allDone = false
			}
		}

		if allDone || (len(ready) == 0 && !hasRunning(plan)) {
			break
		}
		if len(ready) == 0 {
			continue // wait for running tasks
		}

		// Execute ready tasks in parallel
		var wg sync.WaitGroup
		for _, idx := range ready {
			wg.Add(1)
			plan.SubTasks[idx].Status = "running"
			go func(i int) {
				defer wg.Done()
				st := plan.SubTasks[i]

				// Build task with dependency context
				task := st.Description
				for _, dep := range st.DependsOn {
					mu.Lock()
					if r, ok := results[dep]; ok {
						task += fmt.Sprintf("\n\nContext from previous task #%d: %s", dep, r[:min(len(r), 500)])
					}
					mu.Unlock()
				}

				slog.Info("plan.execute", "subtask", st.ID, "soul", st.AssignTo, "desc", st.Description[:min(len(st.Description), 80)])
				result, err := runSoul(ctx, st.AssignTo, task)
				mu.Lock()
				if err != nil {
					plan.SubTasks[i].Status = "failed"
					plan.SubTasks[i].Result = "Error: " + err.Error()
				} else {
					plan.SubTasks[i].Status = "done"
					plan.SubTasks[i].Result = result
					results[st.ID] = result
				}
				mu.Unlock()
			}(idx)
		}
		wg.Wait()
	}
	return nil
}

// Aggregate combines all subtask results into a unified response.
func (p *Planner) Aggregate(ctx context.Context, plan *Plan) (string, error) {
	var taskSummaries string
	for _, st := range plan.SubTasks {
		taskSummaries += fmt.Sprintf("## %s (@%s) [%s]\n%s\n\n", st.Description, st.AssignTo, st.Status, st.Result)
	}

	resp, err := p.provider.Chat(ctx, providers.ChatRequest{
		Model: p.model,
		Messages: []providers.Message{
			{Role: "system", Content: "Synthesize these subtask results into a coherent, unified response for the user. Be concise."},
			{Role: "user", Content: fmt.Sprintf("Goal: %s\n\nResults:\n%s", plan.Goal, taskSummaries)},
		},
		Options: map[string]any{"temperature": 0.3},
	})
	if err != nil {
		return taskSummaries, nil // fallback to raw results
	}
	return resp.Content, nil
}

func hasRunning(plan *Plan) bool {
	for _, st := range plan.SubTasks {
		if st.Status == "running" {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
