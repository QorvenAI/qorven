// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/autonomy"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/tools"
)

// Engine is the universal AI brain. Every surface (web, telegram, API, cron,
// agent-to-agent, heartbeat) calls Engine.Process(). The engine doesn't care
// where the input comes from — it just thinks, acts, remembers, and responds.
type Engine struct {
	// Core
	Loop        *agent.Loop
	Config      *config.HotConfig

	// Memory
	Curated     *memory.CuratedStore
	Working     *memory.WorkingMemory

	// Multi-Agent
	Tasks       *agent.TeamTaskStore
	Budget      *agent.BudgetEnforcer
	Wakeup      *agent.WakeupQueue
	Subagents   *agent.SubagentManager

	// Autonomy
	Cron        *autonomy.CronScheduler
	Heartbeat   *autonomy.HeartbeatRunner
	Qoros      map[string]*agent.QorosMode // agentID → QOROS instance
	PrimeDigest *agent.PrimeDigest         // live all-agent status digest for Prime

	// Providers
	Failover    *providers.AuthRotator

	// Stores (injected)
	AgentStore   *agent.Store
	SessionStore *session.Store
	ProviderReg  *providers.Registry
	ToolReg      *tools.Registry
	SkillLoader  *skills.Loader
	MemStore     *memory.Store
}

// New creates and wires the complete engine.
func New(opts Options) *Engine {
	cfg := config.NewHotConfig(opts.ConfigPath)

	e := &Engine{
		Config:       cfg,
		Curated:      memory.NewCuratedStore(opts.MemoryDir),
		Working:      memory.NewWorkingMemory(),
		Tasks:        agent.NewTeamTaskStore(),
		Budget:       agent.NewBudgetEnforcer(),
		Subagents:    agent.NewSubagentManager(),
		Failover:     providers.NewAuthRotator(),
		AgentStore:   opts.AgentStore,
		SessionStore: opts.SessionStore,
		ProviderReg:  opts.ProviderReg,
		ToolReg:      opts.ToolReg,
		SkillLoader:  opts.SkillLoader,
		MemStore:     opts.MemStore,
		Qoros:      make(map[string]*agent.QorosMode),
	}

	// Create the agent loop
	e.Loop = agent.NewLoop(
		opts.AgentStore, opts.SessionStore, opts.ProviderReg,
		opts.ToolReg, opts.SkillLoader, opts.MemStore, opts.TenantID,
	)

	// Wire Learning Loop — auto-retrospective after every task
	defaultProvider := opts.ProviderReg.Default()
	if defaultProvider != nil {
		ll := agent.NewLearningLoop(defaultProvider, "")
		e.Loop.LearningLoop = ll
	}

	// Wire subagent runner back to the engine (subagents use the same brain)
	e.Subagents.RunFunc = func(ctx context.Context, req agent.SubagentRunRequest) (*agent.SubagentResult, error) {
		result, err := e.Process(ctx, Request{
			AgentID:   req.AgentID,
			SessionID: req.SessionID,
			Message:   req.Task,
			Source:    SourceSubagent,
			Depth:     req.Depth,
		})
		if err != nil {
			return nil, err
		}
		return &agent.SubagentResult{
			Content:    result.Content,
			Iterations: result.Iterations,
			ToolsUsed:  result.ToolsUsed,
		}, nil
	}

	// Wire wakeup handler to the engine
	e.Wakeup = agent.NewWakeupQueue(func(req *agent.WakeupRequest) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		e.Wakeup.MarkRunning(req.ID)

		prompt := req.Reason
		if prompt == "" {
			prompt = "You have been woken up. Check for pending work and act on it."
		}

		_, err := e.Process(ctx, Request{
			AgentID:   req.AgentID,
			SessionID: "wakeup-" + req.ID,
			Message:   prompt,
			Source:    Source(req.Source),
			Context:   req.Context,
		})

		e.Wakeup.MarkDone(req.ID, err != nil)
	})

	// Wire cron to the engine
	e.Cron = autonomy.NewCronScheduler(func(ctx context.Context, job *autonomy.CronJob) (*autonomy.CronRunResult, error) {
		start := time.Now()
		result, err := e.Process(ctx, Request{
			AgentID:   job.AgentID,
			SessionID: "cron-" + job.ID,
			Message:   job.Prompt,
			Source:    SourceCron,
		})
		if err != nil {
			return &autonomy.CronRunResult{JobID: job.ID, Success: false}, err
		}
		return &autonomy.CronRunResult{
			JobID:    job.ID,
			AgentID:  job.AgentID,
			Output:   result.Content,
			Success:  true,
			Duration: time.Since(start),
			Tokens:   result.InputTokens + result.OutputTokens,
		}, nil
	})

	// Wire heartbeat to the engine
	e.Heartbeat = autonomy.NewHeartbeatRunner(func(ctx context.Context, agentID string) error {
		_, err := e.Process(ctx, Request{
			AgentID:   agentID,
			SessionID: "heartbeat-" + agentID,
			Message:   "This is your periodic heartbeat. Check HEARTBEAT.md for your checklist and act on any pending items.",
			Source:    SourceHeartbeat,
		})
		return err
	})

	slog.Info("engine.initialized", "memory_dir", opts.MemoryDir, "config", opts.ConfigPath)
	return e
}

