// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// heartbeat_worker.go — polls heartbeat_queue and runs agents autonomously.
//
// When a ticket is assigned, handleAssignTicket inserts a row into
// heartbeat_queue with trigger='ticket_assigned'. This worker picks it up,
// builds a context message from the ticket, and runs the agent via
// agentLoop.Run with three injected extra tools:
//   - update_ticket_status
//   - add_ticket_comment
//   - record_file_touch

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/tools"
)

const (
	hbPollInterval  = 10 * time.Second
	hbMaxConcurrent = 3
)

// startHeartbeatWorker launches the background poll loop. Called once from Serve().
func (gw *Gateway) startHeartbeatWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(hbPollInterval)
		defer ticker.Stop()
		sem := make(chan struct{}, hbMaxConcurrent)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				gw.processHeartbeatQueue(ctx, sem)
			}
		}
	}()
}

func (gw *Gateway) processHeartbeatQueue(ctx context.Context, sem chan struct{}) {
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, agent_id, context_type, context_id
		 FROM heartbeat_queue
		 WHERE status = 'pending' AND run_at <= NOW() AND tenant_id = $1
		 ORDER BY run_at
		 LIMIT 10`, defaultTenant)
	if err != nil {
		slog.Error("heartbeat.queue.query", "error", err)
		return
	}
	defer rows.Close()

	type entry struct{ id, agentID, ctxType, ctxID string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.id, &e.agentID, &e.ctxType, &e.ctxID); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	rows.Close()

	for _, e := range entries {
		tag, err := gw.db.Pool.Exec(ctx,
			`UPDATE heartbeat_queue SET status = 'running' WHERE id = $1 AND status = 'pending'`, e.id)
		if err != nil || tag.RowsAffected() == 0 {
			continue
		}
		sem <- struct{}{}
		go func(e entry) {
			defer func() { <-sem }()
			gw.runHeartbeat(e.id, e.agentID, e.ctxType, e.ctxID)
		}(e)
	}
}

func (gw *Gateway) runHeartbeat(hbID, agentID, ctxType, ctxID string) {
	ctx := context.Background()
	start := time.Now()

	// Budget check — skip run if agent has exceeded its credit budget.
	if gw.agentOverBudget(ctx, agentID) {
		gw.rtHub.Broadcast(realtime.Event{
			Type: realtime.EventBudgetWarning,
			Data: map[string]any{"agent_id": agentID, "reason": "over_budget"},
		})
		gw.db.Pool.Exec(ctx, //nolint:errcheck
			`UPDATE heartbeat_queue SET status = 'failed', error_msg = 'agent over budget', finished_at = NOW() WHERE id = $1`,
			hbID)
		slog.Warn("heartbeat.over_budget", "hb_id", hbID, "agent", agentID)
		return
	}

	userMsg, ticketObj, err := gw.buildHeartbeatContext(ctx, ctxType, ctxID)
	if err != nil {
		gw.failHeartbeat(hbID, err.Error())
		return
	}

	req := agent.RunRequest{
		AgentID:     agentID,
		UserMessage: userMsg,
		Channel:     "heartbeat",
		ExtraTools:  gw.buildTicketTools(ctx, ctxID, agentID),
		TenantID:    defaultTenant,
	}

	result, runErr := gw.agentLoop.Run(ctx, req, func(_ agent.StreamEvent) {})

	// Record cost and check budget warning.
	if runErr == nil && result != nil && result.CostCents > 0 {
		costInt := int64(result.CostCents)
		gw.db.Pool.Exec(ctx, //nolint:errcheck
			`UPDATE agents SET credit_used_cents = credit_used_cents + $2 WHERE id = $1`,
			agentID, costInt)
		gw.checkBudgetWarning(ctx, agentID)
	}

	status := "done"
	errMsg := ""
	if runErr != nil {
		status = "failed"
		errMsg = runErr.Error()
		slog.Error("heartbeat.run.failed", "hb_id", hbID, "agent", agentID, "error", runErr)
	} else {
		slog.Info("heartbeat.run.done", "hb_id", hbID, "agent", agentID, "elapsed", time.Since(start))
	}

	gw.db.Pool.Exec(ctx, //nolint:errcheck
		`UPDATE heartbeat_queue SET status = $2, error_msg = $3, finished_at = NOW() WHERE id = $1`,
		hbID, status, errMsg)

	if ticketObj != nil && runErr == nil {
		// Testing gate — if the agent marked the ticket done, run tests before unblocking.
		if ticketObj.Status == "done" && ctxType == "ticket" {
			if !gw.runTestingGate(hbID, ctxID, agentID) {
				// Tests failed; testing_gate.go already reopened the ticket.
				return
			}
		}
		gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketUpdated, Data: ticketObj})
		if ticketObj.Status == "done" && ctxType == "ticket" {
			gw.unblockDependents(ctx, ctxID)
		}
	} else if ticketObj != nil {
		gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketUpdated, Data: ticketObj})
	}
}

func (gw *Gateway) buildHeartbeatContext(ctx context.Context, ctxType, ctxID string) (string, *Ticket, error) {
	if ctxType != "ticket" {
		return fmt.Sprintf("You have been assigned work. Context type: %s, id: %s", ctxType, ctxID), nil, nil
	}
	var t Ticket
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT id, tenant_id, slug, title, description, status, priority,
		        assigned_agent_id, goal_id, created_at, updated_at
		 FROM tickets WHERE id = $1 AND tenant_id = $2`, ctxID, defaultTenant).
		Scan(&t.ID, &t.TenantID, &t.Slug, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedAgentID, &t.GoalID,
			&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return "", nil, fmt.Errorf("load ticket: %w", err)
	}

	rows, _ := gw.db.Pool.Query(ctx,
		`SELECT author_type, author_id, body FROM ticket_comments
		 WHERE ticket_id = $1 ORDER BY created_at DESC LIMIT 10`, ctxID)
	commentBlock := ""
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var at, aid, body string
			rows.Scan(&at, &aid, &body) //nolint:errcheck
			commentBlock += fmt.Sprintf("\n[%s %s]: %s", at, aid, body)
		}
	}

	msg := fmt.Sprintf(`You have been assigned ticket %s.

Title: %s
Status: %s
Priority: %s
Description: %s
%s

Work on this ticket. Use update_ticket_status when done or blocked. Use add_ticket_comment to report your progress. Use record_file_touch whenever you create or modify a file.`,
		t.Slug, t.Title, t.Status, t.Priority, t.Description, commentBlock)

	return msg, &t, nil
}

