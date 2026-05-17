// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	daemonPkg "github.com/qorvenai/qorven/internal/daemon"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/realtime"
)

// ProjectBrief is the structured outcome of the intake conversation.
type ProjectBrief struct {
	ID          string        `json:"id"`
	TenantID    string        `json:"tenant_id"`
	Title       string        `json:"title"`
	Idea        string        `json:"idea"`
	Stack       string        `json:"stack"`
	BudgetCents int           `json:"budget_cents"`
	Timeline    string        `json:"timeline"`
	Quality     string        `json:"quality"`
	Status      string        `json:"status"`
	Proposal    *TeamProposal `json:"proposal,omitempty"`
	GoalID      *string       `json:"goal_id,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

func (gw *Gateway) handleListProjectBriefs(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []ProjectBrief{})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		        status, proposal, goal_id, created_at, updated_at
		 FROM project_briefs WHERE tenant_id = $1 ORDER BY created_at DESC`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []ProjectBrief{}
	for rows.Next() {
		var b ProjectBrief
		var proposalJSON []byte
		if err := rows.Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack,
			&b.BudgetCents, &b.Timeline, &b.Quality, &b.Status,
			&proposalJSON, &b.GoalID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			continue
		}
		if proposalJSON != nil {
			var p TeamProposal
			if json.Unmarshal(proposalJSON, &p) == nil {
				b.Proposal = &p
			}
		}
		list = append(list, b)
	}
	if list == nil {
		list = []ProjectBrief{}
	}
	writeJSON(w, 200, list)
}

func (gw *Gateway) handleGetProjectBrief(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var b ProjectBrief
	var proposalJSON []byte
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		        status, proposal, goal_id, created_at, updated_at
		 FROM project_briefs WHERE id = $1 AND tenant_id = $2`, id, defaultTenant).
		Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack,
			&b.BudgetCents, &b.Timeline, &b.Quality, &b.Status,
			&proposalJSON, &b.GoalID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if proposalJSON != nil {
		var p TeamProposal
		if json.Unmarshal(proposalJSON, &p) == nil {
			b.Proposal = &p
		}
	}
	writeJSON(w, 200, b)
}

func (gw *Gateway) handleCreateProjectBrief(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Title       string  `json:"title"`
		Idea        string  `json:"idea"`
		Stack       string  `json:"stack"`
		BudgetCents int     `json:"budget_cents"`
		Timeline    string  `json:"timeline"`
		Quality     string  `json:"quality"`
		GoalID      *string `json:"goal_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	if body.Quality == "" {
		body.Quality = "mvp"
	}
	if body.Timeline == "" {
		body.Timeline = "no_rush"
	}

	var b ProjectBrief
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO project_briefs
		   (tenant_id, title, idea, stack, budget_cents, timeline, quality, goal_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		           status, proposal, goal_id, created_at, updated_at`,
		defaultTenant, body.Title, body.Idea, body.Stack, body.BudgetCents,
		body.Timeline, body.Quality, body.GoalID).
		Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack,
			&b.BudgetCents, &b.Timeline, &b.Quality, &b.Status,
			nil, &b.GoalID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, b)
}

func (gw *Gateway) handleUpdateProjectBrief(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Title       *string `json:"title"`
		Idea        *string `json:"idea"`
		Stack       *string `json:"stack"`
		BudgetCents *int    `json:"budget_cents"`
		Timeline    *string `json:"timeline"`
		Quality     *string `json:"quality"`
		Status      *string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
	var b ProjectBrief
	var proposalJSON []byte
	err := gw.db.Pool.QueryRow(r.Context(),
		`UPDATE project_briefs SET
		   title        = COALESCE($2, title),
		   idea         = COALESCE($3, idea),
		   stack        = COALESCE($4, stack),
		   budget_cents = COALESCE($5, budget_cents),
		   timeline     = COALESCE($6, timeline),
		   quality      = COALESCE($7, quality),
		   status       = COALESCE($8, status),
		   updated_at   = NOW()
		 WHERE id = $1 AND tenant_id = $9
		 RETURNING id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		           status, proposal, goal_id, created_at, updated_at`,
		id, body.Title, body.Idea, body.Stack, body.BudgetCents,
		body.Timeline, body.Quality, body.Status, defaultTenant).
		Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack,
			&b.BudgetCents, &b.Timeline, &b.Quality, &b.Status,
			&proposalJSON, &b.GoalID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if proposalJSON != nil {
		var p TeamProposal
		if json.Unmarshal(proposalJSON, &p) == nil {
			b.Proposal = &p
		}
	}
	gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventProjectUpdated, Data: b})
	writeJSON(w, 200, b)
}

// handleProposeTeam generates a TeamProposal for the given brief.
// It first attempts an LLM-driven plan via the agent loop (Prime, plan_graph
// channel, 90s timeout). If the LLM succeeds and returns valid JSON that
// parses into a TeamProposal, that proposal is used. If the agent loop is
// unavailable, times out, or returns unparseable output, it falls back to
// the deterministic PlanTeam() heuristic so the endpoint never fails.
func (gw *Gateway) handleProposeTeam(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")

	var b ProjectBrief
	var proposalJSON []byte
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		        status, proposal, goal_id, created_at, updated_at
		 FROM project_briefs WHERE id = $1 AND tenant_id = $2`, id, defaultTenant).
		Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack,
			&b.BudgetCents, &b.Timeline, &b.Quality, &b.Status,
			&proposalJSON, &b.GoalID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "brief not found"})
		return
	}

	proposal := gw.llmProposeTeam(r.Context(), b)

	pJSON, _ := json.Marshal(proposal)
	err = gw.db.Pool.QueryRow(r.Context(),
		`UPDATE project_briefs SET proposal = $2, status = 'proposed', updated_at = NOW()
		 WHERE id = $1 RETURNING updated_at`, id, pJSON).Scan(&b.UpdatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	b.Status = "proposed"
	b.Proposal = &proposal

	gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventProjectUpdated, Data: b})
	writeJSON(w, 200, b)
}

