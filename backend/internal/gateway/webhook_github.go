// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/realtime"
)

// handleGitHubWebhook validates the GitHub webhook signature and dispatches
// the event to Prime via the heartbeat queue.
//
// Supports per-project routing via ?project=<id> query param — verifies the
// signature against the project's GitHubSecret, falling back to the global
// GITHUB_WEBHOOK_SECRET env var.
func (gw *Gateway) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "read body"})
		return
	}

	// Resolve the HMAC secret: per-project first, then global env.
	secret := ""
	projectID := r.URL.Query().Get("project")
	if projectID != "" && gw.projectReg != nil {
		if p := gw.projectReg.Get(projectID); p != nil {
			secret = p.GitHubSecret
		}
	}
	if secret == "" {
		secret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	}

	if secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, secret, sig) {
			writeJSON(w, 401, map[string]string{"error": "invalid signature"})
			return
		}
	}

	event := r.Header.Get("X-GitHub-Event")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}

	// Handle check_run completions — advance the GitHubTaskQueue phase.
	if event == "check_run" {
		gw.handleCheckRunEvent(r.Context(), payload)
		writeJSON(w, 200, map[string]string{"status": "handled"})
		return
	}

	// Handle pull_request events that don't need a ticket (merged, closed).
	if event == "pull_request" {
		action, _ := payload["action"].(string)
		if action == "closed" {
			merged, _ := nestedBool(payload, "pull_request", "merged")
			if merged {
				gw.handlePRMerged(r.Context(), payload)
			}
			writeJSON(w, 200, map[string]string{"status": "handled"})
			return
		}
	}

	msg := buildGitHubEventMessage(event, payload)
	if msg == "" {
		writeJSON(w, 200, map[string]string{"status": "ignored"})
		return
	}

	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}

	primeID, err := gw.findPrimeAgentID(r.Context())
	if err != nil || primeID == "" {
		slog.Warn("github.webhook.no_prime", "event", event)
		writeJSON(w, 200, map[string]string{"status": "no_prime_agent"})
		return
	}

	// Build ticket title with GitHub metadata.
	ticketTitle := "GitHub: " + event + " — " + extractTitle(payload)

	// For issues.opened, populate github_issue_number on the created ticket via extra context.
	issueNum := 0
	if event == "issues" {
		if n, ok := nestedFloat(payload, "issue", "number"); ok {
			issueNum = int(n)
		}
	}
	_ = issueNum // used in future: populate tasks.github_issue_number on task creation

	slug, _ := gw.nextTicketSlug(r.Context(), defaultTenant)
	var ticketID string
	gw.db.Pool.QueryRow(r.Context(), //nolint:errcheck
		`INSERT INTO tickets (tenant_id, slug, title, description, priority, assigned_agent_id)
		 VALUES ($1, $2, $3, $4, 'high', $5) RETURNING id`,
		defaultTenant, slug, ticketTitle, msg, primeID,
	).Scan(&ticketID)

	if ticketID != "" {
		gw.db.Pool.Exec(r.Context(), //nolint:errcheck
			`INSERT INTO heartbeat_queue (tenant_id, agent_id, trigger, context_type, context_id)
			 VALUES ($1, $2, 'ticket_assigned', 'ticket', $3)`,
			defaultTenant, primeID, ticketID)
		slog.Info("github.webhook.dispatched", "event", event, "ticket", ticketID)
	}

	// For pull_request.opened: broadcast a pr_ready event so the web UI
	// can render an approval card in Prime's chat feed.
	if event == "pull_request" {
		gw.broadcastPRReadyEvent(r.Context(), payload, primeID)
	}

	writeJSON(w, 200, map[string]string{"status": "queued", "ticket_id": ticketID})
}

