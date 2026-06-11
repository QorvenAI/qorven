// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
)

// mergeQueueRow mirrors the merge_queue table row.
type mergeQueueRow struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	ProjectBriefID string     `json:"project_brief_id"`
	TaskID         *string    `json:"task_id,omitempty"`
	PRNumber       int        `json:"pr_number"`
	Branch         string     `json:"branch"`
	BaseSHA        string     `json:"base_sha"`
	Status         string     `json:"status"`
	Attempt        int        `json:"attempt"`
	Detail         string     `json:"detail"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// enqueueMerge inserts a merge_queue row with status='queued'.
// Idempotent: silently skips if a queued or merging row already exists for
// (project_brief_id, pr_number).
func (gw *Gateway) enqueueMerge(ctx context.Context, briefID string, prNumber int, branch string, taskID *string) error {
	if gw.db == nil || gw.db.Pool == nil {
		return fmt.Errorf("database unavailable")
	}

	// Skip if an active row already exists.
	var existing int
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM merge_queue
		 WHERE project_brief_id=$1 AND pr_number=$2 AND status IN ('queued','merging')`,
		briefID, prNumber,
	).Scan(&existing)
	if existing > 0 {
		slog.Info("merge_queue.enqueue.skip_existing", "brief", briefID, "pr", prNumber)
		return nil
	}

	_, err := gw.db.Pool.Exec(ctx,
		`INSERT INTO merge_queue (tenant_id, project_brief_id, task_id, pr_number, branch, status)
		 VALUES ($1, $2, $3::uuid, $4, $5, 'queued')`,
		defaultTenant, briefID, nullStr(ptrStrVal(taskID)), prNumber, branch,
	)
	if err != nil {
		return fmt.Errorf("enqueue merge: %w", err)
	}
	slog.Info("merge_queue.enqueued", "brief", briefID, "pr", prNumber, "branch", branch)
	return nil
}