// llmProposeTeam attempts to build a TeamProposal using the LLM.
// Falls back to PlanTeam() on any failure — the caller always gets a proposal.
func (gw *Gateway) llmProposeTeam(ctx context.Context, b ProjectBrief) TeamProposal {
	if gw.agentLoop == nil {
		slog.Debug("inception.propose: agent loop not available, using static planner")
		return PlanTeam(b.Idea, b.Stack, b.Quality, b.BudgetCents)
	}

	planCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	prompt := buildInceptionPlannerPrompt(b)

	var collected strings.Builder
	sessionID := fmt.Sprintf("inception-propose-%s", b.ID)

	_, runErr := gw.agentLoop.Run(planCtx, agent.RunRequest{
		AgentID:     "prime",
		SessionID:   sessionID,
		UserMessage: prompt,
		Channel:     "plan_graph",
		Stream:      true,
		NoPersist:   true,
		TenantID:    defaultTenant,
	}, func(ev agent.StreamEvent) {
		if ev.Type == "text_delta" && ev.Delta != "" {
			collected.WriteString(ev.Delta)
		}
	})

	if runErr != nil {
		slog.Warn("inception.propose: llm run failed, using static planner",
			"brief_id", b.ID, "err", runErr)
		return PlanTeam(b.Idea, b.Stack, b.Quality, b.BudgetCents)
	}

	raw := collected.String()
	proposal, parseErr := parseTeamProposalFromLLM(raw)
	if parseErr != nil {
		slog.Warn("inception.propose: llm output not parseable, using static planner",
			"brief_id", b.ID, "err", parseErr, "raw_len", len(raw))
		return PlanTeam(b.Idea, b.Stack, b.Quality, b.BudgetCents)
	}

	slog.Info("inception.propose: llm proposal accepted",
		"brief_id", b.ID, "agents", len(proposal.Agents), "tasks", len(proposal.Tasks))
	return proposal
}

