// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config is the heartbeat configuration for an agent.
type Config struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	Enabled          bool      `json:"enabled"`
	IntervalSec      int       `json:"interval_sec"`
	Model            string    `json:"model,omitempty"`
	TokenBudget      int       `json:"token_budget"`
	MaxIterations    int       `json:"max_iterations"`
	ActiveHoursStart string    `json:"active_hours_start,omitempty"`
	ActiveHoursEnd   string    `json:"active_hours_end,omitempty"`
	Timezone         string    `json:"timezone"`
	Probes           []Probe   `json:"probes"`
	Policy           Policy    `json:"policy"`
	Checklist        string    `json:"checklist,omitempty"`
	CurrentState     string    `json:"current_state"`
	ConsecFailures   int       `json:"consecutive_failures"`
	ConsecPasses     int       `json:"consecutive_passes"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	NextRunAt        *time.Time `json:"next_run_at,omitempty"`
	RunCount         int       `json:"run_count"`
}

// Probe is a deterministic check (no LLM, <100ms).
type Probe struct {
	Name           string `json:"name"`
	Type           string `json:"type"`            // http, exec, tcp
	URL            string `json:"url,omitempty"`    // for http
	Command        string `json:"command,omitempty"` // for exec
	Host           string `json:"host,omitempty"`   // for tcp
	Port           int    `json:"port,omitempty"`   // for tcp
	ExpectedStatus int    `json:"expected_status,omitempty"` // for http
	Threshold      int    `json:"threshold,omitempty"`       // for exec (numeric comparison)
	TimeoutSec     int    `json:"timeout_sec"`
}

// ProbeResult is the output of a deterministic probe.
type ProbeResult struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Value    string `json:"value,omitempty"`
	Error    string `json:"error,omitempty"`
	Duration int    `json:"duration_ms"`
}

// Policy defines state machine rules.
type Policy struct {
	DegradedAfter  int `json:"degraded_after"`  // consecutive failures before degraded (default 2)
	CriticalAfter  int `json:"critical_after"`  // from degraded, failures before critical (default 1)
	RecoveryAfter  int `json:"recovery_after"`  // consecutive passes before healthy (default 3)
	CooldownSec    int `json:"cooldown_sec"`    // min seconds between actions (default 300)
}

// RunResult is the output of a heartbeat run.
type RunResult struct {
	Status       string        `json:"status"`        // completed, suppressed, error, skipped
	PhaseReached int           `json:"phase_reached"` // 1-5
	ProbeResults []ProbeResult `json:"probe_results,omitempty"`
	PolicyState  string        `json:"policy_state"`
	StateChanged bool          `json:"state_changed"`
	LLMCalled    bool          `json:"llm_called"`
	Summary      string        `json:"summary,omitempty"`
	Error        string        `json:"error,omitempty"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	Duration     time.Duration `json:"duration"`
}

// --- Probe Execution ---

// RunProbes executes all deterministic probes. No LLM, <100ms each.
func RunProbes(ctx context.Context, probes []Probe) []ProbeResult {
	results := make([]ProbeResult, 0, len(probes))
	for _, p := range probes {
		start := time.Now()
		var result ProbeResult
		result.Name = p.Name

		timeout := time.Duration(p.TimeoutSec) * time.Second
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		probeCtx, cancel := context.WithTimeout(ctx, timeout)

		switch p.Type {
		case "http":
			result = runHTTPProbe(probeCtx, p)
		case "exec":
			result = runExecProbe(probeCtx, p)
		case "tcp":
			result = runTCPProbe(probeCtx, p)
		default:
			result.Error = fmt.Sprintf("unknown probe type: %s", p.Type)
		}

		cancel()
		result.Duration = int(time.Since(start).Milliseconds())
		results = append(results, result)
	}
	return results
}

func runHTTPProbe(ctx context.Context, p Probe) ProbeResult {
	r := ProbeResult{Name: p.Name}
	req, err := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	r.Value = fmt.Sprintf("HTTP %d", resp.StatusCode)
	expected := p.ExpectedStatus
	if expected == 0 {
		expected = 200
	}
	r.OK = resp.StatusCode == expected
	return r
}

func runExecProbe(ctx context.Context, p Probe) ProbeResult {
	r := ProbeResult{Name: p.Name}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", p.Command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.Error = err.Error()
		r.Value = strings.TrimSpace(string(out))
		return r
	}
	r.Value = strings.TrimSpace(string(out))
	r.OK = true

	// If threshold set, compare numeric value
	if p.Threshold > 0 {
		var val int
		fmt.Sscanf(r.Value, "%d", &val)
		r.OK = val < p.Threshold
		if !r.OK {
			r.Error = fmt.Sprintf("value %d exceeds threshold %d", val, p.Threshold)
		}
	}
	return r
}