func (gw *Gateway) failHeartbeat(hbID, reason string) {
	gw.db.Pool.Exec(context.Background(), //nolint:errcheck
		`UPDATE heartbeat_queue SET status = 'failed', error_msg = $2, finished_at = NOW() WHERE id = $1`,
		hbID, reason)
	slog.Error("heartbeat.failed", "hb_id", hbID, "reason", reason)
}

// ─────────────────────────────────────────────────────────────────────────────
// Ticket-scoped agent tools (injected only during heartbeat runs)
// ─────────────────────────────────────────────────────────────────────────────

func (gw *Gateway) buildTicketTools(ctx context.Context, ticketID, agentID string) []tools.Tool {
	return []tools.Tool{
		&updateTicketStatusTool{gw: gw, ticketID: ticketID},
		&addTicketCommentTool{gw: gw, ticketID: ticketID, agentID: agentID},
		&recordFileTouchTool{gw: gw, ticketID: ticketID},
	}
}

// ── update_ticket_status ──────────────────────────────────────────────────────

type updateTicketStatusTool struct {
	gw       *Gateway
	ticketID string
}

func (t *updateTicketStatusTool) Name() string { return "update_ticket_status" }
func (t *updateTicketStatusTool) Description() string {
	return "Update the status of the current ticket. Call when work is complete or you are blocked."
}
func (t *updateTicketStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []string{"todo", "in_progress", "blocked", "done"},
			},
			"reason": map[string]any{"type": "string"},
		},
		"required": []string{"status"},
	}
}
func (t *updateTicketStatusTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	status, _ := args["status"].(string)
	if status == "" {
		return tools.ErrorResult("status required")
	}
	var ticket Ticket
	err := t.gw.db.Pool.QueryRow(ctx,
		`UPDATE tickets SET status = $2, updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, tenant_id, slug, title, description, status, priority,
		           assigned_agent_id, goal_id, created_at, updated_at`,
		t.ticketID, status).
		Scan(&ticket.ID, &ticket.TenantID, &ticket.Slug, &ticket.Title, &ticket.Description,
			&ticket.Status, &ticket.Priority, &ticket.AssignedAgentID, &ticket.GoalID,
			&ticket.CreatedAt, &ticket.UpdatedAt)
	if err != nil {
		return tools.ErrorResult("db error: " + err.Error())
	}
	t.gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketUpdated, Data: ticket})
	return &tools.Result{ForLLM: fmt.Sprintf("Ticket status updated to %s", status)}
}

// ── add_ticket_comment ────────────────────────────────────────────────────────

type addTicketCommentTool struct {
	gw       *Gateway
	ticketID string
	agentID  string
}

func (t *addTicketCommentTool) Name() string { return "add_ticket_comment" }
func (t *addTicketCommentTool) Description() string {
	return "Post a comment to the current ticket visible to the user. Markdown supported."
}
func (t *addTicketCommentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"body": map[string]any{"type": "string"},
		},
		"required": []string{"body"},
	}
}
func (t *addTicketCommentTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	body, _ := args["body"].(string)
	if body == "" {
		return tools.ErrorResult("body required")
	}
	var c TicketComment
	err := t.gw.db.Pool.QueryRow(ctx,
		`INSERT INTO ticket_comments (ticket_id, author_type, author_id, body)
		 VALUES ($1, 'agent', $2, $3)
		 RETURNING id, ticket_id, author_type, author_id, body, created_at`,
		t.ticketID, t.agentID, body).
		Scan(&c.ID, &c.TicketID, &c.AuthorType, &c.AuthorID, &c.Body, &c.CreatedAt)
	if err != nil {
		return tools.ErrorResult("db error: " + err.Error())
	}
	t.gw.rtHub.Broadcast(realtime.Event{Type: realtime.EventTicketComment, Data: c})
	return &tools.Result{ForLLM: "Comment posted."}
}