// Start begins autonomous systems (cron, heartbeat).
// Also restores QOROS instances and supervisor queue that were active before restart.
func (e *Engine) Start() {
	ctx := context.Background()

	e.Cron.Start()
	e.Heartbeat.Start()

	// Gap B fix: start Prime live digest — keeps Prime aware of all agent activity
	if e.PrimeDigest != nil {
		e.PrimeDigest.Start(ctx)
	}

	// Gap 1 fix: restore QOROS state for agents that were active before restart.
	// Reads qoros_state table — any agent marked active=true gets re-started.
	if e.AgentStore != nil && e.AgentStore.Pool() != nil {
		e.restoreQorosFromDB(ctx)
	}

	slog.Info("engine.started", "cron", "running", "heartbeat", "running")
}

// restoreQorosFromDB re-activates QOROS for agents that were running before restart.
func (e *Engine) restoreQorosFromDB(ctx context.Context) {
	pool := e.AgentStore.Pool()
	if pool == nil {
		return
	}
	tenantID := e.Loop.GetTenantID()
	rows, err := pool.Query(ctx,
		`SELECT agent_id FROM qoros_state WHERE tenant_id = $1 AND active = true`, tenantID)
	if err != nil {
		slog.Warn("engine.qoros_restore_failed", "error", err)
		return
	}
	defer rows.Close()
	restored := 0
	for rows.Next() {
		var agentID string
		if rows.Scan(&agentID) == nil {
			e.StartQoros(ctx, agentID)
			restored++
		}
	}
	if restored > 0 {
		slog.Info("engine.qoros_restored", "count", restored)
	}
}

// Stop gracefully shuts down autonomous systems.
func (e *Engine) Stop() {
	e.Cron.Stop()
	e.Heartbeat.Stop()
	if e.PrimeDigest != nil {
		e.PrimeDigest.Stop()
	}
	for _, k := range e.Qoros {
		k.Stop()
	}
	e.Config.Stop()
	slog.Info("engine.stopped")
}

// StartQoros activates QOROS proactive mode for an agent.
func (e *Engine) StartQoros(ctx context.Context, agentID string) {
	if _, exists := e.Qoros[agentID]; exists {
		return // already running
	}

	k := agent.NewQoros(agentID,
		// onTick: send a tick message through the agent loop.
		// If the agent has active GitHub tasks, the tick carries the current
		// phase context so the agent knows exactly what to do next.
		func(ctx context.Context, tickTime time.Time) error {
			tickMsg := fmt.Sprintf("<tick time=\"%s\" />", tickTime.Format(time.RFC3339))

			// Inject GitHub task context if agent has active work
			if ghCtx := agent.GitHubTaskContext(ctx, agentID); ghCtx != "" {
				tickMsg = fmt.Sprintf(
					"<tick time=\"%s\" />\n\n%s",
					tickTime.Format(time.RFC3339), ghCtx,
				)
			}

			_, err := e.Loop.Run(ctx, agent.RunRequest{
				AgentID:     agentID,
				SessionID:   "qoros-" + agentID[:8],
				UserMessage: tickMsg,
				ChannelType: "qoros",
			}, nil)
			return err
		},
		// onMessage: log proactive messages
		func(aid, content, status string) {
			slog.Info("qoros.proactive", "agent", aid, "status", status, "content_len", len(content))
		},
	)

	// Wire DB persistence — QOROS state and daily logs survive disk wipes
	if e.AgentStore != nil {
		k.SetDB(e.AgentStore, e.Loop.GetTenantID())
		// Mark active in DB so restart recovery picks this agent up
		_ = e.AgentStore.MarkQorosActive(ctx, agentID, e.Loop.GetTenantID(), true)
		// Restore prior state (tick count, sleep) before starting the loop
		k.RestoreState(ctx)
	}

	e.Qoros[agentID] = k
	k.Start(ctx)
	slog.Info("qoros.activated", "agent", agentID)
}

// StopQoros deactivates QOROS for an agent.
func (e *Engine) StopQoros(agentID string) {
	if k, exists := e.Qoros[agentID]; exists {
		k.Stop()
		delete(e.Qoros, agentID)
		// Mark inactive in DB so restart recovery skips this agent
		if e.AgentStore != nil {
			_ = e.AgentStore.MarkQorosActive(context.Background(), agentID, e.Loop.GetTenantID(), false)
		}
		slog.Info("qoros.deactivated", "agent", agentID)
	}
}

