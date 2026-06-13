// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Policy struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Category       string         `json:"category"`
	TriggerEvent   string         `json:"trigger_event"`
	Conditions     []PolicyCond   `json:"conditions"`
	Action         string         `json:"action"`
	ActionParams   map[string]any `json:"action_params"`
	AppliesToRoles []string       `json:"applies_to_roles"`
	AppliesToLevels []int         `json:"applies_to_levels"`
	Priority       int            `json:"priority"`
	Enabled        bool           `json:"enabled"`
}

type PolicyCond struct {
	Field    string `json:"field"`
	Operator string `json:"operator"` // equals, not_equals, gt, lt, gte, lte, contains, in
	Value    string `json:"value"`
}

type PolicyDecision struct {
	Allowed    bool   `json:"allowed"`
	Action     string `json:"action"`      // allow, deny, require_approval, warn, log, throttle, escalate
	PolicyID   string `json:"policy_id"`
	PolicyName string `json:"policy_name"`
	Message    string `json:"message,omitempty"`
}

type PolicyEvent struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	PolicyID     string         `json:"policy_id"`
	PolicyName   string         `json:"policy_name"`
	AgentID      string         `json:"agent_id"`
	AgentKey     string         `json:"agent_key"`
	TriggerEvent string         `json:"trigger_event"`
	ActionTaken  string         `json:"action_taken"`
	Context      map[string]any `json:"context"`
	CreatedAt    time.Time      `json:"created_at"`
}

type PolicyEngine struct {
	db       *pgxpool.Pool
	mu       sync.RWMutex
	policies []Policy
}

func NewPolicyEngine(db *pgxpool.Pool) *PolicyEngine {
	return &PolicyEngine{db: db}
}

func (e *PolicyEngine) LoadPolicies(ctx context.Context, tenantID string) error {
	if e.db == nil {
		return nil
	}
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), category, trigger_event,
		       COALESCE(conditions,'[]'), action, COALESCE(action_params,'{}'),
		       COALESCE(applies_to_roles,'{}'), COALESCE(applies_to_levels,'{}'), priority, enabled
		FROM policies WHERE tenant_id = $1 AND enabled = true
		ORDER BY priority ASC
	`, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		var condJSON, paramsJSON []byte
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.Category, &p.TriggerEvent,
			&condJSON, &p.Action, &paramsJSON, &p.AppliesToRoles, &p.AppliesToLevels, &p.Priority, &p.Enabled); err != nil {
			continue
		}
		json.Unmarshal(condJSON, &p.Conditions)
		json.Unmarshal(paramsJSON, &p.ActionParams)
		policies = append(policies, p)
	}
	e.mu.Lock()
	e.policies = policies
	e.mu.Unlock()
	return nil
}

// Evaluate checks all policies against the given event context.
// Returns the first matching policy decision (deny/require_approval/warn) or allow.
func (e *PolicyEngine) Evaluate(ctx context.Context, triggerEvent, agentKey string, orgLevel int, eventCtx map[string]any) PolicyDecision {
	e.mu.RLock()
	ps := e.policies
	e.mu.RUnlock()
	for _, p := range ps {
		if p.TriggerEvent != triggerEvent {
			continue
		}
		if len(p.AppliesToRoles) > 0 && !containsStr(p.AppliesToRoles, agentKey) {
			continue
		}
		if len(p.AppliesToLevels) > 0 && !containsInt(p.AppliesToLevels, orgLevel) {
			continue
		}
		if !e.matchConditions(p.Conditions, eventCtx) {
			continue
		}
		// Policy matched — record and return decision
		go e.recordEvent(ctx, p, agentKey, eventCtx)

		msg := ""
		if m, ok := p.ActionParams["message"]; ok {
			msg, _ = m.(string)
		}

		return PolicyDecision{
			Allowed:    p.Action == "allow" || p.Action == "log" || p.Action == "warn",
			Action:     p.Action,
			PolicyID:   p.ID,
			PolicyName: p.Name,
			Message:    msg,
		}
	}
	return PolicyDecision{Allowed: true, Action: "allow"}
}

func (e *PolicyEngine) matchConditions(conds []PolicyCond, ctx map[string]any) bool {
	for _, c := range conds {
		val, ok := ctx[c.Field]
		if !ok {
			return false
		}
		valStr := toString(val)
		switch c.Operator {
		case "equals":
			if valStr != c.Value {
				return false
			}
		case "not_equals":
			if valStr == c.Value {
				return false
			}
		case "contains":
			if !strings.Contains(valStr, c.Value) {
				return false
			}
		case "gt":
			if toFloat(val) <= toFloat(c.Value) {
				return false
			}
		case "gte":
			if toFloat(val) < toFloat(c.Value) {
				return false
			}
		case "lt":
			if toFloat(val) >= toFloat(c.Value) {
				return false
			}
		case "lte":
			if toFloat(val) > toFloat(c.Value) {
				return false
			}
		default:
			// Unknown operator: fail closed — the condition does not match.
			return false
		}
	}
	return true
}

func (e *PolicyEngine) recordEvent(ctx context.Context, p Policy, agentKey string, evtCtx map[string]any) {
	if e.db == nil {
		return
	}
	ctxJSON, _ := json.Marshal(evtCtx)
	_, err := e.db.Exec(ctx, `
		INSERT INTO policy_events (tenant_id, policy_id, policy_name, agent_key, trigger_event, action_taken, context)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, p.TenantID, p.ID, p.Name, agentKey, p.TriggerEvent, p.Action, ctxJSON)
	if err != nil {
		slog.Warn("policy.event.record_failed", "policy", p.Name, "error", err)
	}
}

