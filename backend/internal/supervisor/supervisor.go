// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package supervisor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// AgentHealth tracks the health state of a monitored agent.
type AgentHealth struct {
	AgentID          string    `json:"agent_id"`
	AgentName        string    `json:"agent_name"`
	Status           string    `json:"status"` // "healthy", "degraded", "unresponsive", "suspended"
	LastHeartbeat    time.Time `json:"last_heartbeat"`
	LastStatusCheck  time.Time `json:"last_status_check"`
	ConsecErrors     int       `json:"consecutive_errors"`
	TotalErrors7d    int       `json:"total_errors_7d"`
	SamplingRate     float64   `json:"sampling_rate"` // 0.0-1.0, adaptive
	Disagreements    int       `json:"disagreements"`  // progressive suspension counter
	SuspendedFromACK bool      `json:"suspended_from_ack"` // 2nd disagreement → suspended
}

// SupervisorConfig controls the supervisor loop behavior.
type SupervisorConfig struct {
	AuditInterval    time.Duration `json:"audit_interval"`    // how often to check all agents (default 5m)
	HeartbeatTTL     time.Duration `json:"heartbeat_ttl"`     // max time without heartbeat (default 10m)
	ResponseTimeout  time.Duration `json:"response_timeout"`  // max wait for STATUS_REQUEST reply (default 60s)
	MaxAutoFixRisk   RiskLevel     `json:"max_auto_fix_risk"` // highest risk Prime can auto-fix (default low)

	// Adaptive sampling base rates (adjusted by error rate)
	BaseSampleLow    float64 `json:"base_sample_low"`    // default 0.10 (10%)
	BaseSampleMedium float64 `json:"base_sample_medium"` // default 0.30 (30%)
	BaseSampleHigh   float64 `json:"base_sample_high"`   // default 1.00 (100%)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() SupervisorConfig {
	return SupervisorConfig{
		AuditInterval:    5 * time.Minute,
		HeartbeatTTL:     10 * time.Minute,
		ResponseTimeout:  60 * time.Second,
		MaxAutoFixRisk:   RiskLow,
		BaseSampleLow:    0.10,
		BaseSampleMedium: 0.30,
		BaseSampleHigh:   1.00,
	}
}

// Supervisor is Prime's monitoring loop.
// It periodically audits all agents, handles heartbeats, applies auto-fixes,
// and escalates issues to the human.
type Supervisor struct {
	mu       sync.RWMutex
	bus      *Bus
	catalog  *FixCatalog
	config   SupervisorConfig
	agents   map[string]*AgentHealth
	primeID  string // Prime's agent ID

	// External dependencies
	listAgents   func(ctx context.Context) ([]AgentInfo, error)
	evaluateOutput func(ctx context.Context, agentID, output string) (*EvalResult, error)

	stopCh chan struct{}
}

// AgentInfo is minimal agent info from the store.
type AgentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Model       string `json:"model"`
	FallbackModel string `json:"fallback_model"`
}

// EvalResult is Prime's evaluation of an agent's output.
type EvalResult struct {
	Quality    string    `json:"quality"`    // "good", "degraded", "bad"
	Issues     []string  `json:"issues"`     // specific problems found
	Risk       RiskLevel `json:"risk"`       // risk level of the issues
	SuggestedFix *FixType `json:"suggested_fix,omitempty"` // auto-fix suggestion
	FixParams  map[string]any `json:"fix_params,omitempty"`
}

// NewSupervisor creates a new supervisor.
func NewSupervisor(bus *Bus, catalog *FixCatalog, config SupervisorConfig, primeID string) *Supervisor {
	return &Supervisor{
		bus:     bus,
		catalog: catalog,
		config:  config,
		agents:  make(map[string]*AgentHealth),
		primeID: primeID,
		stopCh:  make(chan struct{}),
	}
}

// SetListAgents sets the function to list all agents.
func (s *Supervisor) SetListAgents(fn func(ctx context.Context) ([]AgentInfo, error)) {
	s.listAgents = fn
}

// SetEvaluator sets the function Prime uses to evaluate agent outputs.
func (s *Supervisor) SetEvaluator(fn func(ctx context.Context, agentID, output string) (*EvalResult, error)) {
	s.evaluateOutput = fn
}