// buildInceptionPlannerPrompt composes the prompt for the LLM team planner.
func buildInceptionPlannerPrompt(b ProjectBrief) string {
	stack := b.Stack
	if stack == "" {
		stack = "choose the best stack for the idea"
	}
	budget := "unspecified"
	if b.BudgetCents > 0 {
		budget = fmt.Sprintf("$%d", b.BudgetCents/100)
	}
	return fmt.Sprintf(`You are a senior software architect planning a project team.

Project title: %s
Idea: %s
Stack: %s
Budget: %s
Timeline: %s
Quality tier: %s

Design the minimal effective team for this project. Return a single JSON object with this exact shape:

{
  "agents": [
    {
      "role": "developer",
      "display_name": "Full-Stack Dev",
      "model": "claude-sonnet-4-6",
      "model_label": "Sonnet 4.6",
      "tasks": ["task title 1", "task title 2"],
      "est_min_cents": 800,
      "est_max_cents": 1200
    }
  ],
  "tasks": [
    {
      "title": "task title 1",
      "role": "developer",
      "priority": "high",
      "blocked_by": [],
      "est_min_cents": 200,
      "est_max_cents": 400
    }
  ],
  "est_min_cents": 900,
  "est_max_cents": 1400,
  "reasoning": "one sentence explaining the approach"
}

Rules:
- Use only these models: claude-haiku-4-5-20251001 (budget), claude-sonnet-4-6 (mid), claude-opus-4-7 (premium)
- blocked_by contains task TITLES (strings), not IDs
- Every task must appear in exactly one agent's tasks list
- Roles must be: developer, tester, reviewer, writer (include only needed roles)
- Costs are in USD cents
- Output JSON only — no prose before or after`,
		b.Title, b.Idea, stack, budget, b.Timeline, b.Quality)
}

// parseTeamProposalFromLLM extracts and validates a TeamProposal from raw LLM output.
func parseTeamProposalFromLLM(raw string) (TeamProposal, error) {
	// Find the outermost JSON object.
	start, end := -1, -1
	depth := 0
	inStr, esc := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inStr {
			if esc { esc = false; continue }
			if c == '\\' { esc = true; continue }
			if c == '"' { inStr = false }
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if start < 0 { start = i }
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 { end = i }
		}
		if end >= 0 { break }
	}
	if start < 0 || end < 0 {
		return TeamProposal{}, fmt.Errorf("no JSON object found in LLM output")
	}

	var p TeamProposal
	if err := json.Unmarshal([]byte(raw[start:end+1]), &p); err != nil {
		return TeamProposal{}, fmt.Errorf("parse JSON: %w", err)
	}
	if len(p.Agents) == 0 || len(p.Tasks) == 0 {
		return TeamProposal{}, fmt.Errorf("LLM proposal missing agents or tasks")
	}
	return p, nil
}