// HasBlockingOutputPolicy reports whether the engine has at least one enabled
// output_deliver policy whose action would block or suppress output
// (deny or require_approval). This is queried once per agent turn so the loop
// can decide whether to buffer live text rather than streaming it directly.
func (e *PolicyEngine) HasBlockingOutputPolicy(tenantID string) bool {
	e.mu.RLock()
	ps := e.policies
	e.mu.RUnlock()
	for _, p := range ps {
		if !p.Enabled {
			continue
		}
		if p.TenantID != tenantID {
			continue
		}
		if p.TriggerEvent != "output_deliver" {
			continue
		}
		if p.Action == "deny" || p.Action == "require_approval" {
			return true
		}
	}
	return false
}

// ValidPolicyAction reports whether a is a recognized policy action.
func ValidPolicyAction(a string) bool {
	switch a {
	case "allow", "deny", "warn", "require_approval", "throttle", "log", "escalate":
		return true
	}
	return false
}

// ValidTriggerEvent reports whether t is a recognized policy trigger event.
func ValidTriggerEvent(t string) bool {
	switch t {
	case "tool_call", "model_switch", "output_deliver",
		"memory_write", "agent_spawn", "budget_spend",
		"external_action", "build_approve":
		return true
	}
	return false
}

// ListPolicies returns all policy definitions for the tenant, ordered by priority.
func (e *PolicyEngine) ListPolicies(ctx context.Context, tenantID string) ([]Policy, error) {
	if e.db == nil {
		return nil, nil
	}
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), category, trigger_event,
		       COALESCE(conditions,'[]'), action, COALESCE(action_params,'{}'),
		       COALESCE(applies_to_roles,'{}'), COALESCE(applies_to_levels,'{}'), priority, enabled
		FROM policies WHERE tenant_id = $1
		ORDER BY priority ASC, created_at ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Policy
	for rows.Next() {
		var p Policy
		var condJSON, paramsJSON []byte
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.Category, &p.TriggerEvent,
			&condJSON, &p.Action, &paramsJSON, &p.AppliesToRoles, &p.AppliesToLevels, &p.Priority, &p.Enabled); err != nil {
			continue
		}
		json.Unmarshal(condJSON, &p.Conditions)
		json.Unmarshal(paramsJSON, &p.ActionParams)
		out = append(out, p)
	}
	return out, nil
}

