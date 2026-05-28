// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RoutingCondition struct {
	Field    string `json:"field"`    // channel, keyword, user_role, time_of_day, agent_state, content_type
	Operator string `json:"operator"` // equals, contains, matches_regex, in, not_equals
	Value    string `json:"value"`
}

type RoutingAction struct {
	AgentID   string  `json:"agent_id"`
	AgentKey  string  `json:"agent_key"`
	Priority  int     `json:"priority"`    // 0=interactive, 1=background, 2=batch
	MaxBudget float64 `json:"max_budget"`  // per-request cap USD
	SLAMS     int     `json:"sla_ms"`      // max response time
	Fallback  string  `json:"fallback"`    // fallback agent key
}

type RoutingRule struct {
	ID         string             `json:"id"`
	TenantID   string             `json:"tenant_id"`
	Name       string             `json:"name"`
	Priority   int                `json:"priority"`
	Conditions []RoutingCondition `json:"conditions"`
	Action     RoutingAction      `json:"action"`
	Enabled    bool               `json:"enabled"`
}

type RoutingDecision struct {
	AgentKey string `json:"agent_key"`
	AgentID  string `json:"agent_id"`
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Method   string `json:"method"` // deterministic, skill_based, llm_classification, default
	Priority int    `json:"priority"`
	SLAMS    int    `json:"sla_ms"`
	Fallback string `json:"fallback"`
}

type RoutingContext struct {
	Channel     string
	Content     string
	UserRole    string
	AgentStates map[string]string // agentKey → "idle"|"running"|"offline"
}

type IntentRouter struct {
	db    *pgxpool.Pool
	rules []RoutingRule
	seeds map[string]AgentSeed
}

func NewIntentRouter(db *pgxpool.Pool, seeds map[string]AgentSeed) *IntentRouter {
	return &IntentRouter{
		db:    db,
		seeds: seeds,
	}
}

func (ir *IntentRouter) LoadRules(ctx context.Context, tenantID string) error {
	if ir.db == nil {
		return nil
	}
	rows, err := ir.db.Query(ctx, `
		SELECT id, tenant_id, name, priority, conditions, action, enabled
		FROM routing_rules
		WHERE tenant_id = $1 AND enabled = true
		ORDER BY priority ASC
	`, tenantID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var rules []RoutingRule
	for rows.Next() {
		var r RoutingRule
		var condJSON, actionJSON []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Priority, &condJSON, &actionJSON, &r.Enabled); err != nil {
			continue
		}
		json.Unmarshal(condJSON, &r.Conditions)
		json.Unmarshal(actionJSON, &r.Action)
		rules = append(rules, r)
	}
	ir.rules = rules
	return nil
}

// Route evaluates rules against the routing context and returns a decision.
// Priority: deterministic rules → skill-based → default agent.
func (ir *IntentRouter) Route(ctx context.Context, rctx RoutingContext) RoutingDecision {
	// 1. Deterministic rules (checked in priority order)
	for _, rule := range ir.rules {
		if ir.matchesAllConditions(rule.Conditions, rctx) {
			return RoutingDecision{
				AgentKey: rule.Action.AgentKey,
				AgentID:  rule.Action.AgentID,
				RuleID:   rule.ID,
				RuleName: rule.Name,
				Method:   "deterministic",
				Priority: rule.Action.Priority,
				SLAMS:    rule.Action.SLAMS,
				Fallback: rule.Action.Fallback,
			}
		}
	}

	// 2. Skill-based routing (match required tools to agent capabilities)
	if agentKey := ir.matchBySkill(rctx.Content); agentKey != "" {
		return RoutingDecision{
			AgentKey: agentKey,
			Method:   "skill_based",
			Priority: 1,
		}
	}

	// 3. Default: route to chief
	return RoutingDecision{
		AgentKey: "chief",
		Method:   "default",
		Priority: 1,
	}
}

func (ir *IntentRouter) matchesAllConditions(conditions []RoutingCondition, rctx RoutingContext) bool {
	for _, cond := range conditions {
		if !ir.matchCondition(cond, rctx) {
			return false
		}
	}
	return len(conditions) > 0
}

