// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SetRuleTool lets Prime create background rules that Qorven enforces autonomously.
// Each rule maps a trigger (cron/threshold/event) to an action (run_tool/escalate/notify).
type SetRuleTool struct {
	DB       *pgxpool.Pool
	TenantID string
}

func NewSetRuleTool(db *pgxpool.Pool, tenantID string) *SetRuleTool {
	return &SetRuleTool{DB: db, TenantID: tenantID}
}

func (t *SetRuleTool) Name() string { return "set_rule" }

func (t *SetRuleTool) Description() string {
	return "Create a background rule that Qorven enforces autonomously. " +
		"Use this when the user states a policy: 'alert me if X happens', 'run Y every Sunday', " +
		"'if Z crosses threshold, notify me'. Rules are stored permanently and survive restarts."
}

func (t *SetRuleTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "Human-readable description shown in the rules list. E.g. 'Alert when any PC offline >2min'",
			},
			"trigger_type": map[string]any{
				"type":        "string",
				"enum":        []string{"cron", "threshold", "event"},
				"description": "cron=scheduled (e.g. every Sunday 2am), threshold=monitor a value and fire when crossed, event=react to a named system event",
			},
			"trigger_spec": map[string]any{
				"type":        "object",
				"description": `For cron: {"cron": "0 2 * * 0"}. For threshold: {"tool": "pc_ping_check", "field": "offline_count", "op": ">", "value": 0}. For event: {"event": "task_failed"}`,
			},
			"action_type": map[string]any{
				"type":        "string",
				"enum":        []string{"run_tool", "escalate", "notify"},
				"description": "run_tool=call a connector tool, escalate=interrupt user immediately, notify=send a non-blocking message",
			},
			"action_spec": map[string]any{
				"type":        "object",
				"description": `For run_tool: {"tool": "antivirus_push", "args": {...}}. For escalate/notify: {"message": "2 PCs are offline"}`,
			},
		},
		"required": []string{"description", "trigger_type", "trigger_spec", "action_type", "action_spec"},
	}
}

func (t *SetRuleTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.DB == nil {
		return ErrorResult("database unavailable — cannot create rule")
	}

	description, _ := args["description"].(string)
	triggerType, _ := args["trigger_type"].(string)
	actionType, _ := args["action_type"].(string)

	if description == "" || triggerType == "" || actionType == "" {
		return ErrorResult("description, trigger_type, and action_type are required")
	}

	triggerSpec, err := json.Marshal(args["trigger_spec"])
	if err != nil {
		return ErrorResult("invalid trigger_spec: " + err.Error())
	}
	actionSpec, err := json.Marshal(args["action_spec"])
	if err != nil {
		return ErrorResult("invalid action_spec: " + err.Error())
	}

	agentID := AgentIDFromCtx(ctx)

	var ruleID string
	err = t.DB.QueryRow(ctx,
		`INSERT INTO agent_rules (tenant_id, agent_id, description, trigger_type, trigger_spec, action_type, action_spec, enabled, created_at)
		 VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, true, $8)
		 RETURNING id`,
		t.TenantID, agentID, description, triggerType,
		string(triggerSpec), actionType, string(actionSpec),
		time.Now(),
	).Scan(&ruleID)
	if err != nil {
		return ErrorResult("failed to store rule: " + truncRuleErr(err))
	}

	if triggerType == "cron" {
		ts := map[string]any{}
		if jErr := json.Unmarshal(triggerSpec, &ts); jErr == nil {
			if cronExpr, ok := ts["cron"].(string); ok && cronExpr != "" {
				slog.Info("set_rule.cron_scheduled",
					"rule_id", ruleID, "cron", cronExpr, "description", description)
			}
		}
	}

	slog.Info("set_rule.created",
		"rule_id", ruleID, "trigger", triggerType, "action", actionType, "description", description)

	return TextResult(fmt.Sprintf("Rule created (id: %s). Description: %s. Trigger: %s. Action: %s.",
		ruleID, description, triggerType, actionType))
}

func truncRuleErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 120 {
		return msg[:120] + "…"
	}
	return msg
}
