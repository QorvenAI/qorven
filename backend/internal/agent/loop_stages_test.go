// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestRunStages_HappyPath verifies that runStages calls all stages in
// order and the PipelineState is mutated as expected.
func TestRunStages_HappyPath(t *testing.T) {
	var order []string
	stages := []namedStage{
		{StageContext, func(_ context.Context, s *PipelineState) error {
			order = append(order, "context")
			s.Model = "gpt-4o"
			return nil
		}},
		{StageHistory, func(_ context.Context, s *PipelineState) error {
			order = append(order, "history")
			return nil
		}},
		{StagePrompt, func(_ context.Context, s *PipelineState) error {
			order = append(order, "prompt")
			s.SystemPrompt = "You are helpful."
			return nil
		}},
		{StageThink, func(_ context.Context, _ *PipelineState) error {
			order = append(order, "think")
			return nil
		}},
		{StageAct, func(_ context.Context, s *PipelineState) error {
			order = append(order, "act")
			s.Result = &RunResult{Content: "done"}
			return nil
		}},
		{StageObserve, func(_ context.Context, _ *PipelineState) error {
			order = append(order, "observe")
			return nil
		}},
		{StageMemory, func(_ context.Context, s *PipelineState) error {
			order = append(order, "memory")
			s.MemoryFlushed = true
			return nil
		}},
		{StageSummarize, func(_ context.Context, s *PipelineState) error {
			order = append(order, "summarize")
			s.Duration = time.Millisecond
			return nil
		}},
	}

	state := &PipelineState{Start: time.Now()}
	if err := runStages(context.Background(), stages, state); err != nil {
		t.Fatalf("runStages returned error: %v", err)
	}

	want := []string{"context", "history", "prompt", "think", "act", "observe", "memory", "summarize"}
	if len(order) != len(want) {
		t.Fatalf("stage call order len: want %d, got %d: %v", len(want), len(order), order)
	}
	for i, w := range want {
		if order[i] != w {
			t.Errorf("stage[%d]: want %q, got %q", i, w, order[i])
		}
	}

	// Verify state mutations propagated.
	if state.Model != "gpt-4o" {
		t.Errorf("state.Model = %q, want 'gpt-4o'", state.Model)
	}
	if state.SystemPrompt != "You are helpful." {
		t.Errorf("state.SystemPrompt = %q", state.SystemPrompt)
	}
	if state.Result == nil || state.Result.Content != "done" {
		t.Errorf("state.Result = %+v", state.Result)
	}
	if !state.MemoryFlushed {
		t.Errorf("state.MemoryFlushed not set")
	}
}

// TestRunStages_AbortOnError verifies that a stage returning an error
// aborts the pipeline — subsequent stages are never called.
func TestRunStages_AbortOnError(t *testing.T) {
	var called []string
	boom := errors.New("provider down")
	stages := []namedStage{
		{StageContext, func(_ context.Context, _ *PipelineState) error {
			called = append(called, "context")
			return nil
		}},
		{StageHistory, func(_ context.Context, _ *PipelineState) error {
			called = append(called, "history")
			return boom
		}},
		{StagePrompt, func(_ context.Context, _ *PipelineState) error {
			called = append(called, "prompt")
			return nil
		}},
	}

	err := runStages(context.Background(), stages, &PipelineState{Start: time.Now()})
	if !errors.Is(err, boom) {
		t.Fatalf("runStages error: want %v, got %v", boom, err)
	}
	if len(called) != 2 || called[1] != "history" {
		t.Errorf("stages called after error: %v", called)
	}
}

// TestRunStages_CancelAborts verifies that ctx.Done() stops the
// pipeline before the next stage executes.
func TestRunStages_CancelAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var called int
	stages := []namedStage{
		{StageContext, func(_ context.Context, _ *PipelineState) error {
			called++
			cancel() // cancel after first stage
			return nil
		}},
		{StageHistory, func(_ context.Context, _ *PipelineState) error {
			called++
			return nil
		}},
	}

	err := runStages(ctx, stages, &PipelineState{Start: time.Now()})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if called != 1 {
		t.Errorf("stages called: want 1, got %d", called)
	}
}

// TestStageNames verifies the canonical stage name constants are
// distinct — a regression guard so a copy-paste typo doesn't silently
// create two stages with the same trace label.
func TestStageNames_Distinct(t *testing.T) {
	names := []StageName{
		StageContext, StageHistory, StagePrompt, StageThink,
		StageAct, StageObserve, StageMemory, StageSummarize,
	}
	seen := make(map[StageName]struct{})
	for _, n := range names {
		if _, dup := seen[n]; dup {
			t.Errorf("duplicate stage name: %q", n)
		}
		seen[n] = struct{}{}
	}
	if len(seen) != 8 {
		t.Errorf("expected 8 distinct stage names, got %d", len(seen))
	}
}