func (ir *IntentRouter) matchCondition(cond RoutingCondition, rctx RoutingContext) bool {
	fieldValue := ir.getFieldValue(cond.Field, rctx)

	switch cond.Operator {
	case "equals":
		return strings.EqualFold(fieldValue, cond.Value)
	case "not_equals":
		return !strings.EqualFold(fieldValue, cond.Value)
	case "contains":
		return strings.Contains(strings.ToLower(fieldValue), strings.ToLower(cond.Value))
	case "matches_regex":
		re, err := regexp.Compile(cond.Value)
		if err != nil {
			return false
		}
		return re.MatchString(fieldValue)
	case "in":
		values := strings.Split(cond.Value, ",")
		for _, v := range values {
			if strings.EqualFold(strings.TrimSpace(v), fieldValue) {
				return true
			}
		}
		return false
	}
	return false
}

func (ir *IntentRouter) getFieldValue(field string, rctx RoutingContext) string {
	switch field {
	case "channel":
		return rctx.Channel
	case "content", "keyword":
		return rctx.Content
	case "user_role":
		return rctx.UserRole
	case "time_of_day":
		h := time.Now().Hour()
		if h >= 6 && h < 12 {
			return "morning"
		} else if h >= 12 && h < 18 {
			return "afternoon"
		} else if h >= 18 && h < 22 {
			return "evening"
		}
		return "night"
	}
	return ""
}

// matchBySkill identifies the best agent based on content keywords matching tool capabilities.
func (ir *IntentRouter) matchBySkill(content string) string {
	lower := strings.ToLower(content)

	skillKeywords := map[string][]string{
		"code":     {"code", "bug", "function", "implement", "refactor", "test", "commit", "pull request", "pr"},
		"devops":   {"deploy", "ci/cd", "docker", "kubernetes", "infrastructure", "server", "pipeline"},
		"writer":   {"blog", "article", "write", "content", "copy", "draft", "publish"},
		"marketer": {"campaign", "seo", "social media", "ads", "marketing", "growth"},
		"designer": {"design", "wireframe", "mockup", "ui", "ux", "layout", "figma"},
		"analyst":  {"data", "chart", "metric", "analytics", "dashboard", "report", "sql"},
		"legal":    {"contract", "compliance", "regulation", "privacy", "gdpr", "terms"},
		"sales":    {"lead", "prospect", "deal", "pipeline", "crm", "proposal", "pitch"},
		"support":  {"ticket", "customer", "issue", "bug report", "help", "complaint"},
	}

	bestMatch := ""
	bestScore := 0

	for agentKey, keywords := range skillKeywords {
		score := 0
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestMatch = agentKey
		}
	}

	if bestScore >= 2 {
		return bestMatch
	}
	return ""
}

// CRUD for routing rules

func (ir *IntentRouter) CreateRule(ctx context.Context, rule RoutingRule) error {
	if ir.db == nil {
		return nil
	}
	condJSON, _ := json.Marshal(rule.Conditions)
	actionJSON, _ := json.Marshal(rule.Action)
	_, err := ir.db.Exec(ctx, `
		INSERT INTO routing_rules (tenant_id, name, priority, conditions, action, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, rule.TenantID, rule.Name, rule.Priority, condJSON, actionJSON, rule.Enabled)
	return err
}

func (ir *IntentRouter) UpdateRule(ctx context.Context, rule RoutingRule) error {
	if ir.db == nil {
		return nil
	}
	condJSON, _ := json.Marshal(rule.Conditions)
	actionJSON, _ := json.Marshal(rule.Action)
	_, err := ir.db.Exec(ctx, `
		UPDATE routing_rules SET name=$1, priority=$2, conditions=$3, action=$4, enabled=$5
		WHERE id = $6 AND tenant_id = $7
	`, rule.Name, rule.Priority, condJSON, actionJSON, rule.Enabled, rule.ID, rule.TenantID)
	return err
}

func (ir *IntentRouter) DeleteRule(ctx context.Context, tenantID, ruleID string) error {
	if ir.db == nil {
		return nil
	}
	_, err := ir.db.Exec(ctx, `DELETE FROM routing_rules WHERE id = $1 AND tenant_id = $2`, ruleID, tenantID)
	return err
}

func (ir *IntentRouter) ListRules(ctx context.Context, tenantID string) ([]RoutingRule, error) {
	if ir.db == nil {
		return nil, nil
	}
	rows, err := ir.db.Query(ctx, `
		SELECT id, tenant_id, name, priority, conditions, action, enabled
		FROM routing_rules
		WHERE tenant_id = $1
		ORDER BY priority ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RoutingRule
	for rows.Next() {
		var r RoutingRule
		var condJSON, actionJSON []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Priority, &condJSON, &actionJSON, &r.Enabled); err != nil {
			continue
		}
		json.Unmarshal(condJSON, &r.Conditions)
		json.Unmarshal(actionJSON, &r.Action)
		rules = append(rules, r)
	}
	return rules, nil
}