// handleApproveTeam creates agents + tickets atomically from the stored proposal.
func (gw *Gateway) handleApproveTeam(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var b ProjectBrief
	var proposalJSON []byte
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		        status, proposal, goal_id, created_at, updated_at
		 FROM project_briefs WHERE id = $1 AND tenant_id = $2`, id, defaultTenant).
		Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack,
			&b.BudgetCents, &b.Timeline, &b.Quality, &b.Status,
			&proposalJSON, &b.GoalID, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "brief not found"})
		return
	}
	if b.Status != "proposed" {
		writeJSON(w, 400, map[string]string{"error": "brief must be in proposed status"})
		return
	}
	var proposal TeamProposal
	if proposalJSON == nil || json.Unmarshal(proposalJSON, &proposal) != nil {
		writeJSON(w, 400, map[string]string{"error": "no proposal on this brief"})
		return
	}

	// Compute workspace path once — reused for all agents and the registry entry.
	inceptionSlug := sanitizeKey(b.Title)
	if inceptionSlug == "" {
		inceptionSlug = "project-" + b.ID[:8]
	}
	inceptionHome, _ := os.UserHomeDir()
	inceptionWorkspace := inceptionHome + "/qorven-workspace/" + inceptionSlug

	agentIDs := map[string]string{}

	for _, pa := range proposal.Agents {
		sysPrompt := buildAgentSystemPrompt(pa.Role, b.Title, inceptionWorkspace)
		var agentID string
		err := gw.db.Pool.QueryRow(ctx,
			`INSERT INTO agents
			   (tenant_id, agent_key, display_name, role, model, system_prompt, status, project_brief_id)
			 VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)
			 RETURNING id`,
			defaultTenant,
			sanitizeKey(pa.DisplayName+"-"+b.ID[:8]),
			pa.DisplayName,
			pa.Role,
			pa.Model,
			sysPrompt,
			id,
		).Scan(&agentID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "create agent: " + err.Error()})
			return
		}
		agentIDs[pa.Role] = agentID
		slog.Info("inception.agent_created", "role", pa.Role, "id", agentID)
	}

	ticketIDs := map[string]string{}
	for _, pt := range proposal.Tasks {
		slug, err := gw.nextTicketSlug(ctx, defaultTenant)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "slug: " + err.Error()})
			return
		}
		var ticketID string
		err = gw.db.Pool.QueryRow(ctx,
			`INSERT INTO tickets
			   (tenant_id, slug, title, description, priority, assigned_agent_id, project_brief_id)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 RETURNING id`,
			defaultTenant, slug, pt.Title,
			fmt.Sprintf("Part of project: %s\n\nIdea: %s", b.Title, b.Idea),
			pt.Priority,
			nullStr(agentIDs[pt.Role]),
			id,
		).Scan(&ticketID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "create ticket: " + err.Error()})
			return
		}
		ticketIDs[pt.Title] = ticketID
	}

	for _, pt := range proposal.Tasks {
		if len(pt.BlockedBy) == 0 {
			continue
		}
		blockerIDs := make([]string, 0, len(pt.BlockedBy))
		for _, dep := range pt.BlockedBy {
			if bid, ok := ticketIDs[dep]; ok {
				blockerIDs = append(blockerIDs, bid)
			}
		}
		if len(blockerIDs) == 0 {
			continue
		}
		_, err := gw.db.Pool.Exec(ctx,
			`UPDATE tickets SET blocked_by = $2::uuid[] WHERE id = $1`,
			ticketIDs[pt.Title], blockerIDs)
		if err != nil {
			slog.Warn("inception.blocked_by_failed", "title", pt.Title, "error", err)
		}
	}

	gw.db.Pool.Exec(ctx, //nolint:errcheck
		`UPDATE project_briefs SET status = 'active', updated_at = NOW() WHERE id = $1`, id)
	b.Status = "active"

	// Bridge to multi-agent daemon: propose a plan so external agents (Kiro,
	// Claude Code) can pick up tasks via /v1/daemon/stream SSE.
	// Single-task plans auto-approve; multi-task wait for human review.
	if gw.daemonReg != nil && len(proposal.Tasks) > 0 {
		planTasks := make([]daemonPkg.PlanTask, 0, len(proposal.Tasks))
		for _, pt := range proposal.Tasks {
			planTasks = append(planTasks, daemonPkg.PlanTask{
				ID:              ticketIDs[pt.Title],
				Title:           pt.Title,
				OwnerCapability: pt.Role,
				Priority:        pt.Priority,
				TicketID:        ticketIDs[pt.Title],
				OriginID:        id, // project_brief_id
				OriginType:      "project_brief",
				ExtraContext: map[string]any{
					"brief_title": b.Title,
					"brief_idea":  b.Idea,
					"brief_stack": b.Stack,
					"timeline":    b.Timeline,
					"quality":     b.Quality,
				},
			})
		}
		gw.daemonReg.ProposePlan(b.Title, b.Idea, "inception", planTasks)
	}

	// Create Kanban tasks linked to each ticket so the Tasks board reflects the inception work.
	for _, pt := range proposal.Tasks {
		ticketID := ticketIDs[pt.Title]
		agentID := agentIDs[pt.Role]
		status := "assigned"
		if agentID == "" {
			status = "backlog"
		}
		var taskID string
		if err := gw.db.Pool.QueryRow(ctx,
			`INSERT INTO tasks (tenant_id, ticket_id, title, description, status, assigned_to, priority)
			 VALUES ($1, $2, $3, $4, $5, $6, 3) RETURNING id`,
			defaultTenant, ticketID, pt.Title,
			fmt.Sprintf("Part of project: %s", b.Title),
			status, nullStr(agentID),
		).Scan(&taskID); err != nil {
			slog.Warn("inception.task_create_failed", "title", pt.Title, "error", err)
		} else {
			slog.Info("inception.task_created", "task", taskID, "ticket", ticketID)
		}
	}

	for _, pt := range proposal.Tasks {
		if len(pt.BlockedBy) > 0 {
			continue
		}
		agentID, ok := agentIDs[pt.Role]
		if !ok {
			continue
		}
		ticketID := ticketIDs[pt.Title]
		gw.db.Pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO heartbeat_queue (tenant_id, agent_id, trigger, context_type, context_id)
			 VALUES ($1, $2, 'ticket_assigned', 'ticket', $3)`,
			defaultTenant, agentID, ticketID)
		slog.Info("inception.ticket_queued", "ticket", pt.Title, "agent", pt.Role)
	}

	// Bridge Inception → Project Registry so the agent loop can inject
	// PrimeCoderSystemPrompt for agents spawned from this brief.
	if gw.projectReg != nil {
		os.MkdirAll(inceptionWorkspace, 0755) //nolint:errcheck
		gw.projectReg.CreateFromInception(inceptionSlug, b.Title, inceptionWorkspace, b.ID)
		slog.Info("inception.project_registry_created", "brief_id", b.ID, "workspace", inceptionWorkspace)
	}

	gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventProjectUpdated, Data: b})
	writeJSON(w, 200, map[string]any{
		"brief":   b,
		"agents":  agentIDs,
		"tickets": ticketIDs,
	})
}

