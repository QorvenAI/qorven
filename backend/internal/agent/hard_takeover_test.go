// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"github.com/qorvenai/qorven/internal/providers"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// hard_takeover_test.go — Tests for tool-budget, flush-before-compaction,
// and learning-loop hooks that together let Qorven take over a
// long-running conversation without losing state.

// ── Tool Budget ──

func TestHard_ToolBudget_EnforceResult_UnderLimit(t *testing.T) {
	tb := DefaultToolBudget()
	content := strings.Repeat("x", 1000)
	result, truncated := tb.EnforceResult(content)
	if truncated { t.Error("1KB should not be truncated") }
	if result != content { t.Error("content should be unchanged") }
}

func TestHard_ToolBudget_EnforceResult_OverLimit(t *testing.T) {
	tb := DefaultToolBudget()
	content := strings.Repeat("x", 60000) // 60KB > 50KB limit
	result, truncated := tb.EnforceResult(content)
	if !truncated { t.Error("60KB should be truncated") }
	if len(result) >= len(content) { t.Error("result should be shorter than original") }
	if !strings.Contains(result, "truncated") { t.Error("should contain truncation notice") }
}

func TestHard_ToolBudget_EnforceResult_PreservesHead(t *testing.T) {
	tb := DefaultToolBudget()
	content := "IMPORTANT_HEADER\n" + strings.Repeat("x", 60000)
	result, _ := tb.EnforceResult(content)
	if !strings.HasPrefix(result, "IMPORTANT_HEADER") { t.Error("head should be preserved") }
}

func TestHard_ToolBudget_EnforceTurn_Tracking(t *testing.T) {
	tb := DefaultToolBudget()
	remaining := tb.EnforceTurn(0)
	if remaining != 200000 { t.Errorf("initial budget: %d", remaining) }

	remaining = tb.EnforceTurn(150000)
	if remaining != 50000 { t.Errorf("after 150K: %d remaining", remaining) }

	remaining = tb.EnforceTurn(250000)
	if remaining >= 0 { t.Errorf("over budget should be negative: %d", remaining) }
}

func TestHard_ToolBudget_Defaults(t *testing.T) {
	tb := DefaultToolBudget()
	if tb.PerResultChars != 50000 { t.Errorf("per-result: %d", tb.PerResultChars) }
	if tb.PerTurnChars != 200000 { t.Errorf("per-turn: %d", tb.PerTurnChars) }
	if tb.PreviewChars != 2000 { t.Errorf("preview: %d", tb.PreviewChars) }
}

// ── Activity Tracker ──

func TestHard_ActivityTracker_InitiallyNotIdle(t *testing.T) {
	at := NewActivityTracker(5 * time.Second)
	if at.IsIdle() { t.Error("should not be idle immediately after creation") }
}

func TestHard_ActivityTracker_BecomesIdle(t *testing.T) {
	at := NewActivityTracker(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	if !at.IsIdle() { t.Error("should be idle after timeout") }
}

func TestHard_ActivityTracker_TouchResetsIdle(t *testing.T) {
	at := NewActivityTracker(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	if !at.IsIdle() { t.Fatal("should be idle") }

	at.Touch()
	if at.IsIdle() { t.Error("should not be idle after Touch") }
}

func TestHard_ActivityTracker_ToolActiveNeverIdle(t *testing.T) {
	at := NewActivityTracker(10 * time.Millisecond)
	at.ToolStart()
	time.Sleep(50 * time.Millisecond)
	if at.IsIdle() { t.Error("should NEVER be idle while tool is active") }

	if at.ActiveDuration() < 40*time.Millisecond {
		t.Errorf("active duration too short: %v", at.ActiveDuration())
	}
}

func TestHard_ActivityTracker_ToolEndResetsActive(t *testing.T) {
	at := NewActivityTracker(5 * time.Second)
	at.ToolStart()
	at.ToolEnd()
	if at.ActiveDuration() != 0 { t.Error("active duration should be 0 after ToolEnd") }
}

func TestHard_ActivityTracker_ConcurrentAccess(t *testing.T) {
	at := NewActivityTracker(time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%3 == 0 { at.Touch() }
			if n%5 == 0 { at.ToolStart(); at.ToolEnd() }
			_ = at.IsIdle()
			_ = at.IdleDuration()
			_ = at.ActiveDuration()
		}(i)
	}
	wg.Wait()
	// No panic = pass
	t.Log("100 concurrent operations: no race ✓")
}

// ── Process Notifier ──

func TestHard_ProcessNotifier_StartAndComplete(t *testing.T) {
	var notified string
	pn := NewProcessNotifier(func(agentID, sessionID, message string) {
		notified = message
	})

	id := pn.Start(context.Background(), "agent-123", "sess-456", "test build", func(ctx context.Context) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "build succeeded", nil
	})

	if id == "" { t.Fatal("should return process ID") }

	// Check it's running
	proc, ok := pn.Get(id)
	if !ok { t.Fatal("process should exist") }
	if proc.Status != "running" { t.Errorf("status: %q", proc.Status) }

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	proc, _ = pn.Get(id)
	if proc.Status != "completed" { t.Errorf("status after completion: %q", proc.Status) }
	if proc.Result != "build succeeded" { t.Errorf("result: %q", proc.Result) }
	if notified == "" { t.Error("notification callback not called") }
	if !strings.Contains(notified, "completed") { t.Errorf("notification: %q", notified) }
	t.Logf("process: started→completed, notified in %v ✓", proc.Duration)
}