func runTCPProbe(ctx context.Context, p Probe) ProbeResult {
	r := ProbeResult{Name: p.Name}
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	conn.Close()
	r.OK = true
	r.Value = "connected"
	return r
}

// --- Policy Engine ---

// EvaluatePolicy runs the state machine against probe results.
// Returns new state and whether a transition occurred.
func EvaluatePolicy(cfg *Config, probeResults []ProbeResult) (newState string, changed bool) {
	policy := cfg.Policy
	if policy.DegradedAfter <= 0 {
		policy.DegradedAfter = 2
	}
	if policy.RecoveryAfter <= 0 {
		policy.RecoveryAfter = 3
	}
	if policy.CriticalAfter <= 0 {
		policy.CriticalAfter = 1
	}

	// Count failures
	allOK := true
	for _, r := range probeResults {
		if !r.OK {
			allOK = false
			break
		}
	}

	oldState := cfg.CurrentState
	if len(probeResults) == 0 {
		// No probes configured — stay in current state
		return oldState, false
	}

	if allOK {
		cfg.ConsecPasses++
		cfg.ConsecFailures = 0
	} else {
		cfg.ConsecFailures++
		cfg.ConsecPasses = 0
	}

	// State machine with hysteresis
	switch oldState {
	case "healthy":
		if cfg.ConsecFailures >= policy.DegradedAfter {
			return "degraded", true
		}
	case "degraded":
		if cfg.ConsecFailures >= policy.CriticalAfter {
			return "critical", true
		}
		if cfg.ConsecPasses >= policy.RecoveryAfter {
			return "healthy", true
		}
	case "critical":
		if cfg.ConsecPasses >= 1 {
			return "recovering", true
		}
	case "recovering":
		if cfg.ConsecPasses >= policy.RecoveryAfter {
			return "healthy", true
		}
		if cfg.ConsecFailures >= 1 {
			return "critical", true
		}
	case "unknown":
		if allOK {
			return "healthy", true
		}
		return "degraded", true
	}

	return oldState, false
}

// ShouldEscalateToLLM decides if the heartbeat needs an LLM call.
func ShouldEscalateToLLM(cfg *Config, probeResults []ProbeResult, stateChanged bool, pendingTasks int) bool {
	// Always escalate if state changed (agent needs to know)
	if stateChanged {
		return true
	}
	// Escalate if there are pending tasks
	if pendingTasks > 0 {
		return true
	}
	// Escalate if any probe has an error (ambiguous situation)
	for _, r := range probeResults {
		if r.Error != "" {
			return true
		}
	}
	// Escalate if checklist is configured (agent needs to run through it)
	if cfg.Checklist != "" {
		return true
	}
	// No escalation needed — all probes OK, no tasks, no state change
	return false
}

// --- Active Hours Check ---

// IsWithinActiveHours checks if current time is within configured active hours.
func IsWithinActiveHours(cfg *Config) bool {
	if cfg.ActiveHoursStart == "" || cfg.ActiveHoursEnd == "" {
		return true // no restriction
	}
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	nowMinutes := now.Hour()*60 + now.Minute()

	start := parseHHMM(cfg.ActiveHoursStart)
	end := parseHHMM(cfg.ActiveHoursEnd)

	if start <= end {
		return nowMinutes >= start && nowMinutes < end
	}
	// Midnight wrapping (e.g., 22:00 - 06:00)
	return nowMinutes >= start || nowMinutes < end
}

func parseHHMM(s string) int {
	var h, m int
	fmt.Sscanf(s, "%d:%d", &h, &m)
	return h*60 + m
}

// --- Stagger Offset (from Qorven) ---

// StaggerOffset returns a deterministic offset to spread heartbeats evenly.
// Uses FNV-1a hash of agent ID.
func StaggerOffset(agentID string, intervalSec int) time.Duration {
	if intervalSec <= 0 {
		return 0
	}
	h := uint32(2166136261) // FNV offset basis
	for _, b := range []byte(agentID) {
		h ^= uint32(b)
		h *= 16777619
	}
	maxOffset := intervalSec / 10 // 10% of interval
	if maxOffset < 1 {
		maxOffset = 1
	}
	offset := int(h) % maxOffset
	if offset < 0 {
		offset = -offset
	}
	return time.Duration(offset) * time.Second
}

