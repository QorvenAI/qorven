// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
)

// buildDispatch flips an approved Org project to 'building' and kicks off the
// swarm: requires a connected repo, backfills the project link on tasks, enables
// CTO delegation, and wakes each assigned agent's pending tasks. Best-effort per
// step (logs, doesn't abort) except the repo precondition.
//
// Concurrency note: for v1 the runtime manager serializes per-agent via a
// 32-deep buffered channel; a hard project-wide worker cap is a follow-on item.
func (gw *Gateway) buildDispatch(ctx context.Context, briefID string) error {
	if gw.db == nil || gw.db.Pool == nil {
		return fmt.Errorf("database unavailable")
	}
	var owner, repo, stage string
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT github_owner, github_repo, stage FROM project_briefs WHERE id=$1`, briefID).Scan(&owner, &repo, &stage)
	if owner == "" || repo == "" {
		return fmt.Errorf("org build requires a connected GitHub repo")
	}
	if _, err := gw.db.Pool.Exec(ctx,
		`UPDATE project_briefs SET stage='building', updated_at=NOW() WHERE id=$1`, briefID); err != nil {
		return err
	}
	// Backfill the project link on any tasks created before the column was stamped.
	_, _ = gw.db.Pool.Exec(ctx,
		`UPDATE tasks t SET project_brief_id=$1 FROM agents a
		 WHERE t.assigned_to=a.id AND a.project_brief_id=$1 AND t.project_brief_id IS NULL`, briefID)
	// Enable CTO autonomous delegation.
	_, _ = gw.db.Pool.Exec(ctx,
		`UPDATE agents SET can_delegate=true WHERE project_brief_id=$1 AND org_role='cto'`, briefID)
	// Wake each agent's pending tasks.
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id::text, assigned_to::text FROM tasks
		 WHERE project_brief_id=$1 AND status IN ('backlog','assigned') AND assigned_to IS NOT NULL`, briefID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tid, aid string
			if rows.Scan(&tid, &aid) == nil && gw.runtimeMgr != nil {
				gw.runtimeMgr.WakeAgent(aid, agent.WakeupSignal{Source: agent.WakeupAssignment, TaskID: tid})
				gw.emitProjectEvent(ctx, briefID, "agent_spawned", "Worker activated",
					map[string]any{"agent_id": aid}, tid, aid)
			}
		}
	}
	slog.Info("build_dispatch.started", "brief", briefID, "owner", owner, "repo", repo)
	return nil
}

// handleBriefBuildProject is the HTTP endpoint for manually triggering a build
// dispatch on an inception project brief (POST /v1/project-briefs/{id}/build).
func (gw *Gateway) handleBriefBuildProject(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.buildDispatch(r.Context(), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "building"})
}
