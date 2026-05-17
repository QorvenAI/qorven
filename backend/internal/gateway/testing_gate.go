// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/realtime"
)

// runTestingGate checks whether the test_command for a ticket passes.
// Called by the heartbeat worker after an agent marks a ticket as done.
// If tests fail: reopens the ticket to in_progress, posts a comment, returns false.
// If tests pass or no test_command: returns true (keep status as done).
func (gw *Gateway) runTestingGate(hbID, ticketID, agentID string) bool {
	_ = hbID
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var testCmd string
	var slug string
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT test_command, slug FROM tickets WHERE id = $1`, ticketID).
		Scan(&testCmd, &slug)
	if err != nil || testCmd == "" {
		return true
	}

	slog.Info("testing_gate.start", "ticket", slug, "cmd", testCmd)
	parts := strings.Fields(testCmd)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	output := stdout.String() + stderr.String()

	if runErr != nil {
		gw.db.Pool.Exec(ctx, //nolint:errcheck
			`UPDATE tickets SET status = 'in_progress', updated_at = NOW() WHERE id = $1`, ticketID)

		comment := fmt.Sprintf("🔴 **Tests failed — ticket reopened**\n\n```\n%s\n```\n\nPlease fix the failures and mark the ticket done again.", truncate(output, 2000))
		var commentID string
		gw.db.Pool.QueryRow(ctx,
			`INSERT INTO ticket_comments (ticket_id, author_type, author_id, body)
			 VALUES ($1, 'agent', $2, $3) RETURNING id`,
			ticketID, "prime-testing-gate", comment).Scan(&commentID)

		gw.rtHub.Broadcast(realtime.Event{
			Type: realtime.EventTicketUpdated,
			Data: map[string]string{"id": ticketID, "status": "in_progress"},
		})
		gw.rtHub.Broadcast(realtime.Event{
			Type: realtime.EventTicketComment,
			Data: map[string]string{"ticket_id": ticketID, "id": commentID},
		})

		gw.db.Pool.Exec(ctx, //nolint:errcheck
			`INSERT INTO heartbeat_queue (tenant_id, agent_id, trigger, context_type, context_id)
			 VALUES ($1, $2, 'ticket_assigned', 'ticket', $3)`,
			defaultTenant, agentID, ticketID)

		slog.Warn("testing_gate.failed", "ticket", slug, "error", runErr)
		return false
	}

	slog.Info("testing_gate.passed", "ticket", slug)
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}