// --- Config Store ---

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Get(ctx context.Context, agentID string) (*Config, error) {
	cfg := &Config{}
	var probesJSON, policyJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, enabled, interval_sec, model, token_budget, max_iterations,
		        active_hours_start, active_hours_end, timezone, probes, policy, checklist,
		        current_state, consecutive_failures, consecutive_passes,
		        last_run_at, next_run_at, run_count
		 FROM heartbeat_configs WHERE agent_id = $1`, agentID,
	).Scan(&cfg.ID, &cfg.AgentID, &cfg.Enabled, &cfg.IntervalSec, &cfg.Model,
		&cfg.TokenBudget, &cfg.MaxIterations, &cfg.ActiveHoursStart, &cfg.ActiveHoursEnd,
		&cfg.Timezone, &probesJSON, &policyJSON, &cfg.Checklist,
		&cfg.CurrentState, &cfg.ConsecFailures, &cfg.ConsecPasses,
		&cfg.LastRunAt, &cfg.NextRunAt, &cfg.RunCount)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(probesJSON, &cfg.Probes)
	json.Unmarshal(policyJSON, &cfg.Policy)
	return cfg, nil
}

func (s *Store) Upsert(ctx context.Context, tenantID string, cfg *Config) error {
	probesJSON, _ := json.Marshal(cfg.Probes)
	policyJSON, _ := json.Marshal(cfg.Policy)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO heartbeat_configs (tenant_id, agent_id, enabled, interval_sec, model, token_budget,
		 max_iterations, active_hours_start, active_hours_end, timezone, probes, policy, checklist)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (agent_id) DO UPDATE SET
		 enabled=$3, interval_sec=$4, model=$5, token_budget=$6, max_iterations=$7,
		 active_hours_start=$8, active_hours_end=$9, timezone=$10, probes=$11, policy=$12,
		 checklist=$13, updated_at=NOW()`,
		tenantID, cfg.AgentID, cfg.Enabled, cfg.IntervalSec, cfg.Model, cfg.TokenBudget,
		cfg.MaxIterations, cfg.ActiveHoursStart, cfg.ActiveHoursEnd, cfg.Timezone,
		probesJSON, policyJSON, cfg.Checklist)
	return err
}

func (s *Store) ListDue(ctx context.Context, now time.Time) ([]Config, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, enabled, interval_sec, model, token_budget, max_iterations,
		        active_hours_start, active_hours_end, timezone, probes, policy, checklist,
		        current_state, consecutive_failures, consecutive_passes, run_count
		 FROM heartbeat_configs
		 WHERE enabled = true AND (next_run_at IS NULL OR next_run_at <= $1)`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := []Config{}
	for rows.Next() {
		var cfg Config
		var probesJSON, policyJSON []byte
		rows.Scan(&cfg.ID, &cfg.AgentID, &cfg.Enabled, &cfg.IntervalSec, &cfg.Model,
			&cfg.TokenBudget, &cfg.MaxIterations, &cfg.ActiveHoursStart, &cfg.ActiveHoursEnd,
			&cfg.Timezone, &probesJSON, &policyJSON, &cfg.Checklist,
			&cfg.CurrentState, &cfg.ConsecFailures, &cfg.ConsecPasses, &cfg.RunCount)
		json.Unmarshal(probesJSON, &cfg.Probes)
		json.Unmarshal(policyJSON, &cfg.Policy)
		configs = append(configs, cfg)
	}
	return configs, nil
}

func (s *Store) UpdateState(ctx context.Context, id string, state string, consecFail, consecPass, runCount int, nextRun time.Time, lastStatus, lastError string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE heartbeat_configs SET current_state=$2, consecutive_failures=$3, consecutive_passes=$4,
		 run_count=$5, next_run_at=$6, last_run_at=NOW(), last_status=$7, last_error=$8, updated_at=NOW()
		 WHERE id=$1`,
		id, state, consecFail, consecPass, runCount, nextRun, lastStatus, lastError)
	return err
}

func (s *Store) LogRun(ctx context.Context, tenantID string, run RunResult, heartbeatID, agentID string) error {
	probeJSON, _ := json.Marshal(run.ProbeResults)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO heartbeat_runs (tenant_id, heartbeat_id, agent_id, status, phase_reached,
		 probe_results, policy_state, state_changed, llm_called, summary, error, input_tokens, output_tokens, duration_ms)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		tenantID, heartbeatID, agentID, run.Status, run.PhaseReached,
		probeJSON, run.PolicyState, run.StateChanged, run.LLMCalled,
		run.Summary, run.Error, run.InputTokens, run.OutputTokens, run.Duration.Milliseconds())
	return err
}