// CreatePolicy inserts a new policy and returns its generated UUID.
func (e *PolicyEngine) CreatePolicy(ctx context.Context, p Policy) (string, error) {
	condJSON, err := json.Marshal(p.Conditions)
	if err != nil {
		return "", err
	}
	paramsJSON, err := json.Marshal(p.ActionParams)
	if err != nil {
		return "", err
	}
	if p.Conditions == nil {
		condJSON = []byte("[]")
	}
	if p.ActionParams == nil {
		paramsJSON = []byte("{}")
	}
	roles := p.AppliesToRoles
	if roles == nil {
		roles = []string{}
	}
	levels := p.AppliesToLevels
	if levels == nil {
		levels = []int{}
	}
	var id string
	err = e.db.QueryRow(ctx, `
		INSERT INTO policies
		  (tenant_id, name, description, category, trigger_event, conditions, action,
		   action_params, applies_to_roles, applies_to_levels, priority, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id
	`, p.TenantID, p.Name, p.Description, p.Category, p.TriggerEvent,
		condJSON, p.Action, paramsJSON, roles, levels, p.Priority, p.Enabled,
	).Scan(&id)
	return id, err
}

// UpdatePolicy replaces all mutable fields of an existing policy row.
// Returns an error wrapping "not found" when no row matched (id + tenant_id).
func (e *PolicyEngine) UpdatePolicy(ctx context.Context, p Policy) error {
	condJSON, err := json.Marshal(p.Conditions)
	if err != nil {
		return err
	}
	paramsJSON, err := json.Marshal(p.ActionParams)
	if err != nil {
		return err
	}
	if p.Conditions == nil {
		condJSON = []byte("[]")
	}
	if p.ActionParams == nil {
		paramsJSON = []byte("{}")
	}
	roles := p.AppliesToRoles
	if roles == nil {
		roles = []string{}
	}
	levels := p.AppliesToLevels
	if levels == nil {
		levels = []int{}
	}
	tag, err := e.db.Exec(ctx, `
		UPDATE policies SET
		  name=$1, description=$2, category=$3, trigger_event=$4, conditions=$5,
		  action=$6, action_params=$7, applies_to_roles=$8, applies_to_levels=$9,
		  priority=$10, enabled=$11, updated_at=now()
		WHERE id=$12 AND tenant_id=$13
	`, p.Name, p.Description, p.Category, p.TriggerEvent, condJSON,
		p.Action, paramsJSON, roles, levels, p.Priority, p.Enabled,
		p.ID, p.TenantID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errPolicyNotFound
	}
	return nil
}

// DeletePolicy removes a policy by id scoped to the tenant.
// Returns errPolicyNotFound when no row matched.
func (e *PolicyEngine) DeletePolicy(ctx context.Context, tenantID, id string) error {
	tag, err := e.db.Exec(ctx, `DELETE FROM policies WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errPolicyNotFound
	}
	return nil
}

// sentinel so callers can test for not-found without importing errors strings
var errPolicyNotFound = fmt.Errorf("policy not found")

func (e *PolicyEngine) ListEvents(ctx context.Context, tenantID string, limit int) ([]PolicyEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id, COALESCE(policy_id::text,''), COALESCE(policy_name,''),
		       COALESCE(agent_id::text,''), COALESCE(agent_key,''), COALESCE(trigger_event,''),
		       COALESCE(action_taken,''), COALESCE(context,'{}'), created_at
		FROM policy_events WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PolicyEvent
	for rows.Next() {
		var pe PolicyEvent
		if err := rows.Scan(&pe.ID, &pe.TenantID, &pe.PolicyID, &pe.PolicyName, &pe.AgentID,
			&pe.AgentKey, &pe.TriggerEvent, &pe.ActionTaken, &pe.Context, &pe.CreatedAt); err != nil {
			continue
		}
		out = append(out, pe)
	}
	return out, nil
}