// ── record_file_touch ─────────────────────────────────────────────────────────

type recordFileTouchTool struct {
	gw       *Gateway
	ticketID string
}

func (t *recordFileTouchTool) Name() string { return "record_file_touch" }
func (t *recordFileTouchTool) Description() string {
	return "Record that you created or modified a file while working on this ticket."
}
func (t *recordFileTouchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":      map[string]any{"type": "string", "description": "Path relative to project root"},
			"operation": map[string]any{"type": "string", "enum": []string{"created", "modified", "deleted"}},
		},
		"required": []string{"path", "operation"},
	}
}
func (t *recordFileTouchTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	path, _ := args["path"].(string)
	op, _ := args["operation"].(string)
	if path == "" || op == "" {
		return tools.ErrorResult("path and operation required")
	}
	var f TicketFile
	err := t.gw.db.Pool.QueryRow(ctx,
		`INSERT INTO ticket_files (ticket_id, path, operation)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (ticket_id, path) DO UPDATE SET operation = EXCLUDED.operation, touched_at = NOW()
		 RETURNING id, ticket_id, path, operation, touched_at`,
		t.ticketID, path, op).
		Scan(&f.ID, &f.TicketID, &f.Path, &f.Operation, &f.TouchedAt)
	if err != nil {
		return tools.ErrorResult("db error: " + err.Error())
	}
	t.gw.rtHub.Broadcast(realtime.Event{
		Type: realtime.EventTicketUpdated,
		Data: map[string]string{"id": t.ticketID, "files_changed": "true"},
	})
	return &tools.Result{ForLLM: fmt.Sprintf("Recorded %s of %s", op, path)}
}

// ─────────────────────────────────────────────────────────────────────────────
// Budget helpers
// ─────────────────────────────────────────────────────────────────────────────

// agentOverBudget returns true if credit_used_cents >= credit_budget_cents (and budget > 0).
func (gw *Gateway) agentOverBudget(ctx context.Context, agentID string) bool {
	var used, budget int
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT credit_used_cents, credit_budget_cents FROM agents WHERE id = $1`, agentID).
		Scan(&used, &budget)
	if err != nil || budget == 0 {
		return false
	}
	return used >= budget
}

// checkBudgetWarning broadcasts a warning if agent is at 80%+ of budget.
func (gw *Gateway) checkBudgetWarning(ctx context.Context, agentID string) {
	var used, budget int
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT credit_used_cents, credit_budget_cents FROM agents WHERE id = $1`, agentID).
		Scan(&used, &budget); err != nil || budget == 0 {
		return
	}
	if used*100/budget >= 80 {
		gw.rtHub.Broadcast(realtime.Event{
			Type: realtime.EventBudgetWarning,
			Data: map[string]any{
				"agent_id": agentID,
				"used":     used,
				"budget":   budget,
				"pct":      used * 100 / budget,
			},
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Dependency unblocking
// ─────────────────────────────────────────────────────────────────────────────

// unblockDependents finds tickets blocked by completedID and queues them when all their blockers are done.
func (gw *Gateway) unblockDependents(ctx context.Context, completedTicketID string) {
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, assigned_agent_id, blocked_by FROM tickets
		 WHERE $1 = ANY(blocked_by) AND tenant_id = $2 AND status = 'todo'`,
		completedTicketID, defaultTenant)
	if err != nil {
		slog.Error("unblock.query", "error", err)
		return
	}
	defer rows.Close()

	type candidate struct {
		id        string
		agentID   *string
		blockedBy []string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.agentID, &c.blockedBy); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}
	rows.Close()

	for _, c := range candidates {
		if !gw.allBlockersDone(ctx, c.blockedBy) {
			continue
		}
		if c.agentID == nil {
			continue
		}
		_, err := gw.db.Pool.Exec(ctx,
			`UPDATE tickets SET status = 'in_progress', updated_at = NOW() WHERE id = $1`, c.id)
		if err != nil {
			slog.Warn("unblock.update_status", "ticket", c.id, "error", err)
			continue
		}
		gw.db.Pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO heartbeat_queue (tenant_id, agent_id, trigger, context_type, context_id)
			 VALUES ($1, $2, 'ticket_assigned', 'ticket', $3)`,
			defaultTenant, *c.agentID, c.id)
		slog.Info("heartbeat.unblocked", "ticket", c.id, "agent", *c.agentID)
	}
}

func (gw *Gateway) allBlockersDone(ctx context.Context, blockerIDs []string) bool {
	if len(blockerIDs) == 0 {
		return true
	}
	var count int
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tickets WHERE id = ANY($1) AND status != 'done'`,
		blockerIDs).Scan(&count); err != nil {
		return false // conservative: don't unblock on DB error
	}
	return count == 0
}
