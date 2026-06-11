// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/tasks"
)

const fixAttemptCap = 3

// fixDedupKey returns a stable string that identifies a unique failure event
// so that repeated occurrences can be matched to the same GitHub issue body.
func fixDedupKey(source, ref string) string { return source + ":" + ref }

// fixAttemptExceeded reports whether the attempt count has reached or exceeded
// the cap. A cap of 0 disables the limit entirely.
func fixAttemptExceeded(attempt, cap int) bool { return cap > 0 && attempt >= cap }

// triggerFixLoop is the single funnel for CI/deploy/bug failures: dedup-or-open
// a GitHub issue, create a fix task assigned to the CTO (or prime), wake them,
// and record the event. Best-effort / logged; never blocks the caller. Only
// fires for Org projects with a connected repo. At the attempt cap it escalates
// (labels the issue needs-human + pauses the project) instead of looping.
func (gw *Gateway) triggerFixLoop(ctx context.Context, briefID, source, ref, title, detail string) {
	if gw.db == nil || briefID == "" {
		return
	}

	var owner, repo string
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT github_owner, github_repo FROM project_briefs WHERE id=$1`, briefID,
	).Scan(&owner, &repo)
	if owner == "" || repo == "" {
		slog.Warn("fix_loop.no_repo", "brief", briefID, "source", source)
		return
	}

	key := fixDedupKey(source, ref)
	issueNum := gw.findOpenAutofixIssue(ctx, owner, repo, key)
	if issueNum == 0 {
		issueNum = gw.openAutofixIssue(ctx, owner, repo, title,
			detail+"\n\n<!-- qorven-fix-key: "+key+" -->")
	} else {
		gw.commentOnIssue(ctx, owner, repo, issueNum, "Recurred:\n\n"+detail)
	}
	if issueNum == 0 {
		slog.Warn("fix_loop.issue_failed", "brief", briefID)
		return
	}

	// Compute the attempt count for this issue; escalate at the cap.
	var prior int
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(fix_attempt),0) FROM tasks WHERE github_issue_number=$1 AND project_brief_id=$2`,
		issueNum, briefID,
	).Scan(&prior)
	attempt := prior + 1

	if fixAttemptExceeded(attempt, fixAttemptCap) {
		gw.escalateFix(ctx, briefID, owner, repo, issueNum)
		return
	}

	cto := gw.ctoAgentForBrief(ctx, briefID)
	if cto == "" {
		cto, _ = gw.findPrimeAgentID(ctx)
	}

	taskID, err := gw.createFixTask(ctx, briefID, cto, issueNum, source, attempt, title, detail)
	if err != nil {
		slog.Warn("fix_loop.task_failed", "err", err)
		return
	}

	if gw.runtimeMgr != nil && cto != "" {
		gw.runtimeMgr.WakeAgent(cto, agent.WakeupSignal{
			Source: agent.WakeupAssignment,
			TaskID: taskID,
		})
	}

	gw.emitProjectEvent(ctx, briefID, "fix_triggered", title,
		map[string]any{"source": source, "issue": issueNum, "attempt": attempt},
		taskID, cto)
}

// findOpenAutofixIssue searches for an open issue with the qorven-autofix label
// whose body embeds the given dedup key. Returns the issue number, or 0 if not found.
func (gw *Gateway) findOpenAutofixIssue(ctx context.Context, owner, repo, key string) int {
	data, _, err := gw.ghProxy(ctx,
		fmt.Sprintf("/repos/%s/%s/issues", owner, repo),
		url.Values{
			"state":    {"open"},
			"labels":   {"qorven-autofix"},
			"per_page": {"50"},
		},
	)
	if err != nil {
		slog.Warn("fix_loop.search_issues", "err", err)
		return 0
	}

	var issues []struct {
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(data, &issues); err != nil {
		return 0
	}

	needle := "qorven-fix-key: " + key
	for _, iss := range issues {
		if strings.Contains(iss.Body, needle) {
			return iss.Number
		}
	}
	return 0
}

// openAutofixIssue creates a new GitHub issue with the qorven-autofix label.
// Returns the new issue number, or 0 on error.
func (gw *Gateway) openAutofixIssue(ctx context.Context, owner, repo, title, body string) int {
	data, _, err := gw.ghPost(ctx,
		fmt.Sprintf("/repos/%s/%s/issues", owner, repo),
		map[string]any{
			"title":  title,
			"body":   body,
			"labels": []string{"qorven-autofix"},
		},
	)
	if err != nil {
		slog.Warn("fix_loop.open_issue", "err", err)
		return 0
	}

	var resp struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0
	}
	return resp.Number
}

