// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// releaseGateRow mirrors the release_gates table.
type releaseGateRow struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	ProjectBriefID string    `json:"project_brief_id"`
	Version        string    `json:"version"`
	ChangelogMd    string    `json:"changelog_md"`
	Status         string    `json:"status"`
	ProposedBy     string    `json:"proposed_by"`
	ApprovedBy     string    `json:"approved_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// proposeRelease creates a release_gates row in status='proposed'.
//
// Version scheme: "v0.1.<N+1>" where N is the count of prior released gates
// for the same brief. First release = v0.1.1, second = v0.1.2, etc.
//
// Changelog is built from merge_queue rows with status='merged' for the brief,
// ordered by updated_at: one markdown list item per merged PR.
func (gw *Gateway) proposeRelease(ctx context.Context, briefID, proposedBy string) (releaseGateRow, error) {
	if gw.db == nil || gw.db.Pool == nil {
		return releaseGateRow{}, fmt.Errorf("database not available")
	}

	// Confirm brief exists and is GitHub-connected (owner/repo must be present).
	var owner, repo string
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(github_owner,''), COALESCE(github_repo,'') FROM project_briefs WHERE id=$1`,
		briefID,
	).Scan(&owner, &repo); err != nil {
		return releaseGateRow{}, fmt.Errorf("project brief not found: %w", err)
	}
	if owner == "" || repo == "" {
		return releaseGateRow{}, fmt.Errorf("project is not connected to a GitHub repository")
	}

	// Count prior released gates to derive the next version.
	var releasedCount int
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM release_gates WHERE project_brief_id=$1 AND status='released'`,
		briefID,
	).Scan(&releasedCount)
	version := fmt.Sprintf("v0.1.%d", releasedCount+1)

	// Gather merged PRs for the brief (simple: from merge_queue rows).
	type mergedPR struct {
		Number int
		Branch string
	}
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT pr_number, branch FROM merge_queue
		 WHERE project_brief_id=$1 AND status='merged'
		 ORDER BY updated_at`,
		briefID,
	)
	if err != nil {
		return releaseGateRow{}, fmt.Errorf("query merged PRs: %w", err)
	}
	defer rows.Close()

	var prs []mergedPR
	for rows.Next() {
		var p mergedPR
		if err := rows.Scan(&p.Number, &p.Branch); err != nil {
			continue
		}
		prs = append(prs, p)
	}

	// Build a simple markdown changelog.
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "## %s\n\n", version)
	if len(prs) == 0 {
		fmt.Fprintf(sb, "- No merged PRs tracked yet.\n")
	} else {
		for _, p := range prs {
			fmt.Fprintf(sb, "- PR #%d (`%s`)\n", p.Number, p.Branch)
		}
	}
	changelogMd := sb.String()

	// Insert the release gate row.
	var gate releaseGateRow
	if err := gw.db.Pool.QueryRow(ctx,
		`INSERT INTO release_gates (tenant_id, project_brief_id, version, changelog_md, status, proposed_by)
		 VALUES ($1, $2, $3, $4, 'proposed', $5)
		 RETURNING id, tenant_id, project_brief_id, version, changelog_md, status, proposed_by, approved_by, created_at, updated_at`,
		defaultTenant, briefID, version, changelogMd, proposedBy,
	).Scan(
		&gate.ID, &gate.TenantID, &gate.ProjectBriefID, &gate.Version,
		&gate.ChangelogMd, &gate.Status, &gate.ProposedBy, &gate.ApprovedBy,
		&gate.CreatedAt, &gate.UpdatedAt,
	); err != nil {
		return releaseGateRow{}, fmt.Errorf("insert release gate: %w", err)
	}

	gw.emitProjectEvent(ctx, briefID, "gate_decision",
		fmt.Sprintf("Release %s proposed", version),
		map[string]any{"release_gate_id": gate.ID, "version": version, "action": "proposed"},
		"", "",
	)
	slog.Info("release_gate.proposed", "brief", briefID, "version", version, "gate", gate.ID)
	return gate, nil
}

