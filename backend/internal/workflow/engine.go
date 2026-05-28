// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Extended step types for multi-agent orchestration (complements store.go constants).
const (
	StepAgentCall     = "agent_call"
	StepParallelAgent = "parallel_agents"
	StepApprovalGate  = "approval_gate"
	StepBudgetCheck   = "budget_check"
	StepQualityGate   = "quality_gate"
	StepTimer         = "timer"
	StepWebhook       = "webhook"
	StepTransform     = "transform"
	StepEscalate      = "escalate"
)

type WorkflowStep struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	AgentKey    string            `json:"agent_key,omitempty"`
	AgentKeys   []string          `json:"agent_keys,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Config      map[string]any    `json:"config,omitempty"`
	Next        string            `json:"next,omitempty"`
	Branches    map[string]string `json:"branches,omitempty"`
	RetryPolicy *RetryPolicy      `json:"retry_policy,omitempty"`
	TimeoutMs   int               `json:"timeout_ms,omitempty"`
}

type RetryPolicy struct {
	MaxRetries    int     `json:"max_retries"`
	BackoffMs     int     `json:"backoff_ms"`
	BackoffFactor float64 `json:"backoff_factor"`
}

type WorkflowDefinition struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Steps       []WorkflowStep `json:"steps"`
	EntryStep   string         `json:"entry_step"`
	TriggerType string         `json:"trigger_type"`
}

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusTimedOut  RunStatus = "timed_out"
	RunStatusCancelled RunStatus = "cancelled"
	RunStatusPaused    RunStatus = "paused"
)

type StepRunStatus string

const (
	StepPending   StepRunStatus = "pending"
	StepRunning   StepRunStatus = "running"
	StepCompleted StepRunStatus = "completed"
	StepFailed    StepRunStatus = "failed"
	StepSkipped   StepRunStatus = "skipped"
)

type WorkflowRun struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	WorkflowID    string     `json:"workflow_id"`
	Status        RunStatus  `json:"status"`
	CurrentStepID string     `json:"current_step_id"`
	Context       map[string]any `json:"context"`
	TriggerType   string     `json:"trigger_type"`
	TriggeredBy   string     `json:"triggered_by"`
	Deadline      *time.Time `json:"deadline"`
	TotalCostUUSD int64      `json:"total_cost_uusd"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	Error         string     `json:"error"`
}

type StepRun struct {
	ID           string        `json:"id"`
	RunID        string        `json:"run_id"`
	StepID       string        `json:"step_id"`
	AgentID      string        `json:"agent_id"`
	Status       StepRunStatus `json:"status"`
	Input        map[string]any `json:"input"`
	Output       map[string]any `json:"output"`
	CostUUSD     int64         `json:"cost_uusd"`
	QualityScore float64       `json:"quality_score"`
	DurationMs   int           `json:"duration_ms"`
	RetryCount   int           `json:"retry_count"`
	StartedAt    *time.Time    `json:"started_at"`
	CompletedAt  *time.Time    `json:"completed_at"`
	Error        string        `json:"error"`
}

type StepEvent struct {
	RunID   string `json:"run_id"`
	StepID  string `json:"step_id"`
	Event   string `json:"event"` // step_started, step_completed, step_failed, run_completed
	Payload any    `json:"payload"`
}

type AgentInvoker func(ctx context.Context, agentKey, prompt string, wfContext map[string]any) (string, int64, error)

type Engine struct {
	db           *pgxpool.Pool
	invokeAgent  AgentInvoker
	mu           sync.Mutex
	activeRuns   map[string]context.CancelFunc
	onStepEvent  func(StepEvent)
}

func NewEngine(db *pgxpool.Pool, invoker AgentInvoker) *Engine {
	return &Engine{
		db:          db,
		invokeAgent: invoker,
		activeRuns:  make(map[string]context.CancelFunc),
	}
}

