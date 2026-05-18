// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// ParallelToolExecutor runs independent tool calls concurrently.
// Qorven pattern: if the LLM requests multiple tool calls in one turn,
// execute them in parallel (unless they mutate the same file).
type ParallelToolExecutor struct {
	registry    *tools.Registry
	maxParallel int
}

// NewParallelToolExecutor creates a parallel executor.
func NewParallelToolExecutor(registry *tools.Registry, maxParallel int) *ParallelToolExecutor {
	if maxParallel <= 0 {
		maxParallel = 4
	}
	return &ParallelToolExecutor{registry: registry, maxParallel: maxParallel}
}

// ToolCall represents a single tool invocation request from the LLM.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// ToolResult is the outcome of a tool execution.
type ParallelToolResult struct {
	CallID  string
	Name    string
	Content string
	IsError bool
	Elapsed time.Duration
}

// Execute runs tool calls, parallelizing independent ones.
// File-mutating tools (write_file, edit, exec) are serialized to prevent conflicts.
func (e *ParallelToolExecutor) Execute(ctx context.Context, calls []ToolCall) []ParallelToolResult {
	if len(calls) == 0 {
		return nil
	}
	if len(calls) == 1 {
		return []ParallelToolResult{e.execOne(ctx, calls[0])}
	}

	// Classify: file-mutating vs read-only
	var serial, parallel []ToolCall
	for _, c := range calls {
		if isMutating(c.Name) {
			serial = append(serial, c)
		} else {
			parallel = append(parallel, c)
		}
	}

	results := make([]ParallelToolResult, 0, len(calls))

	// Run read-only tools in parallel
	if len(parallel) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		sem := make(chan struct{}, e.maxParallel)
		for _, c := range parallel {
			wg.Add(1)
			sem <- struct{}{}
			go func(call ToolCall) {
				defer wg.Done()
				defer func() { <-sem }()
				r := e.execOne(ctx, call)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}(c)
		}
		wg.Wait()
	}

	// Run mutating tools serially
	for _, c := range serial {
		results = append(results, e.execOne(ctx, c))
	}

	return results
}

func (e *ParallelToolExecutor) execOne(ctx context.Context, call ToolCall) (result ParallelToolResult) {
	start := time.Now()

	// Panic recovery — a tool must never crash the agent
	defer func() {
		if r := recover(); r != nil {
			result = ParallelToolResult{
				CallID: call.ID, Name: call.Name,
				Content: fmt.Sprintf("tool %q panicked: %v", call.Name, r),
				IsError: true, Elapsed: time.Since(start),
			}
		}
	}()

	tool, ok := e.registry.Get(call.Name)
	if !ok {
		return ParallelToolResult{
			CallID: call.ID, Name: call.Name,
			Content: fmt.Sprintf("tool %q not found", call.Name),
			IsError: true, Elapsed: time.Since(start),
		}
	}

	toolResult := tool.Execute(ctx, call.Args)
	elapsed := time.Since(start)

	if elapsed > 10*time.Second {
		slog.Info("slow tool", "name", call.Name, "elapsed", elapsed)
	}

	return ParallelToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Content: toolResult.ForLLM,
		IsError: toolResult.IsError,
		Elapsed: elapsed,
	}
}

// isMutating returns true for tools that modify files or state.
func isMutating(name string) bool {
	switch name {
	case "write_file", "edit", "exec", "patch", "delete_file",
		"create_directory", "move_file", "sandbox_exec":
		return true
	}
	return false
}