// handleProposeRelease serves POST /v1/projects/{id}/release.
// Any authenticated user or agent can trigger a release proposal.
func (gw *Gateway) handleProposeRelease(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	briefID := chi.URLParam(r, "id")

	user := userFromContext(r.Context())
	proposedBy := "user"
	if user != nil {
		proposedBy = user.Username
	}

	gate, err := gw.proposeRelease(r.Context(), briefID, proposedBy)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not connected") {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, gate)
}

// approveRequest is the optional JSON body for the approve endpoint.
// auto_deploy: if true the server triggers a deploy immediately after marking
// the release as 'released' (detached, non-blocking).
// deploy_target: deploy target to use when auto_deploy is true (default "hosted").
type approveRequest struct {
	AutoDeploy   bool   `json:"auto_deploy"`
	DeployTarget string `json:"deploy_target"`
}

// handleApproveRelease serves POST /v1/projects/{id}/release/{releaseId}/approve.
//
// Human gate: requires an authenticated user (401 if not present).
// The gate must be in status='proposed'. On approval it is immediately executed:
// the GitHub release is published. On success the gate moves to 'released'.
// On GitHub API failure the gate stays 'approved' so the caller can retry.
//
// Optional JSON body: {"auto_deploy": true, "deploy_target": "hosted"}.
// When auto_deploy is false (default), a gate_decision event is emitted so the
// UI can surface the Deploy button.
func (gw *Gateway) handleApproveRelease(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	// ── Human gate: only an authenticated human can approve a release ────────
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	briefID := chi.URLParam(r, "id")
	releaseID := chi.URLParam(r, "releaseId")
	ctx := r.Context()

	// Decode optional body (auto_deploy / deploy_target). Tolerate empty body.
	var approveReq approveRequest
	json.NewDecoder(r.Body).Decode(&approveReq) //nolint:errcheck
	deployTarget := approveReq.DeployTarget
	if deployTarget == "" {
		deployTarget = "hosted"
	}

	// Load the gate and verify it belongs to this project.
	var gate releaseGateRow
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT id, tenant_id, project_brief_id, version, changelog_md, status, proposed_by, approved_by, created_at, updated_at
		 FROM release_gates WHERE id=$1 AND project_brief_id=$2`,
		releaseID, briefID,
	).Scan(
		&gate.ID, &gate.TenantID, &gate.ProjectBriefID, &gate.Version,
		&gate.ChangelogMd, &gate.Status, &gate.ProposedBy, &gate.ApprovedBy,
		&gate.CreatedAt, &gate.UpdatedAt,
	); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "release gate not found"})
		return
	}
	if gate.Status != "proposed" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": fmt.Sprintf("release gate is in status %q, expected 'proposed'", gate.Status),
		})
		return
	}

	// Mark approved.
	approvedBy := user.Username
	if _, err := gw.db.Pool.Exec(ctx,
		`UPDATE release_gates SET status='approved', approved_by=$1, updated_at=NOW() WHERE id=$2`,
		approvedBy, releaseID,
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	gate.Status = "approved"
	gate.ApprovedBy = approvedBy

	// Resolve owner/repo for the brief.
	var owner, repo string
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(github_owner,''), COALESCE(github_repo,'') FROM project_briefs WHERE id=$1`,
		briefID,
	).Scan(&owner, &repo); err != nil || owner == "" || repo == "" {
		// GitHub not configured — return the approved gate; the caller can retry
		// once the project is connected.
		writeJSON(w, http.StatusOK, gate)
		return
	}

	// Execute the release via GitHub Releases API.
	ghPayload := map[string]any{
		"tag_name": gate.Version,
		"name":     gate.Version,
		"body":     gate.ChangelogMd,
	}
	rawResp, ghStatus, ghErr := gw.ghPost(ctx,
		fmt.Sprintf("/repos/%s/%s/releases", owner, repo),
		ghPayload,
	)

	if ghErr != nil || ghStatus >= 400 {
		// Leave status='approved' so it can be retried without re-proposing.
		var ghMsg string
		if ghErr != nil {
			ghMsg = ghErr.Error()
		} else {
			var e struct {
				Message string `json:"message"`
			}
			json.Unmarshal(rawResp, &e)
			ghMsg = e.Message
			if ghMsg == "" {
				ghMsg = fmt.Sprintf("GitHub API error %d", ghStatus)
			}
		}
		slog.Warn("release_gate.github_error",
			"gate", releaseID, "version", gate.Version, "err", ghMsg)
		writeJSON(w, http.StatusOK, map[string]any{
			"gate":         gate,
			"github_error": ghMsg,
		})
		return
	}

	// GitHub call succeeded — mark released.
	if _, err := gw.db.Pool.Exec(ctx,
		`UPDATE release_gates SET status='released', updated_at=NOW() WHERE id=$1`,
		releaseID,
	); err != nil {
		slog.Error("release_gate.mark_released_failed", "gate", releaseID, "err", err)
	}
	gate.Status = "released"

	gw.emitProjectEvent(ctx, briefID, "gate_decision",
		fmt.Sprintf("Release %s published", gate.Version),
		map[string]any{
			"release_gate_id": gate.ID,
			"version":         gate.Version,
			"action":          "released",
			"approved_by":     approvedBy,
		},
		"", "",
	)
	slog.Info("release_gate.released", "brief", briefID, "version", gate.Version, "gate", releaseID)

	// ── Post-release deploy hook ─────────────────────────────────────────────
	if approveReq.AutoDeploy && gw.deployReg != nil {
		// Detached context: the HTTP request will return immediately; the deploy
		// runs in the background and emits its own deploy_started / deploy_live /
		// deploy_failed events.
		go gw.deployReleasedVersion(context.Background(), briefID, releaseID, gate.Version, deployTarget)
	} else {
		// Manual-deploy path: emit a gate_decision event so the UI can surface
		// the Deploy button for this release.
		gw.emitProjectEvent(ctx, briefID, "gate_decision",
			fmt.Sprintf("Release %s ready to deploy", gate.Version),
			map[string]any{
				"release_gate_id": gate.ID,
				"version":         gate.Version,
				"action":          "ready_to_deploy",
				"deploy_target":   deployTarget,
			},
			"", "",
		)
	}

	writeJSON(w, http.StatusOK, gate)
}