// handleCheckRunEvent advances or fails a GitHubTask when a CI check completes.
func (gw *Gateway) handleCheckRunEvent(ctx context.Context, payload map[string]any) {
	action, _ := payload["action"].(string)
	if action != "completed" {
		return
	}
	conclusion, _ := nestedStr2(payload, "check_run", "conclusion")
	headSHA, _ := nestedStr2(payload, "check_run", "head_sha")
	checkName, _ := nestedStr2(payload, "check_run", "name")

	// Find a task in PhaseAwaitingCI that matches this SHA or branch.
	tasks := agent.GlobalGitHubTaskQueue.ListAll()
	for _, t := range tasks {
		if t.Phase != agent.PhaseAwaitingCI {
			continue
		}
		// Match by branch name embedded in the check's pull_requests list.
		matched := false
		if prs, ok := payload["check_run"].(map[string]any); ok {
			if prList, ok := prs["pull_requests"].([]any); ok {
				for _, pr := range prList {
					if prMap, ok := pr.(map[string]any); ok {
						if head, ok := prMap["head"].(map[string]any); ok {
							if ref, _ := head["ref"].(string); ref == t.Branch {
								matched = true
								break
							}
						}
					}
				}
			}
		}
		if !matched && headSHA != "" && strings.HasPrefix(headSHA, t.Branch) {
			matched = true
		}
		if !matched {
			continue
		}

		switch conclusion {
		case "success", "neutral", "skipped":
			slog.Info("github.check_run.success", "task", t.ID, "check", checkName)
			// CI passed — advance to complete (agent will merge via gh_merge_pr).
			agent.GlobalGitHubTaskQueue.Advance(t.ID, agent.PhaseComplete)
			// Notify Prime so it can call gh_merge_pr.
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{
					Type: string(apievents.TypeGitHubCIStatus),
					Data: map[string]any{
						"task_id":    t.ID,
						"pr_number":  t.PRNumber,
						"pr_url":     t.PRURL,
						"ci_status":  "passing",
						"conclusion": conclusion,
						"check":      checkName,
					},
				})
			}
		case "failure", "timed_out", "cancelled", "action_required":
			output := fmt.Sprintf("CI check %q failed with conclusion: %s", checkName, conclusion)
			slog.Warn("github.check_run.failure", "task", t.ID, "check", checkName, "conclusion", conclusion)
			agent.GlobalGitHubTaskQueue.RecordTestFailure(t.ID, output)
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{
					Type: string(apievents.TypeGitHubCIStatus),
					Data: map[string]any{
						"task_id":   t.ID,
						"pr_number": t.PRNumber,
						"ci_status": "failing",
						"check":     checkName,
						"output":    output,
					},
				})
			}
		}
		break
	}
}

// handlePRMerged notifies Prime and marks the relevant GitHubTask complete.
func (gw *Gateway) handlePRMerged(ctx context.Context, payload map[string]any) {
	prNum, _ := nestedFloat(payload, "pull_request", "number")
	branchName, _ := nestedStr2(payload, "pull_request", "head", "ref")
	prURL, _ := nestedStr2(payload, "pull_request", "html_url")
	prTitle, _ := nestedStr2(payload, "pull_request", "title")
	_ = prURL

	tasks := agent.GlobalGitHubTaskQueue.ListAll()
	for _, t := range tasks {
		if t.PRNumber == int(prNum) || t.Branch == branchName {
			agent.GlobalGitHubTaskQueue.Complete(t.ID)
			slog.Info("github.pr.merged", "task", t.ID, "pr", prNum)
			break
		}
	}

	if gw.rtHub != nil {
		gw.rtHub.Broadcast(realtime.Event{
			Type: "github.pr_merged",
			Data: map[string]any{
				"pr_number": int(prNum),
				"pr_title":  prTitle,
				"branch":    branchName,
			},
		})
	}
}

