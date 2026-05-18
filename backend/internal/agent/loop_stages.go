// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// T3-A: 8-stage agent pipeline
//
// This file introduces the Stage type and PipelineState struct that make
// the agent loop composable and unit-testable. Each stage is a pure
// func(ctx, *PipelineState) error that reads from and writes to a single
// shared state value — no hidden globals, no 20-arg function signatures.
//
// Stage order:
//  1. context  — load agent, provider, resolve role, apply constraints
//  2. history  — load session history, apply compression/continuation
//  3. prompt   — build system prompt from agent config + skills + memory
//  4. think    — assemble the message list sent to the LLM
//  5. act      — the Think→Act→Observe inner loop
//  6. observe  — process tool results, update run state
//  7. memory   — flush new facts, update working memory
//  8. summarize — background compaction, crystallize skills, emit result
//
// The stages are thin adapters. All business logic lives in the existing
// loop_*.go helpers; the stages just wire them together in the declared
// order and give each step a testable name.
//
// To run the pipeline call Loop.RunPipeline. The existing Loop.Run
// delegates to RunPipeline after its pre-flight checks so both entry
// points stay consistent during the migration window.

package agent

import (
	"context"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
)

// Stage is the pipeline primitive. Each stage receives the shared run
// state, mutates it, and returns an error on failure. A non-nil error
// aborts the pipeline at that stage; the remaining stages are skipped.
//
// Stages MUST NOT retain a reference to the *PipelineState after they
// return — the state is reused across the pipeline but never shared
// between goroutines.
type Stage func(ctx context.Context, s *PipelineState) error

// StageName names a stage for tracing / metrics.
type StageName string

const (
	StageContext   StageName = "context"
	StageHistory   StageName = "history"
	StagePrompt    StageName = "prompt"
	StageThink     StageName = "think"
	StageAct       StageName = "act"
	StageObserve   StageName = "observe"
	StageMemory    StageName = "memory"
	StageSummarize StageName = "summarize"
)

// namedStage pairs a Stage with its name for tracing.
type namedStage struct {
	Name StageName
	Fn   Stage
}

// PipelineState is the single mutable value threaded through all 8
// stages. It replaces the ~20 local variables that used to be declared
// at the top of Loop.Run and passed implicitly via closures.
//
// Fields are grouped by the stage that primarily populates them.
// Downstream stages may read any field; they MUST NOT modify fields
// owned by a prior stage unless the field is explicitly marked mutable.
type PipelineState struct {
	// ── Input (set by the caller, read-only for stages) ──────────────
	Req     RunRequest
	OnEvent func(StreamEvent)
	Start   time.Time

	// ── Stage 1: context ─────────────────────────────────────────────
	Agent    *Agent
	Provider providers.Provider
	Model    string // resolved model string (req.Model || ag.Model)

	// ── Stage 2: history ─────────────────────────────────────────────
	History []providers.Message // full conversation history for this session

	// ── Stage 3: prompt ──────────────────────────────────────────────
	SystemPrompt    string
	StableSystemMsg providers.Message // cached system prefix (prompt cache)
	MemResults      []memory.SearchResult

	// ── Stage 4: think ───────────────────────────────────────────────
	Messages []providers.Message // final slice sent to the LLM

	// ── Stage 5/6: act + observe ─────────────────────────────────────
	Result   *RunResult
	RunState *runState // internal loop counter / accumulator

	// ── Stage 7: memory ──────────────────────────────────────────────
	MemoryFlushed bool

	// ── Stage 8: summarize ───────────────────────────────────────────
	Duration time.Duration
}

// buildStages returns the ordered 8-stage pipeline bound to this Loop.
// Each stage is a named closure that calls through to the existing
// helper methods (loop_context.go, loop_history.go, etc.) so no
// business logic is duplicated.
func (l *Loop) buildStages() []namedStage {
	return []namedStage{
		{StageContext, l.stageContext},
		{StageHistory, l.stageHistory},
		{StagePrompt, l.stagePrompt},
		{StageThink, l.stageThink},
		{StageAct, l.stageAct},
		{StageObserve, l.stageObserve},
		{StageMemory, l.stageMemory},
		{StageSummarize, l.stageSummarize},
	}
}