// deployReleasedVersion deploys the just-released version of a project to the
// chosen target, stamping the deployment with the originating release for lineage.
// Called in a detached goroutine from handleApproveRelease when auto_deploy=true.
func (gw *Gateway) deployReleasedVersion(ctx context.Context, briefID, releaseID, version, target string) {
	if gw.projectReg == nil || gw.deployReg == nil {
		slog.Warn("deploy_released.skip", "reason", "registry not ready", "release", releaseID)
		return
	}

	// Look up the project via the brief ID.
	project := gw.projectReg.GetByBriefID(briefID)
	if project == nil {
		slog.Warn("deploy_released.skip", "reason", "project not found for brief", "brief", briefID, "release", releaseID)
		return
	}

	slog.Info("deploy_released.start", "brief", briefID, "release", releaseID, "version", version, "target", target)

	// startDeploy creates the Deployment record (with release_id stamped), launches
	// the goroutine, and returns immediately. The version tag is passed as
	// ReleaseTag so cloud targets can check out the exact ref.
	gw.startDeploy(project, target, releaseID, version)
}

// handleListReleases serves GET /v1/projects/{id}/releases.
// Returns all release_gates rows for the brief, ordered newest-first.
func (gw *Gateway) handleListReleases(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	briefID := chi.URLParam(r, "id")

	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, tenant_id, project_brief_id, version, changelog_md, status, proposed_by, approved_by, created_at, updated_at
		 FROM release_gates
		 WHERE project_brief_id=$1
		 ORDER BY created_at DESC`,
		briefID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	gates := []releaseGateRow{}
	for rows.Next() {
		var g releaseGateRow
		if err := rows.Scan(
			&g.ID, &g.TenantID, &g.ProjectBriefID, &g.Version,
			&g.ChangelogMd, &g.Status, &g.ProposedBy, &g.ApprovedBy,
			&g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			continue
		}
		gates = append(gates, g)
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": gates})
}