// WakeQoros interrupts sleep for an agent (e.g., user sent a message).
func (e *Engine) WakeQoros(agentID, reason string) {
	if k, exists := e.Qoros[agentID]; exists {
		k.Wake(reason)
	}
}

// --- Universal Entry Point ---

// Source identifies where the input came from.
type Source string

const (
	SourceWebChat   Source = "web"
	SourceTelegram  Source = "telegram"
	SourceDiscord   Source = "discord"
	SourceSlack     Source = "slack"
	SourceWhatsApp  Source = "whatsapp"
	SourceAPI       Source = "api"
	SourceCLI       Source = "cli"
	SourceCron      Source = "cron"
	SourceHeartbeat Source = "heartbeat"
	SourceSubagent  Source = "subagent"
	SourceAgent     Source = "agent"
	SourceWebhook   Source = "webhook"
)

// Request is the universal input to the engine.
// Every surface normalizes its input into this format.
type Request struct {
	AgentID   string         // which Soul handles this
	SessionID string         // conversation session
	UserID    string         // who sent it (human or agent)
	Message   string         // the actual message
	Source    Source          // where it came from
	Channel   string         // specific channel (e.g., telegram chat ID)
	Depth     int            // subagent depth (0 = top level)
	Context   map[string]any // extra context (issueId, taskId, etc.)
}

// Response is the universal output from the engine.
type Response struct {
	Content      string
	Thinking     string
	Parts        []agent.MessagePart
	ToolsUsed    []string
	Iterations   int
	InputTokens  int
	OutputTokens int
	SessionID    string
}

// Process is THE universal entry point. Every surface calls this.
// Web chat, Telegram, API, cron, heartbeat, agent-to-agent — all flow here.
func (e *Engine) Process(ctx context.Context, req Request) (*Response, error) {
	// 1. Record in working memory
	e.Working.Emit(memory.EventUserMessage, truncateEngine(req.Message, 200)).
		Importance(0.5).
		Channel(string(req.Source)).
		Record()

	// 2. Check budget before running
	if budgetCheck := e.Budget.CheckBeforeRun(req.UserID, req.AgentID); budgetCheck.Status == agent.BudgetExceeded {
		return &Response{Content: "⚠️ " + budgetCheck.Message}, nil
	}

	// 3. Build the model ID for prompt optimization
	cfg := e.Config.Get()
	modelID := cfg.DefaultModel

	// 4. Smart route: simple → cheap, complex → primary
	if cfg.CheapModel != "" {
		modelID = providers.SmartRoute(req.Message, cfg.DefaultModel, cfg.CheapModel)
	}

	// 5. Get auth profile (with rotation on failure)
	profileID, apiKey := e.Failover.GetKey("default")
	_ = profileID // used for failure tracking
	_ = apiKey    // injected into provider

	// 6. Build the agent loop request with all brain context
	loopReq := agent.RunRequest{
		AgentID:     req.AgentID,
		SessionID:   req.SessionID,
		UserID:      req.UserID,
		UserMessage: req.Message,
		Channel:     string(req.Source),
		Model:       modelID,
		// Inject memory into the runtime context
		MemoryBulletin: e.Curated.ForSystemPrompt("memory"),
		UserProfile:    e.Curated.ForSystemPrompt("user"),
		WorkingMemory:  e.Working.ForSystemPrompt(),
	}

	// 7. Run the brain
	result, err := e.Loop.Run(ctx, loopReq, func(event agent.StreamEvent) {
		// Stream events back to the surface (handled by caller)
	})

	// 8. Record result in working memory
	if err != nil {
		e.Working.Emit(memory.EventError, err.Error()).Importance(0.8).Record()
		// Track auth failure for rotation
		reason := providers.ClassifyError(err.Error())
		e.Failover.MarkFailure("default", profileID, reason)
		return nil, err
	}

	e.Working.Emit(memory.EventAgentResponse, truncateEngine(result.Content, 200)).
		Importance(0.3).Record()
	e.Failover.MarkSuccess("default", profileID)

	// 9. Record spending
	if result.InputTokens+result.OutputTokens > 0 {
		// Rough cost estimate: $0.01 per 1K tokens
		costCents := int64((result.InputTokens + result.OutputTokens) / 100)
		e.Budget.RecordSpend("agent", req.AgentID, costCents)
	}

	// 10. TODO: check for completed team tasks → dispatch dependents

	return &Response{
		Content:      result.Content,
		Thinking:     result.Thinking,
		Parts:        result.Parts,
		ToolsUsed:    result.ToolsUsed,
		Iterations:   result.Iterations,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		SessionID:    req.SessionID,
	}, nil
}

// Options for creating the engine.
type Options struct {
	ConfigPath  string
	MemoryDir   string
	TenantID    string
	AgentStore  *agent.Store
	SessionStore *session.Store
	ProviderReg *providers.Registry
	ToolReg     *tools.Registry
	SkillLoader *skills.Loader
	MemStore    *memory.Store
}

func truncateEngine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
