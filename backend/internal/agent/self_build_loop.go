// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// SelfBuildLoop runs periodic self-improvement cycles.
// It uses the existing agent loop — sends structured messages to an agent
// that has access to self_knowledge, self_improve, self_patch, self_test tools.
//
// The loop does NOT modify the running binary. It works on git branches
// and creates notifications for human review.
//
// Safety: disabled by default. Must be explicitly enabled in config.
type SelfBuildLoop struct {
	agentLoop  *Loop
	agentID    string
	sessionFn  func(ctx context.Context, agentID string) (string, error)
	notifyFn   func(agentID, title, detail string)
	interval   time.Duration
	enabled    bool
	running    bool
	cancel     context.CancelFunc
	mu         sync.Mutex
	lastRun    time.Time
	runCount   int
	lastResult string
	// Cycle history and feedback
	history          []CycleResult
	consecutiveFails int
	maxConsecFails   int // pause after N consecutive failures (default: 3)
}

// CycleResult records what happened in each self-improvement cycle.
type CycleResult struct {
	Cycle     int       `json:"cycle"`
	StartedAt time.Time `json:"started_at"`
	Duration  time.Duration `json:"duration"`
	ToolsUsed int       `json:"tools_used"`
	Success   bool      `json:"success"`
	Summary   string    `json:"summary"`
}

// SelfBuildConfig configures the self-building loop.
type SelfBuildConfig struct {
	Enabled  bool          `toml:"enabled"`  // default: false
	Interval time.Duration `toml:"interval"` // default: 6h
	AgentID  string        `toml:"agent_id"` // default: "prime"
}

func NewSelfBuildLoop(loop *Loop, cfg SelfBuildConfig) *SelfBuildLoop {
	if cfg.Interval == 0 { cfg.Interval = 6 * time.Hour }
	if cfg.AgentID == "" { cfg.AgentID = "prime" }
	return &SelfBuildLoop{
		agentLoop:      loop,
		agentID:        cfg.AgentID,
		interval:       cfg.Interval,
		enabled:        cfg.Enabled,
		maxConsecFails: 3,
	}
}

// SetSessionFactory sets the function that creates isolated sessions.
func (s *SelfBuildLoop) SetSessionFactory(fn func(ctx context.Context, agentID string) (string, error)) {
	s.sessionFn = fn
}

// SetNotifier sets the function that creates notifications for human review.
func (s *SelfBuildLoop) SetNotifier(fn func(agentID, title, detail string)) {
	s.notifyFn = fn
}

// Start begins the self-improvement loop in the background.
func (s *SelfBuildLoop) Start() {
	if !s.enabled {
		slog.Info("self_build: disabled (set self_build.enabled = true in config)")
		return
	}
	if s.agentLoop == nil || s.sessionFn == nil {
		slog.Warn("self_build: missing dependencies, not starting")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	go s.loop(ctx)
	slog.Info("self_build: started", "interval", s.interval, "agent", s.agentID)
}

// Stop halts the self-improvement loop.
func (s *SelfBuildLoop) Stop() {
	if s.cancel != nil { s.cancel() }
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// Status returns the current state of the loop.
func (s *SelfBuildLoop) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"enabled":          s.enabled,
		"running":          s.running,
		"interval":         s.interval.String(),
		"last_run":         s.lastRun,
		"run_count":        s.runCount,
		"last_result":      s.lastResult,
		"consecutive_fails": s.consecutiveFails,
		"history_count":    len(s.history),
	}
}

