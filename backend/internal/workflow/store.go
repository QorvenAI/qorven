// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Step types
const (
	StepPrompt    = "prompt"    // Send prompt to LLM
	StepTool      = "tool"      // Execute a tool
	StepCondition = "condition" // Branch based on LLM classification
	StepCollect   = "collect"   // Ask user for input
	StepAPI       = "api"       // Call external HTTP API
	StepDelegate  = "delegate"  // Delegate to another Soul
	StepNotify    = "notify"    // Send notification
	StepWait      = "wait"      // Pause for input or delay
)

// Trigger types
const (
	TriggerManual  = "manual"
	TriggerWebhook = "webhook"
	TriggerCron    = "cron"
	TriggerChannel = "channel_message"
	TriggerEvent   = "event"
)

type Workflow struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	AgentID       *string         `json:"agent_id,omitempty"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Steps         json.RawMessage `json:"steps"`
	Variables     json.RawMessage `json:"variables"`
	Enabled       bool            `json:"enabled"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Step struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Prompt   string         `json:"prompt,omitempty"`
	Tool     string         `json:"tool,omitempty"`
	Args     map[string]any `json:"args,omitempty"`
	Branches map[string]string `json:"branches,omitempty"` // for condition: value → step_id
	Fields   []string       `json:"fields,omitempty"`     // for collect
	Method   string         `json:"method,omitempty"`     // for api
	URL      string         `json:"url,omitempty"`        // for api
	Body     map[string]any `json:"body,omitempty"`       // for api
	SoulKey  string         `json:"soul_key,omitempty"`   // for delegate
	Task     string         `json:"task,omitempty"`       // for delegate
	SaveAs   string         `json:"save_as,omitempty"`    // store result in variable
	Next     string         `json:"next,omitempty"`       // next step ID (empty = end)
	Parallel []Step         `json:"parallel,omitempty"`   // sub-steps to run concurrently
}

type Run struct {
	ID          string          `json:"id"`
	WorkflowID  string          `json:"workflow_id"`
	TenantID    string          `json:"tenant_id"`
	AgentID     *string         `json:"agent_id,omitempty"`
	Status      string          `json:"status"`
	CurrentStep int             `json:"current_step"`
	Context     json.RawMessage `json:"context"`
	Result      string          `json:"result"`
	StartedAt   time.Time       `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// Store handles workflow CRUD.
type Store struct {
	pool *pgxpool.Pool
	// In-memory fallback for testing (when pool is nil)
	memRuns map[string]*Run
	memSeq  int
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool, memRuns: make(map[string]*Run)} }

func (s *Store) Create(ctx context.Context, tenantID string, w Workflow) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO workflows (tenant_id, name, description, agent_id, trigger_type, trigger_config, steps, variables, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		tenantID, w.Name, w.Description, w.AgentID, w.TriggerType, w.TriggerConfig, w.Steps, w.Variables, w.Enabled,
	).Scan(&id)
	return id, err
}

func (s *Store) Get(ctx context.Context, id string) (*Workflow, error) {
	w := &Workflow{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, description, agent_id, trigger_type, trigger_config, steps, variables, enabled, created_at, updated_at
		 FROM workflows WHERE id = $1`, id,
	).Scan(&w.ID, &w.TenantID, &w.Name, &w.Description, &w.AgentID, &w.TriggerType, &w.TriggerConfig, &w.Steps, &w.Variables, &w.Enabled, &w.CreatedAt, &w.UpdatedAt)
	if err != nil { return nil, err }
	return w, nil
}

func (s *Store) List(ctx context.Context, tenantID string) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, description, agent_id, trigger_type, trigger_config, steps, variables, enabled, created_at, updated_at
		 FROM workflows WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	wfs := []Workflow{}
	for rows.Next() {
		var w Workflow
		rows.Scan(&w.ID, &w.TenantID, &w.Name, &w.Description, &w.AgentID, &w.TriggerType, &w.TriggerConfig, &w.Steps, &w.Variables, &w.Enabled, &w.CreatedAt, &w.UpdatedAt)
		wfs = append(wfs, w)
	}
	return wfs, nil
}

func (s *Store) Update(ctx context.Context, id string, w Workflow) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE workflows SET name=$2, description=$3, agent_id=$4, trigger_type=$5, trigger_config=$6, steps=$7, variables=$8, enabled=$9, updated_at=NOW()
		 WHERE id=$1`,
		id, w.Name, w.Description, w.AgentID, w.TriggerType, w.TriggerConfig, w.Steps, w.Variables, w.Enabled)
	return err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM workflows WHERE id = $1`, id)
	return err
}

// Run management
func (s *Store) CreateRun(ctx context.Context, tenantID, workflowID string, agentID *string) (string, error) {
	if s.pool == nil {
		s.memSeq++
		id := fmt.Sprintf("run-%d", s.memSeq)
		s.memRuns[id] = &Run{ID: id, WorkflowID: workflowID, TenantID: tenantID, AgentID: agentID, Status: "pending", StartedAt: time.Now()}
		return id, nil
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO workflow_runs (tenant_id, workflow_id, agent_id, context) VALUES ($1, $2, $3, '{}') RETURNING id`,
		tenantID, workflowID, agentID,
	).Scan(&id)
	return id, err
}

func (s *Store) GetRun(ctx context.Context, id string) (*Run, error) {
	if s.pool == nil {
		if r, ok := s.memRuns[id]; ok {
			cp := *r
			return &cp, nil
		}
		return nil, fmt.Errorf("run not found: %s", id)
	}
	r := &Run{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, workflow_id, tenant_id, agent_id, status, current_step, context, result, started_at, completed_at, error
		 FROM workflow_runs WHERE id = $1`, id,
	).Scan(&r.ID, &r.WorkflowID, &r.TenantID, &r.AgentID, &r.Status, &r.CurrentStep, &r.Context, &r.Result, &r.StartedAt, &r.CompletedAt, &r.Error)
	if err != nil { return nil, err }
	return r, nil
}

func (s *Store) UpdateRun(ctx context.Context, id string, status string, step int, runCtx json.RawMessage, result, errMsg string) error {
	if s.pool == nil {
		if r, ok := s.memRuns[id]; ok {
			r.Status = status
			r.CurrentStep = step
			r.Context = runCtx
			r.Result = result
			r.Error = errMsg
			if status == "completed" || status == "failed" {
				now := time.Now()
				r.CompletedAt = &now
			}
		}
		return nil
	}
	if status == "completed" || status == "failed" {
		_, err := s.pool.Exec(ctx,
			`UPDATE workflow_runs SET status=$2, current_step=$3, context=$4, result=$5, error=$6, completed_at=NOW() WHERE id=$1`,
			id, status, step, runCtx, result, errMsg)
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE workflow_runs SET status=$2, current_step=$3, context=$4, result=$5, error=$6 WHERE id=$1`,
		id, status, step, runCtx, result, errMsg)
	return err
}

func (s *Store) ListRuns(ctx context.Context, workflowID string, limit int) ([]Run, error) {
	if limit <= 0 { limit = 20 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, workflow_id, tenant_id, agent_id, status, current_step, context, result, started_at, completed_at, error
		 FROM workflow_runs WHERE workflow_id = $1 ORDER BY started_at DESC LIMIT $2`, workflowID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var r Run
		rows.Scan(&r.ID, &r.WorkflowID, &r.TenantID, &r.AgentID, &r.Status, &r.CurrentStep, &r.Context, &r.Result, &r.StartedAt, &r.CompletedAt, &r.Error)
		runs = append(runs, r)
	}
	return runs, nil
}