// Start begins the supervisor loop.
func (s *Supervisor) Start(ctx context.Context) {
	slog.Info("supervisor.starting", "prime", s.primeID, "interval", s.config.AuditInterval)

	// Register Prime's handler on the bus
	s.bus.Register(s.primeID, s.handleMessage)

	// Start the periodic audit loop
	go s.auditLoop(ctx)

	// Start the heartbeat monitor
	go s.heartbeatMonitor(ctx)
}

// Stop stops the supervisor loop.
func (s *Supervisor) Stop() {
	close(s.stopCh)
}

// auditLoop runs the periodic audit cycle.
// Fires STATUS_REQUEST to all agents concurrently, collects responses.
func (s *Supervisor) auditLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.AuditInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runAuditCycle(ctx)
		}
	}
}

// runAuditCycle sends STATUS_REQUEST to all agents concurrently.
// Never serial — all requests fire in a burst.
func (s *Supervisor) runAuditCycle(ctx context.Context) {
	if s.listAgents == nil {
		return
	}

	agents, err := s.listAgents(ctx)
	if err != nil {
		slog.Error("supervisor.audit.list_agents_failed", "error", err)
		return
	}

	slog.Info("supervisor.audit.start", "agent_count", len(agents))

	var wg sync.WaitGroup
	for _, agent := range agents {
		if agent.ID == s.primeID {
			continue // don't audit yourself
		}

		// Adaptive sampling: decide whether to audit this agent
		if !s.shouldSample(agent.ID) {
			continue
		}

		wg.Add(1)
		go func(a AgentInfo) {
			defer wg.Done()
			s.auditAgent(ctx, a)
		}(agent)
	}

	wg.Wait()
	slog.Info("supervisor.audit.complete", "agent_count", len(agents))
}

// shouldSample decides whether to audit an agent based on adaptive sampling.
// Agents with recent errors get sampled more frequently.
func (s *Supervisor) shouldSample(agentID string) bool {
	s.mu.RLock()
	health, ok := s.agents[agentID]
	s.mu.RUnlock()

	if !ok {
		return true // unknown agent → always sample first time
	}

	// Adaptive rate based on error history
	rate := s.config.BaseSampleLow
	if health.ConsecErrors > 0 {
		rate = s.config.BaseSampleMedium // recent error → sample more
	}
	if health.TotalErrors7d > 5 {
		rate = s.config.BaseSampleHigh // many errors → sample everything
	}
	if health.Status == "degraded" || health.Status == "unresponsive" {
		rate = 1.0 // always sample unhealthy agents
	}

	// Zero errors for 7 days → drop to 5%
	if health.TotalErrors7d == 0 && health.ConsecErrors == 0 {
		rate = 0.05
	}

	health.SamplingRate = rate

	// Probabilistic sampling
	// Use a deterministic hash so the same agent gets consistent sampling
	hash := uint32(0)
	for _, c := range agentID {
		hash = hash*31 + uint32(c)
	}
	threshold := uint32(rate * float64(^uint32(0)))
	return hash < threshold
}

// auditAgent sends a STATUS_REQUEST to one agent and processes the response.
func (s *Supervisor) auditAgent(ctx context.Context, agent AgentInfo) {
	timeout := s.config.ResponseTimeout

	msg := Message{
		From:        s.primeID,
		To:          agent.ID,
		Intent:      IntentStatusRequest,
		Content:     fmt.Sprintf("Status check for %s", agent.Name),
		SyncTimeout: &timeout,
	}

	err := s.bus.Send(ctx, msg)
	if err != nil {
		slog.Warn("supervisor.audit.send_failed", "agent", agent.ID, "error", err)
		s.recordError(agent.ID, agent.Name)
	}
}

// handleMessage processes incoming messages to Prime.
func (s *Supervisor) handleMessage(ctx context.Context, msg Message) *Message {
	switch msg.Intent {
	case IntentReviewRequest:
		return s.handleReviewRequest(ctx, msg)

	case IntentHeartbeat:
		s.recordHeartbeat(msg.From)
		return nil // heartbeat is informational, no response

	case IntentACK:
		return nil // ACK is terminal, never respond

	default:
		slog.Warn("supervisor.unexpected_intent", "intent", msg.Intent, "from", msg.From)
		return nil
	}
}