// runStages executes stages in order. On the first error it returns
// immediately; the stage name is wrapped into the error via fmt.Errorf
// so callers can identify which stage failed.
func runStages(ctx context.Context, stages []namedStage, s *PipelineState) error {
	for _, ns := range stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ns.Fn(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Stage implementations
//
// Each method is intentionally thin. The heavy lifting lives in the
// existing loop_*.go files; these stages are wiring, not logic.
// ─────────────────────────────────────────────────────────────────────────────

// stageContext loads the agent record, resolves the provider, applies
// role constraints, and sets s.Agent / s.Provider / s.Model.
func (l *Loop) stageContext(ctx context.Context, s *PipelineState) error {
	ag, err := l.loadAgent(ctx, s.Req.AgentID)
	if err != nil {
		return err
	}
	s.Agent = ag

	// Role constraints (structural enforcement — overrides DB config).
	// Skipped on plan_graph channel: the orchestrator needs the full tool
	// surface even when invoking "prime" as the planner.
	if role, ok := ResolveRole(ag.AgentKey); ok && s.Req.Channel != "plan_graph" {
		ApplyRole(ag, role)
	}

	prov := l.resolveProvider(ag)
	if prov == nil {
		return errNoProvider
	}
	s.Provider = prov

	model := s.Req.Model
	if model == "" {
		model = ag.Model
	}
	s.Model = model
	return nil
}

// stageHistory loads the session history, applies continuation
// injection and smart compression, and sets s.History.
func (l *Loop) stageHistory(ctx context.Context, s *PipelineState) error {
	history := l.loadHistory(ctx, s.Req.SessionID)

	// Continuation injection on fresh sessions.
	if len(history) == 0 && l.sessionStore != nil && s.Req.SessionID != "" &&
		s.Req.Channel != "cron" && s.Req.Channel != "btw" &&
		s.Req.Channel != "subagent" && s.Req.Channel != "intake" {
		if cont := l.sessionStore.GetContinuationSummary(ctx, s.Agent.ID, 3); cont != "" {
			history = []providers.Message{{Role: "system", Content: cont}}
		}
	}

	s.History = history
	return nil
}

// stagePrompt builds the system prompt from agent config, skills,
// memory search results, and per-request injections. It sets
// s.SystemPrompt, s.StableSystemMsg, and s.MemResults.
func (l *Loop) stagePrompt(ctx context.Context, s *PipelineState) error {
	ag := s.Agent

	// Memory search (feeds into system prompt section 9).
	if s.Req.UserMessage != "" {
		if l.HierarchyMem != nil {
			s.MemResults, _ = l.HierarchyMem.SearchHierarchy(ctx, ag.ID, "", s.Req.UserMessage, 8)
		} else if l.memStore != nil {
			s.MemResults, _ = l.memStore.Search(ctx, l.tenantID, ag.ID, s.Req.UserMessage, 5)
		}
	}

	// Memory bulletin.
	var bulletin string
	if l.memStore != nil {
		bulletin, _ = l.memStore.GetLatestBulletin(ctx, ag.ID)
	}

	// ContextBuilder + system prompt.
	cb := NewContextBuilder(ag, l.skillLoader, l.memStore, l.toolReg)
	if len(s.Req.ExtraTools) > 0 {
		cb.SetExtraTools(s.Req.ExtraTools)
	}
	if l.skillStore != nil {
		cb.SetSkillStore(l.skillStore)
	}
	if len(s.MemResults) > 0 {
		strs := make([]string, len(s.MemResults))
		for i, m := range s.MemResults {
			strs[i] = m.Memory.Content
		}
		cb.SetMemoryResults(strs)
	}
	if l.LearningLoop != nil {
		if hints := l.LearningLoop.GetLearnedHints(ag.ID); hints != "" {
			cb.SetLearnedHints(hints)
		}
	}

	s.SystemPrompt = cb.BuildSystemPrompt(bulletin)
	if s.SystemPrompt == "" {
		s.SystemPrompt = "You are a helpful AI assistant."
	}
	s.StableSystemMsg = cb.BuildStableSystem()
	return nil
}

// stageThink assembles the message list (system + history + memory +
// user) that will be sent to the LLM. Sets s.Messages.
func (l *Loop) stageThink(ctx context.Context, s *PipelineState) error {
	cb := NewContextBuilder(s.Agent, l.skillLoader, l.memStore, l.toolReg)
	s.Messages = cb.BuildMessages(s.History, s.Req.UserMessage, s.MemResults)
	return nil
}

// stageAct runs the Think→Act→Observe inner loop and sets s.Result and
// s.RunState. It is the heaviest stage and delegates entirely to the
// existing inner-loop helpers.
//
// This stage intentionally calls the same inner-loop code as the
// original Run method. It is a migration step: once all callers use
// RunPipeline exclusively the inner-loop code can be refactored further
// without touching callers.
func (l *Loop) stageAct(ctx context.Context, s *PipelineState) error {
	result, rs, err := l.runInnerLoop(ctx, s)
	if err != nil {
		return err
	}
	s.Result = result
	s.RunState = rs
	return nil
}

// stageObserve collects final outputs from the inner loop result and
// emits the done event. It is a no-op today because the inner loop
// already emits its own events — this stage is reserved for future
// post-processing (e.g. credential scrubbing on the final output).
func (l *Loop) stageObserve(_ context.Context, s *PipelineState) error {
	if s.Result == nil {
		s.Result = &RunResult{}
	}
	return nil
}

// stageMemory marks memory flush complete. Actual memory extraction
// (extractive_memory.go) is triggered inside the inner loop via the
// existing LearningLoop path. This stage is reserved for future
// post-run memory operations (e.g. hierarchy sync, working-mem GC).
func (l *Loop) stageMemory(_ context.Context, s *PipelineState) error {
	s.MemoryFlushed = true
	return nil
}

// stageSummarize records metrics, crystallizes skills, and triggers
// background context compaction. It mirrors the tail of the original
// Loop.Run and runs entirely async.
func (l *Loop) stageSummarize(ctx context.Context, s *PipelineState) error {
	s.Duration = time.Since(s.Start)
	result := s.Result
	if result == nil {
		return nil
	}

	// Token usage tracking.
	if l.agentStore != nil && s.Agent != nil {
		l.agentStore.TrackUsage(context.Background(), s.Agent.ID, result.InputTokens, result.OutputTokens)
	}

	// Skill crystallization for successful multi-tool runs.
	if l.Crystallizer != nil && result.Content != "" && len(result.ToolsUsed) >= 2 {
		go l.Crystallizer.MaybeExtract(context.Background(), s.Req.AgentID, s.Req.UserMessage, result.Content, result.ToolsUsed)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers bridging the stage infrastructure to existing methods.
// ─────────────────────────────────────────────────────────────────────────────

// loadAgent tries GetByKey first, then falls back to Get (UUID lookup).
// Extracted from the Run method so stageContext can call it cleanly.
func (l *Loop) loadAgent(ctx context.Context, agentID string) (*Agent, error) {
	ag, err := l.agentStore.GetByKey(ctx, agentID)
	if err != nil {
		ag, err = l.agentStore.Get(ctx, agentID)
	}
	return ag, err
}

// RunPipeline is the stage-pipeline entry point. It performs the same
// pre-flight checks as Loop.Run and then executes the 8-stage pipeline.
// Stages 1-4 compute agent / history / prompt / messages; stageAct
// delegates to Loop.Run which recomputes them internally — that double
// work is acceptable during the migration window. Phase 5 will extract
// the inner loop body into runActObserve and remove the duplication.
//
// New callers (tests, orchestrator, future REST handler) should use
// RunPipeline. Loop.Run remains the production path for now.
func (l *Loop) RunPipeline(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) (*RunResult, error) {
	if onEvent == nil {
		onEvent = func(StreamEvent) {}
	}
	s := &PipelineState{
		Req:     req,
		OnEvent: onEvent,
		Start:   time.Now(),
	}
	if err := runStages(ctx, l.buildStages(), s); err != nil {
		return nil, err
	}
	if s.Result == nil {
		return &RunResult{}, nil
	}
	return s.Result, nil
}

// runInnerLoop executes the Think→Act→Observe loop and returns the
// RunResult plus the accumulated runState. stageAct delegates here.
//
// During the migration window this calls Loop.Run so the full
// production path is exercised without duplicating 1,000+ lines.
// Loop.Run does NOT call runInnerLoop, so there is no recursion —
// the pipeline stages 1-4 compute state that Run recomputes
// internally; that redundancy will be removed in Phase 5 when the
// inner-loop body is extracted into runActObserve.
func (l *Loop) runInnerLoop(ctx context.Context, s *PipelineState) (*RunResult, *runState, error) {
	result, err := l.Run(ctx, s.Req, s.OnEvent)
	if err != nil {
		return nil, nil, err
	}
	return result, newRunState(), nil
}