// commentOnIssue appends a comment to an existing GitHub issue.
func (gw *Gateway) commentOnIssue(ctx context.Context, owner, repo string, n int, body string) {
	_, _, err := gw.ghPost(ctx,
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, n),
		map[string]any{"body": body},
	)
	if err != nil {
		slog.Warn("fix_loop.comment_issue", "issue", n, "err", err)
	}
}

// ctoAgentForBrief returns the agent id of the CTO role agent provisioned for
// the given project brief, or "" if none exists.
func (gw *Gateway) ctoAgentForBrief(ctx context.Context, briefID string) string {
	if gw.db == nil {
		return ""
	}
	var id string
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT id::text FROM agents WHERE project_brief_id=$1 AND org_role='cto' LIMIT 1`,
		briefID,
	).Scan(&id)
	return id
}

// createFixTask creates a task for the given fix and stamps the project_brief_id,
// github_issue_number, fix_source, and fix_attempt via a follow-up UPDATE (the
// INSERT omits those columns). Returns the task id.
func (gw *Gateway) createFixTask(
	ctx context.Context,
	briefID, agentID string,
	issueNum int,
	source string,
	attempt int,
	title, detail string,
) (string, error) {
	t := tasks.Task{
		Title:       title,
		Description: detail,
		Status:      tasks.StatusAssigned,
		Priority:    1,
	}
	if agentID != "" {
		t.AssignedTo = &agentID
	}

	taskID, err := gw.taskStore.Create(ctx, defaultTenant, t)
	if err != nil {
		return "", fmt.Errorf("createFixTask: create: %w", err)
	}

	_, err = gw.db.Pool.Exec(ctx,
		`UPDATE tasks
		    SET project_brief_id   = $2::uuid,
		        github_issue_number = $3,
		        fix_source          = $4,
		        fix_attempt         = $5
		  WHERE id = $1`,
		taskID, briefID, issueNum, source, attempt,
	)
	if err != nil {
		slog.Warn("fix_loop.stamp_task", "task", taskID, "err", err)
		// Non-fatal: the task exists; the stamps are best-effort bookkeeping.
	}
	return taskID, nil
}

// escalateFix adds the needs-human label to the GitHub issue, pauses the
// project, and emits a blocked event so the dashboard reflects the stall.
func (gw *Gateway) escalateFix(ctx context.Context, briefID, owner, repo string, issueNum int) {
	slog.Warn("fix_loop.escalate", "brief", briefID, "issue", issueNum, "cap", fixAttemptCap)

	// Add the needs-human label — GitHub labels endpoint accepts {"labels":[...]}.
	_, _, err := gw.ghPost(ctx,
		fmt.Sprintf("/repos/%s/%s/issues/%d/labels", owner, repo, issueNum),
		map[string]any{"labels": []string{"needs-human"}},
	)
	if err != nil {
		slog.Warn("fix_loop.escalate.label", "issue", issueNum, "err", err)
	}

	// Pause the project — identical UPDATE used by checkProjectBreaker (8C budget cap).
	if gw.db != nil {
		_, err = gw.db.Pool.Exec(ctx,
			`UPDATE project_briefs SET paused=true, pause_reason='fix-loop attempt cap reached' WHERE id=$1`,
			briefID,
		)
		if err != nil {
			slog.Warn("fix_loop.escalate.pause", "brief", briefID, "err", err)
		}
	}

	gw.emitProjectEvent(ctx, briefID, "blocked",
		"Fix-loop attempt cap reached — needs human review",
		map[string]any{"issue": issueNum, "cap": fixAttemptCap},
		"", "",
	)
}