// processMergeQueue processes pending merge_queue rows for a project one at a time.
//
// Lock pattern: claim-then-release (safer than tx-held).
//   1. Take a short advisory lock scoped to the project to claim exactly one
//      queued row (status → merging), then commit to release the lock.
//   2. Do the GitHub PUT merge call entirely outside any transaction.
//   3. Update status in a second short transaction.
//
// This means the advisory lock is held only for the brief claim-and-mark step,
// not for the full network round-trip. Multiple callers for the same project
// racing on step 1 will serialize safely: whichever wins claims the row and
// the loser finds nothing to process and exits.
func (gw *Gateway) processMergeQueue(ctx context.Context, briefID string) {
	if gw.db == nil || gw.db.Pool == nil {
		return
	}

	// Look up owner/repo for the brief (needed for the GitHub API call).
	var owner, repo string
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT github_owner, github_repo FROM project_briefs WHERE id=$1`, briefID,
	).Scan(&owner, &repo); err != nil || owner == "" || repo == "" {
		slog.Warn("merge_queue.process.no_repo", "brief", briefID)
		return
	}

	for {
		// --- Phase 1: claim one row under advisory lock (short tx) ---
		var rowID string
		var prNumber int
		var branch string

		tx, err := gw.db.Pool.Begin(ctx)
		if err != nil {
			slog.Error("merge_queue.process.tx_begin", "err", err)
			return
		}

		// Advisory lock scoped to this project's merge queue.
		lockKey := fmt.Sprintf("merge_queue:%s", briefID)
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, lockKey); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			slog.Error("merge_queue.process.lock", "err", err)
			return
		}

		err = tx.QueryRow(ctx,
			`UPDATE merge_queue
			 SET status='merging', attempt=attempt+1, updated_at=NOW()
			 WHERE id = (
			   SELECT id FROM merge_queue
			   WHERE project_brief_id=$1 AND status='queued'
			   ORDER BY created_at
			   LIMIT 1
			   FOR UPDATE SKIP LOCKED
			 )
			 RETURNING id, pr_number, branch`,
			briefID,
		).Scan(&rowID, &prNumber, &branch)

		if err != nil {
			// No more queued rows — nothing to do.
			tx.Rollback(ctx) //nolint:errcheck
			return
		}

		if err := tx.Commit(ctx); err != nil {
			slog.Error("merge_queue.process.commit_claim", "err", err)
			return
		}

		// --- Phase 2: call GitHub PUT /repos/{owner}/{repo}/pulls/{n}/merge ---
		httpStatus, mergeErr := gw.doGitHubMerge(ctx, owner, repo, prNumber)

		// --- Phase 3: update row status in a second short transaction ---
		switch {
		case mergeErr == nil:
			// 200: merged successfully.
			_, _ = gw.db.Pool.Exec(ctx,
				`UPDATE merge_queue SET status='merged', detail='', updated_at=NOW() WHERE id=$1`,
				rowID,
			)
			slog.Info("merge_queue.merged", "brief", briefID, "pr", prNumber)
			gw.emitProjectEvent(ctx, briefID, "pr_merged",
				fmt.Sprintf("PR #%d merged", prNumber),
				map[string]any{"pr_number": prNumber, "branch": branch},
				"", "",
			)

		case httpStatus == 405 || httpStatus == 409:
			// 405 = Not Allowed (merge conflict) / 409 = Conflict.
			detail := mergeErr.Error()
			_, _ = gw.db.Pool.Exec(ctx,
				`UPDATE merge_queue SET status='conflict', detail=$1, updated_at=NOW() WHERE id=$2`,
				detail, rowID,
			)
			slog.Warn("merge_queue.conflict", "brief", briefID, "pr", prNumber, "detail", detail)
			gw.dispatchConflictWorker(ctx, briefID, prNumber, branch, rowID)

		default:
			// Other error (network, auth, etc.) — mark failed and continue to next row.
			detail := mergeErr.Error()
			_, _ = gw.db.Pool.Exec(ctx,
				`UPDATE merge_queue SET status='failed', detail=$1, updated_at=NOW() WHERE id=$2`,
				detail, rowID,
			)
			slog.Error("merge_queue.failed", "brief", briefID, "pr", prNumber, "status", httpStatus, "detail", detail)
		}
	}
}

// doGitHubMerge performs the GitHub PUT /repos/{owner}/{repo}/pulls/{n}/merge call.
// Returns (httpStatus, error). On success returns (200, nil).
func (gw *Gateway) doGitHubMerge(ctx context.Context, owner, repo string, prNumber int) (int, error) {
	tok := gw.ghVaultToken(ctx)
	if tok == "" {
		return 401, fmt.Errorf("no GitHub token configured")
	}

	payload := map[string]any{"merge_method": "squash"}
	payloadBytes, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/merge", owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "PUT", apiURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return 500, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 500, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode >= 400 {
		var ghErr struct {
			Message string `json:"message"`
		}
		json.Unmarshal(respBody, &ghErr)
		msg := ghErr.Message
		if msg == "" {
			msg = fmt.Sprintf("GitHub API error %d", resp.StatusCode)
		}
		return resp.StatusCode, fmt.Errorf("%s", msg)
	}
	return resp.StatusCode, nil
}

// dispatchConflictWorker creates a task asking an agent to resolve the merge conflict
// and wakes the project's prime agent (if one is assigned) to pick it up.
func (gw *Gateway) dispatchConflictWorker(ctx context.Context, briefID string, prNumber int, branch, queueRowID string) {
	if gw.db == nil || gw.db.Pool == nil {
		return
	}

	title := fmt.Sprintf("Resolve merge conflict in PR #%d", prNumber)
	desc := fmt.Sprintf(
		"PR #%d (branch: %s) could not be merged automatically due to conflicts. "+
			"Checkout the branch, resolve conflicts against base, push, and re-enqueue the merge. "+
			"Queue row ID: %s",
		prNumber, branch, queueRowID,
	)

	var taskID string
	if err := gw.db.Pool.QueryRow(ctx,
		`INSERT INTO tasks (tenant_id, title, description, status, priority, project_brief_id)
		 VALUES ($1, $2, $3, 'pending', 3, $4::uuid) RETURNING id`,
		defaultTenant, title, desc, briefID,
	).Scan(&taskID); err != nil {
		slog.Error("merge_queue.conflict_worker.task_create_failed", "err", err, "brief", briefID, "pr", prNumber)
		return
	}
	slog.Info("merge_queue.conflict_worker.task_created", "task", taskID, "brief", briefID, "pr", prNumber)

	// Attach the task_id to the queue row for traceability.
	_, _ = gw.db.Pool.Exec(ctx,
		`UPDATE merge_queue SET task_id=$1::uuid WHERE id=$2`,
		taskID, queueRowID,
	)

	// Emit a project event so the Hub surface shows the conflict.
	gw.emitProjectEvent(ctx, briefID, "merge_conflict",
		fmt.Sprintf("Merge conflict in PR #%d", prNumber),
		map[string]any{"pr_number": prNumber, "branch": branch, "task_id": taskID},
		taskID, "",
	)

	// Find the agent assigned to this project brief and wake them.
	if gw.runtimeMgr == nil {
		return
	}
	var agentID string
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT id::text FROM agents WHERE project_brief_id=$1 AND org_role='cto' LIMIT 1`,
		briefID,
	).Scan(&agentID)
	if agentID == "" {
		// Fall back to prime.
		agentID, _ = gw.findPrimeAgentID(ctx)
	}
	if agentID != "" {
		gw.runtimeMgr.WakeAgent(agentID, agent.WakeupSignal{
			Source: agent.WakeupAssignment,
			TaskID: taskID,
		})
		slog.Info("merge_queue.conflict_worker.woke_agent", "agent", agentID, "task", taskID)
	}
}

// getMergeQueue returns all merge_queue rows for a project, ordered by created_at.
func (gw *Gateway) getMergeQueue(ctx context.Context, briefID string) ([]mergeQueueRow, error) {
	if gw.db == nil || gw.db.Pool == nil {
		return nil, fmt.Errorf("database unavailable")
	}

	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, tenant_id, project_brief_id,
		        task_id::text,
		        pr_number, branch, base_sha, status, attempt, detail,
		        created_at, updated_at
		 FROM merge_queue
		 WHERE project_brief_id=$1
		 ORDER BY created_at`,
		briefID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []mergeQueueRow{}
	for rows.Next() {
		var r mergeQueueRow
		var taskIDStr *string
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.ProjectBriefID,
			&taskIDStr,
			&r.PRNumber, &r.Branch, &r.BaseSHA, &r.Status, &r.Attempt, &r.Detail,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.TaskID = taskIDStr
		out = append(out, r)
	}
	return out, rows.Err()
}

// handleGetMergeQueue serves GET /v1/projects/{id}/merge-queue.
func (gw *Gateway) handleGetMergeQueue(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	briefID := chi.URLParam(r, "id")
	rows, err := gw.getMergeQueue(r.Context(), briefID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"queue": rows})
}

// ptrStrVal dereferences a *string safely.
func ptrStrVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