// handleGetBriefTeam returns agents scoped to a project brief.
func (gw *Gateway) handleGetBriefTeam(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"agents": []any{}})
		return
	}
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	type AgentSummary struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name"`
		Role        string  `json:"role"`
		Model       string  `json:"model"`
		Status      string  `json:"status"`
		BudgetCents *int64  `json:"credit_budget_cents,omitempty"`
		UsedCents   int64   `json:"credit_used_cents"`
	}

	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, display_name, role, model, status, credit_budget_cents, credit_used_cents
		 FROM agents
		 WHERE project_brief_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at`,
		id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	agents := []AgentSummary{}
	for rows.Next() {
		var a AgentSummary
		if err := rows.Scan(&a.ID, &a.DisplayName, &a.Role, &a.Model, &a.Status, &a.BudgetCents, &a.UsedCents); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		agents = append(agents, a)
	}
	writeJSON(w, 200, map[string]any{"agents": agents})
}

func buildAgentSystemPrompt(role, projectTitle, workspacePath string) string {
	base := fmt.Sprintf("You are working on the project: %s.\nWorkspace: %s\n\n", projectTitle, workspacePath)
	switch role {
	case "developer":
		return base + "You are a senior software developer. Write clean, idiomatic code. Use the record_file_touch tool whenever you create or modify a file. Mark your ticket done when the implementation is complete and tests pass."
	case "tester":
		return base + "You are a QA engineer. Write comprehensive tests. Prioritise edge cases and integration scenarios. Use record_file_touch when you write test files."
	case "reviewer":
		return base + "You are a code reviewer and security auditor. Review code for correctness, security vulnerabilities, and adherence to best practices. Leave detailed comments."
	case "writer":
		return base + "You are a technical writer. Write clear, concise documentation. Include examples."
	default:
		return base + fmt.Sprintf("You are a %s working on this project.", role)
	}
}

func sanitizeKey(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s) && i < 40; i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			out = append(out, c)
		} else if c >= 'A' && c <= 'Z' {
			out = append(out, c+32)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Ensure context import is used (nextTicketSlug already uses context.Context via ctx).
var _ context.Context