// handleReviewRequest evaluates an agent's output and decides: ACK, AUTO_FIX, or ESCALATE.
func (s *Supervisor) handleReviewRequest(ctx context.Context, msg Message) *Message {
	agentID := msg.From

	// If we have an evaluator, use it
	if s.evaluateOutput != nil {
		output, _ := msg.Context["output"].(string)
		eval, err := s.evaluateOutput(ctx, agentID, output)
		if err != nil {
			slog.Error("supervisor.eval_failed", "agent", agentID, "error", err)
			// Can't evaluate → ACK with warning
			return &Message{
				From:    s.primeID,
				To:      agentID,
				Intent:  IntentACK,
				Content: "Evaluation failed, ACK with warning. Will retry next cycle.",
				Risk:    RiskLow,
			}
		}

		switch eval.Quality {
		case "good":
			s.recordSuccess(agentID)
			return &Message{
				From:    s.primeID,
				To:      agentID,
				Intent:  IntentACK,
				Content: "Output verified. Clean.",
			}

		case "degraded":
			// Try auto-fix if available and low risk
			if eval.SuggestedFix != nil && eval.Risk == RiskLow {
				result, err := s.catalog.Apply(ctx, *eval.SuggestedFix, eval.FixParams, s.config.MaxAutoFixRisk)
				if err == nil && result.Success {
					s.recordSuccess(agentID)
					return &Message{
						From:    s.primeID,
						To:      agentID,
						Intent:  IntentAutoFix,
						Content: fmt.Sprintf("Auto-fixed: %s", *eval.SuggestedFix),
						Context: map[string]any{"fix_type": *eval.SuggestedFix, "fix_result": result},
					}
				}
			}
			// Can't auto-fix → escalate
			s.recordError(agentID, "")
			return s.escalate(msg, eval)

		case "bad":
			s.recordError(agentID, "")
			return s.escalate(msg, eval)
		}
	}

	// No evaluator → ACK everything (safe default)
	return &Message{
		From:    s.primeID,
		To:      agentID,
		Intent:  IntentACK,
		Content: "Acknowledged (no evaluator configured).",
	}
}

// escalate sends an ESCALATION_NOTICE to the human.
func (s *Supervisor) escalate(originalMsg Message, eval *EvalResult) *Message {
	// Check progressive suspension
	s.mu.Lock()
	health := s.getOrCreateHealth(originalMsg.From, "")
	health.Disagreements++

	if health.Disagreements >= 2 && !health.SuspendedFromACK {
		health.SuspendedFromACK = true
		slog.Warn("supervisor.agent_suspended", "agent", originalMsg.From, "disagreements", health.Disagreements)
	}
	s.mu.Unlock()

	// Build escalation message
	issues := "No specific issues"
	if eval != nil && len(eval.Issues) > 0 {
		issues = fmt.Sprintf("%v", eval.Issues)
	}

	escalation := Message{
		From:       s.primeID,
		To:         "human",
		Intent:     IntentEscalationNotice,
		Content:    fmt.Sprintf("Agent %s needs review. Issues: %s", originalMsg.From, issues),
		Risk:       eval.Risk,
		Context:    map[string]any{"original_message": originalMsg, "evaluation": eval},
		ExchangeID: originalMsg.ExchangeID,
	}

	// Send to human via the bus (triggers onEscalation callback)
	s.bus.Send(context.Background(), escalation)

	// ACK the original agent — Prime handled it (escalated)
	return &Message{
		From:    s.primeID,
		To:      originalMsg.From,
		Intent:  IntentACK,
		Content: "Escalated to human for review.",
	}
}

// heartbeatMonitor checks for agents that haven't sent a heartbeat within TTL.
func (s *Supervisor) heartbeatMonitor(ctx context.Context) {
	ticker := time.NewTicker(s.config.HeartbeatTTL / 2) // check at half the TTL
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkHeartbeats(ctx)
		}
	}
}

