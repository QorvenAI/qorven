// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/tools"
)

// ── ask_followup_question ─────────────────────────────────────────────────────

// AskFollowupTool surfaces a single focused question to the user during the
// Prime intake conversation. The result is shown directly in the chat UI;
// the LLM receives an acknowledgment so it knows the question was delivered.
type AskFollowupTool struct{}

func NewAskFollowupTool() *AskFollowupTool { return &AskFollowupTool{} }

func (t *AskFollowupTool) Name() string { return "ask_followup_question" }
func (t *AskFollowupTool) Description() string {
	return "Ask the user a focused clarifying question during the project intake conversation. Use this to gather missing details about their idea, stack, timeline, budget, or quality bar before producing the brief."
}
func (t *AskFollowupTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The specific question to ask the user",
			},
			"topic": map[string]any{
				"type":        "string",
				"description": "Short label for this question's topic (e.g. 'stack', 'budget', 'timeline')",
			},
		},
		"required": []string{"question"},
	}
}

func (t *AskFollowupTool) Execute(_ context.Context, args map[string]any) *tools.Result {
	question, _ := args["question"].(string)
	if question == "" {
		return tools.ErrorResult("question is required")
	}
	return &tools.Result{
		ForLLM:  "QUESTION_SENT: " + question,
		ForUser: question,
	}
}

// ── produce_project_brief ─────────────────────────────────────────────────────

// ProduceProjectBriefTool persists a structured project brief to the database
// once Prime has gathered enough information. It writes to the project_briefs
// table and returns the newly created brief ID so the caller can link to it.
type ProduceProjectBriefTool struct {
	db       *pgxpool.Pool
	tenantID string
}

func NewProduceProjectBriefTool(db *pgxpool.Pool, tenantID string) *ProduceProjectBriefTool {
	return &ProduceProjectBriefTool{db: db, tenantID: tenantID}
}

func (t *ProduceProjectBriefTool) Name() string { return "produce_project_brief" }
func (t *ProduceProjectBriefTool) Description() string {
	return "Produce and persist a structured project brief once you have enough information from the user. Creates a brief record that can be reviewed, team-proposed, and approved to spawn agents and tickets."
}
func (t *ProduceProjectBriefTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{
				"type":        "string",
				"description": "Short, descriptive project title (e.g. 'Customer Portal Redesign')",
			},
			"idea": map[string]any{
				"type":        "string",
				"description": "Full description of what needs to be built — goals, key features, and any constraints",
			},
			"stack": map[string]any{
				"type":        "string",
				"description": "Technology stack (e.g. 'Next.js + Go + PostgreSQL'). Leave blank if not specified.",
			},
			"budget_cents": map[string]any{
				"type":        "integer",
				"description": "Approximate budget in USD cents (e.g. 50000 = $500). Use 0 if unknown.",
			},
			"timeline": map[string]any{
				"type":        "string",
				"description": "Delivery urgency: 'asap' | 'this_week' | 'this_month' | 'no_rush'",
				"enum":        []string{"asap", "this_week", "this_month", "no_rush"},
			},
			"quality": map[string]any{
				"type":        "string",
				"description": "Quality tier: 'mvp' (fast/minimal) | 'production' (solid) | 'enterprise' (high-spec)",
				"enum":        []string{"mvp", "production", "enterprise"},
			},
			"status": map[string]any{
				"type":        "string",
				"description": "Use 'proposed' when you have all required information and the brief is complete. Use 'draft' when you still need clarification.",
				"enum":        []string{"proposed", "draft"},
			},
			"open_questions": map[string]any{
				"type":        "array",
				"description": "List of questions still unanswered that would improve the brief. Leave empty when status is 'proposed'.",
				"items":       map[string]any{"type": "string"},
			},
		},
		"required": []string{"title", "idea"},
	}
}

func (t *ProduceProjectBriefTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	title, _ := args["title"].(string)
	idea, _ := args["idea"].(string)
	if title == "" || idea == "" {
		return tools.ErrorResult("title and idea are required")
	}

	stack, _ := args["stack"].(string)
	timeline, _ := args["timeline"].(string)
	if timeline == "" {
		timeline = "no_rush"
	}
	quality, _ := args["quality"].(string)
	if quality == "" {
		quality = "mvp"
	}

	var budgetCents int
	switch v := args["budget_cents"].(type) {
	case float64:
		budgetCents = int(v)
	case int:
		budgetCents = v
	case json.Number:
		n, _ := v.Int64()
		budgetCents = int(n)
	}

	if t.db == nil {
		return tools.ErrorResult("database unavailable — brief not persisted")
	}

	var briefID string
	err := t.db.QueryRow(ctx,
		`INSERT INTO project_briefs
		   (tenant_id, title, idea, stack, budget_cents, timeline, quality)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id`,
		t.tenantID, title, idea, stack, budgetCents, timeline, quality,
	).Scan(&briefID)
	if err != nil {
		return tools.ErrorResult("failed to save brief: " + err.Error())
	}

	summary := fmt.Sprintf(
		"Project brief saved.\n\nTitle: %s\nTimeline: %s\nQuality: %s\n\nBrief ID: %s\n\nThe brief is ready for team proposal. Visit the Projects page to review it.",
		title, timeline, quality, briefID,
	)
	return &tools.Result{
		ForLLM:  fmt.Sprintf(`{"brief_id":%q,"status":"created","title":%q}`, briefID, title),
		ForUser: summary,
	}
}