func (e *Engine) SetEventHandler(handler func(StepEvent)) {
	e.onStepEvent = handler
}

func (e *Engine) StartRun(ctx context.Context, def WorkflowDefinition, triggerType, triggeredBy string, deadline *time.Time) (*WorkflowRun, error) {
	if e.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	run := &WorkflowRun{
		ID:            uuid.New().String(),
		TenantID:      def.TenantID,
		WorkflowID:    def.ID,
		Status:        RunStatusRunning,
		CurrentStepID: def.EntryStep,
		Context:       make(map[string]any),
		TriggerType:   triggerType,
		TriggeredBy:   triggeredBy,
		Deadline:      deadline,
		StartedAt:     time.Now(),
	}

	ctxJSON, _ := json.Marshal(run.Context)
	_, err := e.db.Exec(ctx, `
		INSERT INTO workflow_runs (id, tenant_id, workflow_id, status, current_step_id, context, trigger_type, triggered_by, deadline, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, run.ID, run.TenantID, run.WorkflowID, run.Status, run.CurrentStepID, ctxJSON, run.TriggerType, run.TriggeredBy, run.Deadline, run.StartedAt)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.activeRuns[run.ID] = cancel
	e.mu.Unlock()

	go e.executeRun(runCtx, run, def)

	return run, nil
}

func (e *Engine) CancelRun(runID string) {
	e.mu.Lock()
	cancel, ok := e.activeRuns[runID]
	e.mu.Unlock()
	if ok {
		cancel()
	}
}

func (e *Engine) executeRun(ctx context.Context, run *WorkflowRun, def WorkflowDefinition) {
	defer func() {
		e.mu.Lock()
		delete(e.activeRuns, run.ID)
		e.mu.Unlock()
	}()

	stepMap := make(map[string]WorkflowStep)
	for _, s := range def.Steps {
		stepMap[s.ID] = s
	}

	currentStepID := def.EntryStep
	for currentStepID != "" {
		if ctx.Err() != nil {
			e.completeRun(ctx, run, RunStatusCancelled, "cancelled by user")
			return
		}

		if run.Deadline != nil && time.Now().After(*run.Deadline) {
			e.completeRun(ctx, run, RunStatusTimedOut, "deadline exceeded")
			return
		}

		step, ok := stepMap[currentStepID]
		if !ok {
			e.completeRun(ctx, run, RunStatusFailed, fmt.Sprintf("step %s not found", currentStepID))
			return
		}

		run.CurrentStepID = currentStepID
		e.updateRunStatus(ctx, run)

		nextStepID, err := e.executeStep(ctx, run, step)
		if err != nil {
			if step.RetryPolicy != nil && e.shouldRetry(ctx, run, step, err) {
				continue
			}
			e.completeRun(ctx, run, RunStatusFailed, fmt.Sprintf("step %s failed: %v", step.ID, err))
			return
		}

		currentStepID = nextStepID
	}

	e.completeRun(ctx, run, RunStatusCompleted, "")
}

func (e *Engine) executeStep(ctx context.Context, run *WorkflowRun, step WorkflowStep) (string, error) {
	startedAt := time.Now()
	stepRun := &StepRun{
		ID:      uuid.New().String(),
		RunID:   run.ID,
		StepID:  step.ID,
		Status:  StepRunning,
		Input:   run.Context,
		StartedAt: &startedAt,
	}
	e.recordStepRun(ctx, stepRun)
	e.emitEvent(StepEvent{RunID: run.ID, StepID: step.ID, Event: "step_started"})

	var stepCtx context.Context
	var cancel context.CancelFunc
	if step.TimeoutMs > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutMs)*time.Millisecond)
	} else {
		stepCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
	}
	defer cancel()

	var output string
	var costUUSD int64
	var err error

	switch step.Type {
	case StepAgentCall:
		output, costUUSD, err = e.invokeAgent(stepCtx, step.AgentKey, e.renderPrompt(step.Prompt, run.Context), run.Context)

	case StepParallelAgent:
		output, costUUSD, err = e.executeParallel(stepCtx, step, run)

	case StepCondition:
		nextID := e.evaluateCondition(step, run.Context)
		completedAt := time.Now()
		stepRun.Status = StepCompleted
		stepRun.CompletedAt = &completedAt
		stepRun.DurationMs = int(time.Since(startedAt).Milliseconds())
		e.recordStepRun(ctx, stepRun)
		e.emitEvent(StepEvent{RunID: run.ID, StepID: step.ID, Event: "step_completed"})
		return nextID, nil

	case StepApprovalGate:
		e.completeRun(ctx, run, RunStatusPaused, "awaiting approval at step "+step.ID)
		return "", nil

	case StepBudgetCheck:
		if !e.checkBudget(stepCtx, run, step) {
			return "", fmt.Errorf("budget exceeded for step %s", step.ID)
		}
		completedAt := time.Now()
		stepRun.Status = StepCompleted
		stepRun.CompletedAt = &completedAt
		e.recordStepRun(ctx, stepRun)
		return step.Next, nil

	case StepTimer:
		durationMs := 0
		if v, ok := step.Config["duration_ms"]; ok {
			if d, ok := v.(float64); ok {
				durationMs = int(d)
			}
		}
		select {
		case <-time.After(time.Duration(durationMs) * time.Millisecond):
		case <-stepCtx.Done():
			return "", stepCtx.Err()
		}
		completedAt := time.Now()
		stepRun.Status = StepCompleted
		stepRun.CompletedAt = &completedAt
		e.recordStepRun(ctx, stepRun)
		return step.Next, nil

	case StepTransform:
		output = e.executeTransform(step, run.Context)

	default:
		return "", fmt.Errorf("unknown step type: %s", step.Type)
	}

	completedAt := time.Now()
	if err != nil {
		stepRun.Status = StepFailed
		stepRun.Error = err.Error()
		stepRun.CompletedAt = &completedAt
		stepRun.DurationMs = int(time.Since(startedAt).Milliseconds())
		e.recordStepRun(ctx, stepRun)
		e.emitEvent(StepEvent{RunID: run.ID, StepID: step.ID, Event: "step_failed", Payload: err.Error()})
		return "", err
	}

	run.Context["last_output"] = output
	run.Context["step_"+step.ID+"_output"] = output
	run.TotalCostUUSD += costUUSD

	stepRun.Status = StepCompleted
	stepRun.Output = map[string]any{"content": output}
	stepRun.CostUUSD = costUUSD
	stepRun.DurationMs = int(time.Since(startedAt).Milliseconds())
	stepRun.CompletedAt = &completedAt
	e.recordStepRun(ctx, stepRun)
	e.emitEvent(StepEvent{RunID: run.ID, StepID: step.ID, Event: "step_completed", Payload: output})

	return step.Next, nil
}

func (e *Engine) executeParallel(ctx context.Context, step WorkflowStep, run *WorkflowRun) (string, int64, error) {
	if len(step.AgentKeys) == 0 {
		return "", 0, fmt.Errorf("parallel_agents step has no agent_keys")
	}

	type result struct {
		output string
		cost   int64
		err    error
	}
	results := make([]result, len(step.AgentKeys))
	var wg sync.WaitGroup

	for i, agentKey := range step.AgentKeys {
		wg.Add(1)
		go func(idx int, key string) {
			defer wg.Done()
			out, cost, err := e.invokeAgent(ctx, key, e.renderPrompt(step.Prompt, run.Context), run.Context)
			results[idx] = result{out, cost, err}
		}(i, agentKey)
	}
	wg.Wait()

	var combined []string
	var totalCost int64
	for _, r := range results {
		if r.err != nil {
			slog.Warn("workflow.parallel_step_error", "error", r.err)
			continue
		}
		combined = append(combined, r.output)
		totalCost += r.cost
	}

	return fmt.Sprintf("[%s]", joinJSON(combined)), totalCost, nil
}

func (e *Engine) evaluateCondition(step WorkflowStep, wfCtx map[string]any) string {
	condField, _ := step.Config["field"].(string)
	condValue, _ := step.Config["value"].(string)

	actual := fmt.Sprintf("%v", wfCtx[condField])
	if actual == condValue {
		if next, ok := step.Branches["true"]; ok {
			return next
		}
	}
	if next, ok := step.Branches["false"]; ok {
		return next
	}
	return step.Next
}

func (e *Engine) checkBudget(_ context.Context, run *WorkflowRun, step WorkflowStep) bool {
	maxCostUUSD := int64(0)
	if v, ok := step.Config["max_cost_uusd"]; ok {
		if d, ok := v.(float64); ok {
			maxCostUUSD = int64(d)
		}
	}
	if maxCostUUSD > 0 && run.TotalCostUUSD >= maxCostUUSD {
		return false
	}
	return true
}

func (e *Engine) executeTransform(step WorkflowStep, wfCtx map[string]any) string {
	// Simple key extraction transform
	if key, ok := step.Config["extract_key"].(string); ok {
		if val, exists := wfCtx[key]; exists {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}

func (e *Engine) shouldRetry(ctx context.Context, run *WorkflowRun, step WorkflowStep, _ error) bool {
	if step.RetryPolicy == nil {
		return false
	}
	// Check retry count from step runs
	var retryCount int
	e.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM workflow_step_runs WHERE run_id = $1 AND step_id = $2 AND status = 'failed'
	`, run.ID, step.ID).Scan(&retryCount)

	if retryCount >= step.RetryPolicy.MaxRetries {
		return false
	}

	backoff := time.Duration(step.RetryPolicy.BackoffMs) * time.Millisecond
	for i := 0; i < retryCount; i++ {
		backoff = time.Duration(float64(backoff) * step.RetryPolicy.BackoffFactor)
	}
	time.Sleep(backoff)
	return true
}