// broadcastPRReadyEvent emits a github.pr_ready WebSocket event so the web UI
// can render an inline PR approval card in Prime's chat feed.
func (gw *Gateway) broadcastPRReadyEvent(ctx context.Context, payload map[string]any, primeID string) {
	if gw.rtHub == nil {
		return
	}
	action, _ := payload["action"].(string)
	if action != "opened" && action != "reopened" && action != "synchronize" {
		return
	}
	prNum, _ := nestedFloat(payload, "pull_request", "number")
	prTitle, _ := nestedStr2(payload, "pull_request", "title")
	prURL, _ := nestedStr2(payload, "pull_request", "html_url")
	headBranch, _ := nestedStr2(payload, "pull_request", "head", "ref")
	baseBranch, _ := nestedStr2(payload, "pull_request", "base", "ref")
	additions, _ := nestedFloat(payload, "pull_request", "additions")
	deletions, _ := nestedFloat(payload, "pull_request", "deletions")
	owner, _ := nestedStr2(payload, "repository", "owner", "login")
	repo, _ := nestedStr2(payload, "repository", "name")

	gw.rtHub.Broadcast(realtime.Event{
		Type: string(apievents.TypeGitHubPRReady),
		Data: map[string]any{
			"pr_number":       int(prNum),
			"pr_title":        prTitle,
			"pr_url":          prURL,
			"head_branch":     headBranch,
			"base_branch":     baseBranch,
			"diff_additions":  int(additions),
			"diff_deletions":  int(deletions),
			"owner":           owner,
			"repo":            repo,
			"ci_status":       "pending",
			"agent_id":        primeID,
		},
	})
	slog.Info("github.pr_ready.broadcast", "pr", prNum, "title", prTitle)
}

func verifyGitHubSignature(body []byte, secret, sigHeader string) bool {
	if !strings.HasPrefix(sigHeader, "sha256=") {
		return false
	}
	expected := sigHeader[7:]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actual := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(actual), []byte(expected))
}

func buildGitHubEventMessage(event string, payload map[string]any) string {
	switch event {
	case "pull_request":
		action, _ := payload["action"].(string)
		switch action {
		case "opened", "reopened":
			prNum, _ := nestedFloat(payload, "pull_request", "number")
			title, _ := nestedStr2(payload, "pull_request", "title")
			body, _ := nestedStr2(payload, "pull_request", "body")
			prURL, _ := nestedStr2(payload, "pull_request", "html_url")
			headBranch, _ := nestedStr2(payload, "pull_request", "head", "ref")
			baseBranch, _ := nestedStr2(payload, "pull_request", "base", "ref")
			author, _ := nestedStr2(payload, "pull_request", "user", "login")
			return fmt.Sprintf(
				"A new pull request was opened.\n\nPR #%d: %s\nURL: %s\nAuthor: @%s\nBranch: %s → %s\n\nDescription:\n%s\n\n"+
					"Review this PR. Check the changes, run the CI checks with gh_list_pr_checks, and when CI passes, "+
					"present it to the user for final approval before merging with gh_merge_pr.",
				int(prNum), title, prURL, author, headBranch, baseBranch, body,
			)
		case "review_requested":
			prNum, _ := nestedFloat(payload, "pull_request", "number")
			title, _ := nestedStr2(payload, "pull_request", "title")
			reviewer, _ := nestedStr2(payload, "requested_reviewer", "login")
			prURL, _ := nestedStr2(payload, "pull_request", "html_url")
			return fmt.Sprintf(
				"A code review was requested on PR #%d: %s\nURL: %s\nRequested reviewer: @%s\n\n"+
					"Review the pull request, provide detailed code review feedback, and post comments on specific lines if needed.",
				int(prNum), title, prURL, reviewer,
			)
		default:
			return ""
		}

	case "issue_comment":
		action, _ := payload["action"].(string)
		if action != "created" {
			return ""
		}
		body, _ := nestedStr2(payload, "comment", "body")
		issueURL, _ := nestedStr2(payload, "issue", "html_url")
		issueTitle, _ := nestedStr2(payload, "issue", "title")
		user, _ := nestedStr2(payload, "comment", "user", "login")
		return "A new comment was posted on GitHub issue.\n\n" +
			"Issue: " + issueTitle + "\nURL: " + issueURL + "\nAuthor: " + user +
			"\n\nComment:\n" + body +
			"\n\nReview this comment. If it describes a bug, create a ticket and assign it to the appropriate developer. If it's a question, respond with a comment on the issue."

	case "pull_request_review_comment":
		body, _ := nestedStr2(payload, "comment", "body")
		prURL, _ := nestedStr2(payload, "pull_request", "html_url")
		prTitle, _ := nestedStr2(payload, "pull_request", "title")
		path, _ := nestedStr2(payload, "comment", "path")
		user, _ := nestedStr2(payload, "comment", "user", "login")
		return "A code review comment was posted on a pull request.\n\n" +
			"PR: " + prTitle + "\nURL: " + prURL + "\nFile: " + path + "\nAuthor: " + user +
			"\n\nComment:\n" + body +
			"\n\nReview this feedback. If it requires code changes, create a ticket and assign it to the developer who wrote the file."

	case "issues":
		action, _ := payload["action"].(string)
		if action != "opened" {
			return ""
		}
		title, _ := nestedStr2(payload, "issue", "title")
		body, _ := nestedStr2(payload, "issue", "body")
		issueURL, _ := nestedStr2(payload, "issue", "html_url")
		issueNum, _ := nestedFloat(payload, "issue", "number")
		labels := extractLabelNames(payload)
		return fmt.Sprintf(
			"A new GitHub issue was opened.\n\nIssue #%d: %s\nURL: %s\nLabels: %s\n\nBody:\n%s\n\n"+
				"Triage this issue: determine severity, call gh_task_register to start the dev loop, create a branch, and assign coding to the right soul.",
			int(issueNum), title, issueURL, strings.Join(labels, ", "), body,
		)

	default:
		return ""
	}
}