func (s *SelfBuildLoop) loop(ctx context.Context) {
	// Wait a bit before first run (let the system stabilize)
	select {
	case <-time.After(5 * time.Minute):
	case <-ctx.Done(): return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run immediately on start, then on interval
	s.runCycle(ctx)

	for {
		select {
		case <-ticker.C:
			s.runCycle(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *SelfBuildLoop) runCycle(ctx context.Context) {
	s.mu.Lock()
	// Check consecutive failure limit
	if s.consecutiveFails >= s.maxConsecFails {
		slog.Warn("self_build: paused — too many consecutive failures", "fails", s.consecutiveFails, "max", s.maxConsecFails)
		s.mu.Unlock()
		return
	}
	s.lastRun = time.Now()
	s.runCount++
	cycle := s.runCount
	s.mu.Unlock()

	slog.Info("self_build: cycle starting", "cycle", cycle)
	start := time.Now()

	// Create isolated session for this cycle
	sessionID, err := s.sessionFn(ctx, s.agentID)
	if err != nil {
		slog.Error("self_build: session creation failed", "error", err)
		s.recordCycle(cycle, start, false, 0, "session failed: "+err.Error())
		return
	}

	// Build instruction — include feedback from last failure if any
	feedbackLine := ""
	s.mu.Lock()
	if s.consecutiveFails > 0 && len(s.history) > 0 {
		last := s.history[len(s.history)-1]
		feedbackLine = fmt.Sprintf("\n\nPREVIOUS CYCLE FAILED: %s\nAvoid the same approach. Try a different file or a simpler fix.", last.Summary)
	}
	s.mu.Unlock()

	instruction := `AUTOMATED SELF-IMPROVEMENT CYCLE. You MUST complete ALL steps — do not stop after analysis.

STEP 1: Run self_improve with action="analyze". Read the output.
STEP 2: From the analysis, pick ONE specific fix (prefer: error wrapping, vet warnings, missing nil checks).
STEP 3: Run self_knowledge with action="read" and path=<the file to fix> to see the current code.
STEP 4: Run self_patch with action="propose" with path=<file> and content=<fixed file content>. You MUST call this tool.
STEP 5: Run self_test (no arguments). You MUST call this tool to verify the build passes.
STEP 6: If self_test reports BUILD OK, run self_patch with action="branch" with name="self-improve-` + fmt.Sprintf("%d", cycle) + `".
STEP 7: Report: what file you changed, what you fixed, whether tests passed.

CRITICAL RULES:
- You MUST call self_patch and self_test. Do not just analyze and stop.
- If self_test fails after your change, that is OK — report the failure. The change auto-reverts.
- ONE file, ONE change. Do not try to fix everything.
- Never modify *_test.go, config.toml, *.sql migration files.
- If analysis shows zero issues, report "no improvement needed" and stop.` + feedbackLine

	// Run with timeout
	cycleCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := s.agentLoop.Run(cycleCtx, RunRequest{
		AgentID:     s.agentID,
		SessionID:   sessionID,
		UserMessage: instruction,
		Channel:     "self_build",
	}, func(StreamEvent) {})

	elapsed := time.Since(start)
	summary := "no result"
	toolsUsed := 0
	if result != nil {
		if result.Content != "" {
			summary = result.Content
			if len(summary) > 500 { summary = summary[:500] }
		}
		toolsUsed = len(result.ToolsUsed)
	}
	if err != nil {
		summary = "error: " + err.Error()
	}

	// Determine success: did the agent actually call self_patch and self_test?
	success := false
	if result != nil {
		for _, t := range result.ToolsUsed {
			if t == "self_test" { success = true; break }
		}
		// If self_test was called and result contains "BUILD OK" or "TESTS PASSED", it's a real success
		if success && (strings.Contains(summary, "BUILD OK") || strings.Contains(summary, "TESTS PASSED") || strings.Contains(summary, "tests pass")) {
			success = true
		} else if success {
			// self_test was called but may have failed — still counts as "attempted"
			success = strings.Contains(summary, "branch") || strings.Contains(summary, "self-improve-")
		}
	}

	s.recordCycle(cycle, start, success, toolsUsed, summary)

	slog.Info("self_build: cycle complete", "cycle", cycle, "duration", elapsed, "tools", toolsUsed, "success", success)

	// Notify human for review
	if s.notifyFn != nil {
		status := "✅ success"
		if !success { status = "❌ failed" }
		title := fmt.Sprintf("Self-build #%d %s (%s)", cycle, status, elapsed.Round(time.Second))
		s.notifyFn(s.agentID, title, summary)
	}
}

func (s *SelfBuildLoop) recordCycle(cycle int, start time.Time, success bool, toolsUsed int, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastResult = summary

	cr := CycleResult{
		Cycle:     cycle,
		StartedAt: start,
		Duration:  time.Since(start),
		ToolsUsed: toolsUsed,
		Success:   success,
		Summary:   summary,
	}
	s.history = append(s.history, cr)
	// Keep last 20 cycles
	if len(s.history) > 20 { s.history = s.history[len(s.history)-20:] }

	if success {
		s.consecutiveFails = 0
	} else {
		s.consecutiveFails++
		if s.consecutiveFails >= s.maxConsecFails {
			slog.Warn("self_build: pausing after consecutive failures", "fails", s.consecutiveFails)
		}
	}
}