func (e *Engine) renderPrompt(template string, wfCtx map[string]any) string {
	result := template
	for k, v := range wfCtx {
		result = replaceAll(result, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}
	return result
}

func (e *Engine) completeRun(ctx context.Context, run *WorkflowRun, status RunStatus, errMsg string) {
	now := time.Now()
	run.Status = status
	run.CompletedAt = &now
	run.Error = errMsg

	if e.db != nil {
		ctxJSON, _ := json.Marshal(run.Context)
		e.db.Exec(ctx, `
			UPDATE workflow_runs SET status=$1, completed_at=$2, error=$3, total_cost_uusd=$4, context=$5
			WHERE id = $6
		`, run.Status, run.CompletedAt, run.Error, run.TotalCostUUSD, ctxJSON, run.ID)
	}

	e.emitEvent(StepEvent{RunID: run.ID, Event: "run_completed", Payload: map[string]any{"status": status, "error": errMsg}})
}

func (e *Engine) updateRunStatus(ctx context.Context, run *WorkflowRun) {
	if e.db == nil {
		return
	}
	ctxJSON, _ := json.Marshal(run.Context)
	e.db.Exec(ctx, `
		UPDATE workflow_runs SET current_step_id=$1, context=$2, total_cost_uusd=$3 WHERE id = $4
	`, run.CurrentStepID, ctxJSON, run.TotalCostUUSD, run.ID)
}

func (e *Engine) recordStepRun(ctx context.Context, sr *StepRun) {
	if e.db == nil {
		return
	}
	inputJSON, _ := json.Marshal(sr.Input)
	outputJSON, _ := json.Marshal(sr.Output)
	e.db.Exec(ctx, `
		INSERT INTO workflow_step_runs (id, run_id, step_id, agent_id, status, input, output, cost_uusd, quality_score, duration_ms, retry_count, started_at, completed_at, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			output = EXCLUDED.output,
			cost_uusd = EXCLUDED.cost_uusd,
			quality_score = EXCLUDED.quality_score,
			duration_ms = EXCLUDED.duration_ms,
			completed_at = EXCLUDED.completed_at,
			error = EXCLUDED.error
	`, sr.ID, sr.RunID, sr.StepID, sr.AgentID, sr.Status, inputJSON, outputJSON, sr.CostUUSD, sr.QualityScore, sr.DurationMs, sr.RetryCount, sr.StartedAt, sr.CompletedAt, sr.Error)
}

func (e *Engine) emitEvent(evt StepEvent) {
	if e.onStepEvent != nil {
		e.onStepEvent(evt)
	}
}

// GetRun retrieves a workflow run by ID.
func (e *Engine) GetRun(ctx context.Context, runID string) (*WorkflowRun, error) {
	if e.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	var run WorkflowRun
	var ctxJSON []byte
	err := e.db.QueryRow(ctx, `
		SELECT id, tenant_id, workflow_id, status, current_step_id, context, trigger_type, triggered_by, deadline, total_cost_uusd, started_at, completed_at, error
		FROM workflow_runs WHERE id = $1
	`, runID).Scan(&run.ID, &run.TenantID, &run.WorkflowID, &run.Status, &run.CurrentStepID, &ctxJSON, &run.TriggerType, &run.TriggeredBy, &run.Deadline, &run.TotalCostUUSD, &run.StartedAt, &run.CompletedAt, &run.Error)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(ctxJSON, &run.Context)
	return &run, nil
}

// ListRuns lists workflow runs for a tenant.
func (e *Engine) ListRuns(ctx context.Context, tenantID string, limit int) ([]WorkflowRun, error) {
	if e.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id, workflow_id, status, current_step_id, context, trigger_type, triggered_by, deadline, total_cost_uusd, started_at, completed_at, error
		FROM workflow_runs WHERE tenant_id = $1
		ORDER BY started_at DESC LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []WorkflowRun
	for rows.Next() {
		var r WorkflowRun
		var ctxJSON []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.WorkflowID, &r.Status, &r.CurrentStepID, &ctxJSON, &r.TriggerType, &r.TriggeredBy, &r.Deadline, &r.TotalCostUUSD, &r.StartedAt, &r.CompletedAt, &r.Error); err != nil {
			continue
		}
		json.Unmarshal(ctxJSON, &r.Context)
		runs = append(runs, r)
	}
	return runs, nil
}

