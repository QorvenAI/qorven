// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SetMailRuleTool lets an agent propose an inbound routing rule.
// The rule is created with status='pending_confirmation'; a frontend card is shown
// to the user, who confirms or discards via POST /agents/:id/inbound-rules/:ruleId/confirm.
type SetMailRuleTool struct {
	pool *pgxpool.Pool
}

func NewSetMailRuleTool(pool *pgxpool.Pool) *SetMailRuleTool {
	return &SetMailRuleTool{pool: pool}
}

func (t *SetMailRuleTool) Name() string { return "set_mail_rule" }
func (t *SetMailRuleTool) Description() string {
	return "Propose a new inbound email routing rule. The rule is held as pending until the user confirms it in the chat UI. Use this when the user says 'always auto-reply to X' or 'never respond to newsletters'."
}
func (t *SetMailRuleTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"match_type": map[string]any{
				"type":        "string",
				"enum":        []string{"contact", "domain", "keyword", "default"},
				"description": "How to match incoming email: 'contact' = exact address, 'domain' = @domain.com, 'keyword' = subject/body keyword, 'default' = catch-all fallback.",
			},
			"match_value": map[string]any{
				"type":        "string",
				"description": "Value to match. e.g. 'sarah@acme.com', '@acme.com', 'unsubscribe'. Leave blank for match_type=default.",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"fully_autonomous", "draft_and_approve", "draft_only", "context_only", "drop"},
				"description": "Action to take: fully_autonomous=auto-reply, draft_and_approve=write draft+hold for approval, context_only=read and log only, drop=ignore completely.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Short plain-English explanation of why this rule is being created. Shown to the user in the confirmation card.",
			},
			"priority": map[string]any{
				"type":        "integer",
				"description": "Rule priority — lower numbers run first. Default 100.",
			},
		},
		"required": []string{"match_type", "mode", "reason"},
	}
}

func (t *SetMailRuleTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil {
		return ErrorResult("database not available")
	}
	agentID := AgentIDFromCtx(ctx)
	tenantID := TenantIDFromCtx(ctx)
	if agentID == "" {
		return ErrorResult("agent context required")
	}

	matchType, _ := args["match_type"].(string)
	matchValue, _ := args["match_value"].(string)
	mode, _ := args["mode"].(string)
	reason, _ := args["reason"].(string)
	priority := 100
	if p, ok := args["priority"].(float64); ok {
		priority = int(p)
	}

	if matchType == "" || mode == "" {
		return ErrorResult("match_type and mode are required")
	}

	var ruleID string
	err := t.pool.QueryRow(ctx,
		`INSERT INTO inbound_rules (tenant_id, agent_id, match_type, match_value, mode, reason, status, priority)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending_confirmation', $7)
		 RETURNING id`,
		tenantID, agentID, matchType, matchValue, mode, reason, priority,
	).Scan(&ruleID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create rule: %v", err))
	}

	return TextResult(fmt.Sprintf(
		`#!qorven:mail_rule_card
rule_id=%s
match_type=%s
match_value=%s
mode=%s
reason=%s`,
		ruleID, matchType, matchValue, mode, reason,
	))
}

// SetMailPolicyTool lets an agent draft a new natural-language mail policy.
// Like set_mail_rule it creates a pending item; the frontend shows a confirmation card.
type SetMailPolicyTool struct {
	pool *pgxpool.Pool
}

func NewSetMailPolicyTool(pool *pgxpool.Pool) *SetMailPolicyTool {
	return &SetMailPolicyTool{pool: pool}
}

func (t *SetMailPolicyTool) Name() string { return "set_mail_policy" }
func (t *SetMailPolicyTool) Description() string {
	return "Propose an update to this agent's natural-language mail policy. The draft is shown to the user for confirmation before it is saved."
}
func (t *SetMailPolicyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"policy": map[string]any{
				"type":        "string",
				"description": "The complete new mail policy in plain English. This replaces the existing policy when confirmed.",
			},
		},
		"required": []string{"policy"},
	}
}

func (t *SetMailPolicyTool) Execute(ctx context.Context, args map[string]any) *Result {
	policy, _ := args["policy"].(string)
	if policy == "" {
		return ErrorResult("policy text is required")
	}
	agentID := AgentIDFromCtx(ctx)
	if agentID == "" {
		return ErrorResult("agent context required")
	}

	return TextResult(fmt.Sprintf(
		`#!qorven:mail_policy_card
agent_id=%s
policy=%s`,
		agentID, policy,
	))
}