func (gw *Gateway) findPrimeAgentID(ctx context.Context) (string, error) {
	var id string
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT id FROM agents WHERE tenant_id = $1 AND (role = 'prime' OR display_name ILIKE 'prime%')
		 ORDER BY created_at LIMIT 1`, defaultTenant).Scan(&id)
	return id, err
}

func nestedStr(m map[string]any, keys ...string) string {
	s, _ := nestedStr2(m, keys...)
	return s
}

func nestedStr2(m map[string]any, keys ...string) (string, bool) {
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			s, ok := cur[k].(string)
			return s, ok
		}
		next, ok := cur[k].(map[string]any)
		if !ok {
			return "", false
		}
		cur = next
	}
	return "", false
}

func nestedFloat(m map[string]any, keys ...string) (float64, bool) {
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			f, ok := cur[k].(float64)
			return f, ok
		}
		next, ok := cur[k].(map[string]any)
		if !ok {
			return 0, false
		}
		cur = next
	}
	return 0, false
}

func nestedBool(m map[string]any, keys ...string) (bool, bool) {
	cur := m
	for i, k := range keys {
		if i == len(keys)-1 {
			b, ok := cur[k].(bool)
			return b, ok
		}
		next, ok := cur[k].(map[string]any)
		if !ok {
			return false, false
		}
		cur = next
	}
	return false, false
}

func extractLabelNames(payload map[string]any) []string {
	issue, ok := payload["issue"].(map[string]any)
	if !ok {
		return nil
	}
	labels, ok := issue["labels"].([]any)
	if !ok {
		return nil
	}
	names := []string{}
	for _, l := range labels {
		if lm, ok := l.(map[string]any); ok {
			if name, ok := lm["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func extractTitle(payload map[string]any) string {
	for _, key := range []string{"title", "issue", "pull_request"} {
		if s, ok := payload[key].(string); ok {
			return s
		}
		if m, ok := payload[key].(map[string]any); ok {
			if t, ok := m["title"].(string); ok {
				return t
			}
		}
	}
	return "event"
}