// GetStepRuns returns all step runs for a workflow run.
func (e *Engine) GetStepRuns(ctx context.Context, runID string) ([]StepRun, error) {
	if e.db == nil {
		return nil, nil
	}
	rows, err := e.db.Query(ctx, `
		SELECT id, run_id, step_id, COALESCE(agent_id::text,''), status, input, output, cost_uusd, COALESCE(quality_score,0), duration_ms, retry_count, started_at, completed_at, error
		FROM workflow_step_runs WHERE run_id = $1
		ORDER BY started_at ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []StepRun
	for rows.Next() {
		var s StepRun
		var inputJSON, outputJSON []byte
		if err := rows.Scan(&s.ID, &s.RunID, &s.StepID, &s.AgentID, &s.Status, &inputJSON, &outputJSON, &s.CostUUSD, &s.QualityScore, &s.DurationMs, &s.RetryCount, &s.StartedAt, &s.CompletedAt, &s.Error); err != nil {
			continue
		}
		json.Unmarshal(inputJSON, &s.Input)
		json.Unmarshal(outputJSON, &s.Output)
		steps = append(steps, s)
	}
	return steps, nil
}

func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx < 0 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func joinJSON(items []string) string {
	if len(items) == 0 {
		return ""
	}
	result := ""
	for i, item := range items {
		if i > 0 {
			result += ","
		}
		result += fmt.Sprintf("%q", item)
	}
	return result
}