// checkHeartbeats finds agents that haven't sent a heartbeat and probes them.
func (s *Supervisor) checkHeartbeats(ctx context.Context) {
	s.mu.RLock()
	var stale []string
	cutoff := time.Now().Add(-s.config.HeartbeatTTL)
	for id, health := range s.agents {
		if !health.LastHeartbeat.IsZero() && health.LastHeartbeat.Before(cutoff) && health.Status != "unresponsive" {
			stale = append(stale, id)
		}
	}
	s.mu.RUnlock()

	for _, agentID := range stale {
		slog.Warn("supervisor.heartbeat.stale", "agent", agentID)

		// Send one STATUS_REQUEST as a probe
		timeout := s.config.ResponseTimeout
		err := s.bus.Send(ctx, Message{
			From:        s.primeID,
			To:          agentID,
			Intent:      IntentStatusRequest,
			Content:     "Heartbeat timeout — are you alive?",
			SyncTimeout: &timeout,
		})

		if err != nil {
			// No response → mark as unresponsive
			s.mu.Lock()
			if health, ok := s.agents[agentID]; ok {
				health.Status = "unresponsive"
				slog.Error("supervisor.agent.unresponsive", "agent", agentID)
			}
			s.mu.Unlock()

			// Escalate
			s.bus.Send(ctx, Message{
				From:    s.primeID,
				To:      "human",
				Intent:  IntentEscalationNotice,
				Content: fmt.Sprintf("Agent %s is unresponsive (no heartbeat for %s)", agentID, s.config.HeartbeatTTL),
				Risk:    RiskMedium,
			})
		}
	}
}

// --- Health tracking ---

func (s *Supervisor) recordHeartbeat(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	health := s.getOrCreateHealth(agentID, "")
	health.LastHeartbeat = time.Now()
	if health.Status == "unresponsive" {
		health.Status = "healthy"
		slog.Info("supervisor.agent.recovered", "agent", agentID)
	}
}

func (s *Supervisor) recordSuccess(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	health := s.getOrCreateHealth(agentID, "")
	health.ConsecErrors = 0
	health.Status = "healthy"
	health.LastStatusCheck = time.Now()
}

func (s *Supervisor) recordError(agentID, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	health := s.getOrCreateHealth(agentID, name)
	health.ConsecErrors++
	health.TotalErrors7d++
	health.LastStatusCheck = time.Now()
	if health.ConsecErrors >= 3 {
		health.Status = "degraded"
	}
}

func (s *Supervisor) getOrCreateHealth(agentID, name string) *AgentHealth {
	health, ok := s.agents[agentID]
	if !ok {
		health = &AgentHealth{
			AgentID:   agentID,
			AgentName: name,
			Status:    "healthy",
		}
		s.agents[agentID] = health
	}
	return health
}

// --- Public API ---

// IsSuspended returns true if the agent has been suspended from ACK (progressive suspension).
// The daemon Registry calls this before dispatching tasks.
func (s *Supervisor) IsSuspended(agentID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.agents[agentID]
	return ok && h.SuspendedFromACK
}

// Unsuspend clears the SuspendedFromACK flag and resets disagreements.
// Called when a human explicitly clears the suspension.
func (s *Supervisor) Unsuspend(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.agents[agentID]; ok {
		h.SuspendedFromACK = false
		h.Disagreements = 0
		h.Status = "healthy"
		slog.Info("supervisor.agent_unsuspended", "agent", agentID)
	}
}

// AgentHealthList returns health status for all monitored agents.
func (s *Supervisor) AgentHealthList() []*AgentHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*AgentHealth, 0, len(s.agents))
	for _, h := range s.agents {
		copy := *h
		list = append(list, &copy)
	}
	return list
}

// ResolveEscalation marks an escalation as resolved by the human.
// Catalog returns the fix catalog for external consumers (e.g. the /supervisor/fixes route).
func (s *Supervisor) Catalog() *FixCatalog { return s.catalog }

func (s *Supervisor) ResolveEscalation(ctx context.Context, escalationID string, approved bool, reason string) error {
	intent := IntentACK
	content := "Human approved: " + reason
	if !approved {
		content = "Human rejected: " + reason
	}

	return s.bus.Send(ctx, Message{
		From:    "human",
		To:      s.primeID,
		Intent:  intent,
		Content: content,
		ReplyTo: escalationID,
	})
}