func TestHard_ProcessNotifier_FailedProcess(t *testing.T) {
	pn := NewProcessNotifier(func(_, _, msg string) {})

	pn.Start(context.Background(), "agent-123", "sess-456", "failing task", func(ctx context.Context) (string, error) {
		return "", context.DeadlineExceeded
	})

	time.Sleep(100 * time.Millisecond)

	procs := pn.List("agent-123")
	if len(procs) == 0 { t.Fatal("should have process") }
	if procs[0].Status != "failed" { t.Errorf("status: %q", procs[0].Status) }
}

func TestHard_ProcessNotifier_Cleanup(t *testing.T) {
	pn := NewProcessNotifier(nil)
	pn.Start(context.Background(), "agent-123", "sess", "old task", func(ctx context.Context) (string, error) {
		return "done", nil
	})
	time.Sleep(100 * time.Millisecond)

	removed := pn.Cleanup(0) // remove everything completed
	if removed != 1 { t.Errorf("should remove 1, removed %d", removed) }
}

// ── Compaction Registry ──

func TestHard_CompactionRegistry_ThreeStrategies(t *testing.T) {
	reg := NewCompactionRegistry()

	truncate := reg.Get("truncate")
	if truncate == nil || truncate.Name() != "truncate" { t.Error("truncate strategy missing") }

	summarize := reg.Get("summarize")
	if summarize == nil || summarize.Name() != "summarize" { t.Error("summarize strategy missing") }

	hybrid := reg.Get("hybrid")
	if hybrid == nil || hybrid.Name() != "hybrid" { t.Error("hybrid strategy missing") }
}

func TestHard_CompactionRegistry_DefaultIsHybrid(t *testing.T) {
	reg := NewCompactionRegistry()
	def := reg.Get("nonexistent")
	if def == nil { t.Fatal("should return default for unknown key") }
	if def.Name() != "hybrid" { t.Errorf("default should be hybrid, got %q", def.Name()) }
}

func TestHard_TruncateStrategy_KeepsSystemAndRecent(t *testing.T) {
	strategy := &TruncateStrategy{}
	msgs := makeTestMessages(20) // system + 19 user/assistant turns

	compacted := strategy.Compact(context.Background(), msgs, 100)

	// System prompt must survive
	hasSystem := false
	for _, m := range compacted {
		if m.Role == "system" { hasSystem = true }
	}
	if !hasSystem { t.Error("system prompt lost") }

	// Should be shorter than original
	if len(compacted) >= len(msgs) { t.Error("should have fewer messages after truncation") }

	// Last message must survive
	last := compacted[len(compacted)-1]
	origLast := msgs[len(msgs)-1]
	if last.Content != origLast.Content { t.Error("last message should survive") }

	t.Logf("truncate: %d → %d messages ✓", len(msgs), len(compacted))
}

func TestHard_TruncateStrategy_SmallConversation(t *testing.T) {
	strategy := &TruncateStrategy{}
	msgs := makeTestMessages(3)
	compacted := strategy.Compact(context.Background(), msgs, 100000)
	if len(compacted) != len(msgs) { t.Error("small conversation should not be truncated") }
}

// ── Tool Guidance ──

func TestHard_MandatoryToolUseSection_HasAllCategories(t *testing.T) {
	section := MandatoryToolUseSection()
	required := []string{"ARITHMETIC", "TIME/DATE", "FILE CONTENTS", "SYSTEM STATE", "HASH/ENCODING", "SEARCH", "CODE EXECUTION"}
	for _, cat := range required {
		if !strings.Contains(section, cat) { t.Errorf("missing category: %s", cat) }
	}
}

func TestHard_ActDontAskSection_HasExamples(t *testing.T) {
	section := ActDontAskSection()
	if !strings.Contains(section, "ACT immediately") { t.Error("should contain ACT directive") }
	if !strings.Contains(section, "clarification") { t.Error("should mention clarification") }
}

// ── Helper ──

func makeTestMessages(n int) []providers.Message {
	msgs := []providers.Message{{Role: "system", Content: strings.Repeat("System prompt. ", 50)}}
	for i := 0; i < n-1; i++ {
		if i%2 == 0 {
			msgs = append(msgs, providers.Message{Role: "user", Content: "User message " + string(rune('A'+i%26)) + " with extra content to increase token count significantly"})
		} else {
			msgs = append(msgs, providers.Message{Role: "assistant", Content: "Assistant response " + string(rune('A'+i%26)) + " with detailed explanation and reasoning about the topic"})
		}
	}
	return msgs
}
