// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/qorvenai/qorven/internal/agent"
)

// ProjectArtifact is a typed, versioned, gated pipeline document.
type ProjectArtifact struct {
	ID            string     `json:"id"`
	BriefID       string     `json:"brief_id"`
	Type          string     `json:"type"`
	Version       int        `json:"version"`
	ContentMD     string     `json:"content_md"`
	Status        string     `json:"status"`
	RepoCommitted bool       `json:"repo_committed"`
	CreatedBy     string     `json:"created_by"`
	ApprovedBy    *string    `json:"approved_by,omitempty"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func nextArtifactVersion(prev *int) int {
	if prev == nil { return 1 }
	return *prev + 1
}

func artifactRepoPath(typ string) string { return "docs/" + typ + ".md" }

// listArtifacts returns all active (non-superseded) artifacts for a brief.
func (gw *Gateway) listArtifacts(ctx context.Context, briefID string) ([]ProjectArtifact, error) {
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, brief_id, type, version, content_md, status, repo_committed,
		        created_by, approved_by, approved_at, created_at
		 FROM project_artifacts
		 WHERE brief_id = $1 AND status <> 'superseded'
		 ORDER BY CASE type WHEN 'prd' THEN 1 WHEN 'spec' THEN 2 WHEN 'design' THEN 3 WHEN 'resource_plan' THEN 4 ELSE 5 END`, briefID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []ProjectArtifact{}
	for rows.Next() {
		var a ProjectArtifact
		if err := rows.Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// getActiveArtifact returns the active artifact of a type, or (nil,nil) if none.
func (gw *Gateway) getActiveArtifact(ctx context.Context, briefID, typ string) (*ProjectArtifact, error) {
	var a ProjectArtifact
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT id, brief_id, type, version, content_md, status, repo_committed,
		        created_by, approved_by, approved_at, created_at
		 FROM project_artifacts
		 WHERE brief_id = $1 AND type = $2 AND status <> 'superseded'`, briefID, typ).
		Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// upsertArtifactRevision supersedes any active artifact of this type and inserts
// a fresh draft (status in_review) as the next version.
func (gw *Gateway) upsertArtifactRevision(ctx context.Context, briefID, typ, contentMD string) (*ProjectArtifact, error) {
	prev, _ := gw.getActiveArtifact(ctx, briefID, typ)
	var prevV *int
	if prev != nil { prevV = &prev.Version }
	if prev != nil {
		if _, err := gw.db.Pool.Exec(ctx,
			`UPDATE project_artifacts SET status='superseded' WHERE id=$1`, prev.ID); err != nil {
			return nil, err
		}
	}
	var a ProjectArtifact
	err := gw.db.Pool.QueryRow(ctx,
		`INSERT INTO project_artifacts (tenant_id, brief_id, type, version, content_md, status, created_by)
		 VALUES ($1,$2,$3,$4,$5,'in_review','cto')
		 RETURNING id, brief_id, type, version, content_md, status, repo_committed,
		           created_by, approved_by, approved_at, created_at`,
		defaultTenant, briefID, typ, nextArtifactVersion(prevV), contentMD).
		Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt)
	if err != nil { return nil, err }
	return &a, nil
}

// handleClarify runs one CTO (system-architect) clarification turn. The CTO asks
// targeted questions until it has enough to draft the PRD; it never drafts here.
// POST /v1/project-briefs/{id}/clarify  {message, history}
func (gw *Gateway) handleClarify(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Message string                            `json:"message"`
		History []struct{ Role, Content string } `json:"history"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	brief, err := gw.readBrief(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "brief not found"})
		return
	}

	prompt := buildClarifyPrompt(brief, body.History, body.Message)
	var collected strings.Builder
	if gw.agentLoop != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		_, _ = gw.agentLoop.Run(ctx, agent.RunRequest{
			AgentID: "prime", SessionID: "clarify-" + id, UserMessage: prompt,
			Channel: "plan_graph", Stream: true, NoPersist: true, TenantID: defaultTenant,
		}, func(ev agent.StreamEvent) {
			if ev.Type == "text_delta" && ev.Delta != "" {
				collected.WriteString(ev.Delta)
			}
		})
	}
	reply := strings.TrimSpace(collected.String())
	if reply == "" {
		reply = "Could you share more detail on the core feature, the users, and any must-have integrations?"
	}
	if brief.Stage == "intake" {
		_, _ = gw.db.Pool.Exec(r.Context(),
			`UPDATE project_briefs SET stage='clarify', updated_at=now() WHERE id=$1`, id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"reply": reply})
}

func buildClarifyPrompt(b *ProjectBrief, history []struct{ Role, Content string }, msg string) string {
	var sb strings.Builder
	sb.WriteString("You are the CTO of a software org, acting as a senior system architect. ")
	sb.WriteString("You are clarifying requirements with the user BEFORE writing a PRD. ")
	sb.WriteString("Ask focused, high-signal questions (scope, users, must-have features, integrations, constraints). ")
	sb.WriteString("Do NOT write the PRD yet. Ask at most 2-3 questions per turn. When you have enough, say: 'I have enough to draft the PRD.'\n\n")
	fmt.Fprintf(&sb, "Project: %s\nIdea: %s\nStack: %s\nQuality: %s\n\n", b.Title, b.Idea, b.Stack, b.Quality)
	for _, h := range history {
		fmt.Fprintf(&sb, "%s: %s\n", h.Role, h.Content)
	}
	if msg != "" {
		fmt.Fprintf(&sb, "user: %s\n", msg)
	}
	sb.WriteString("\ncto:")
	return sb.String()
}

// readBrief loads a single brief (incl. stage/mode) for handlers.
func (gw *Gateway) readBrief(ctx context.Context, id string) (*ProjectBrief, error) {
	var b ProjectBrief
	var proposalJSON []byte
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT id, tenant_id, title, idea, stack, budget_cents, timeline, quality,
		        status, proposal, goal_id, created_at, updated_at, stage, mode
		 FROM project_briefs WHERE id=$1 AND tenant_id=$2`, id, defaultTenant).
		Scan(&b.ID, &b.TenantID, &b.Title, &b.Idea, &b.Stack, &b.BudgetCents, &b.Timeline,
			&b.Quality, &b.Status, &proposalJSON, &b.GoalID, &b.CreatedAt, &b.UpdatedAt, &b.Stage, &b.Mode)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// artifactPromptFor builds the CTO generation prompt for an artifact type,
// grounding it in the brief + the prior approved artifact.
func (gw *Gateway) artifactPromptFor(ctx context.Context, b *ProjectBrief, typ string) string {
	var sb strings.Builder
	titles := map[string]string{
		"prd":    "Product Requirements Document (PRD)",
		"spec":   "Technical Specification",
		"design": "System Design Document",
	}
	fmt.Fprintf(&sb, "You are the CTO. Write a complete %s in Markdown for this project. Be concrete and implementation-ready. Output ONLY the markdown document.\n\n", titles[typ])
	fmt.Fprintf(&sb, "Project: %s\nIdea: %s\nStack: %s\nQuality: %s\n\n", b.Title, b.Idea, b.Stack, b.Quality)
	prior := map[string]string{"spec": "prd", "design": "spec"}
	if p, ok := prior[typ]; ok {
		if a, _ := gw.getActiveArtifact(ctx, b.ID, p); a != nil && a.Status == "approved" {
			fmt.Fprintf(&sb, "Build on the approved %s:\n\n%s\n", p, a.ContentMD)
		}
	}
	// If a prior draft of this artifact was sent back with revision feedback, include it.
	if prevMD, _ := gw.getLatestNeedsReview(ctx, b.ID, typ); prevMD != "" {
		fmt.Fprintf(&sb, "\nThe previous draft was returned with this feedback — address it:\n%s\n", prevMD)
	}
	return sb.String()
}

// getLatestNeedsReview returns the content_md of the most recent needs_review
// (or superseded) artifact of a type, for feeding revision feedback into regen.
func (gw *Gateway) getLatestNeedsReview(ctx context.Context, briefID, typ string) (string, error) {
	var md string
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT content_md FROM project_artifacts
		 WHERE brief_id=$1 AND type=$2 AND status IN ('needs_review','superseded')
		 ORDER BY version DESC LIMIT 1`, briefID, typ).Scan(&md)
	if err != nil { return "", err }
	return md, nil
}

// handleGenerateArtifact has the CTO draft an artifact (prd|spec|design).
// POST /v1/project-briefs/{id}/artifacts/{type}/generate
func (gw *Gateway) handleGenerateArtifact(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"}); return }
	id := chi.URLParam(r, "id")
	typ := chi.URLParam(r, "type")
	if typ != "prd" && typ != "spec" && typ != "design" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be prd, spec, or design"}); return
	}
	brief, err := gw.readBrief(r.Context(), id)
	if err != nil { writeJSON(w, http.StatusNotFound, map[string]string{"error": "brief not found"}); return }

	statuses, _ := gw.artifactStatusMap(r.Context(), id)
	if !CanAdvanceTo(typ, statuses) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "previous artifact not approved"}); return
	}

	if cur, _ := gw.getActiveArtifact(r.Context(), id, typ); cur != nil && cur.Status == "approved" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "artifact already approved; use request-changes first"})
		return
	}

	prompt := gw.artifactPromptFor(r.Context(), brief, typ)
	sid := fmt.Sprintf("artifact-%s-%s", typ, id)
	if cur, _ := gw.getActiveArtifact(r.Context(), id, typ); cur != nil {
		sid = fmt.Sprintf("%s-v%d", sid, cur.Version+1)
	}
	var collected strings.Builder
	if gw.agentLoop != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		_, _ = gw.agentLoop.Run(ctx, agent.RunRequest{
			AgentID: "prime", SessionID: sid, UserMessage: prompt,
			Channel: "plan_graph", Stream: true, NoPersist: true, TenantID: defaultTenant,
		}, func(ev agent.StreamEvent) {
			if ev.Type == "text_delta" && ev.Delta != "" { collected.WriteString(ev.Delta) }
		})
	}
	md := strings.TrimSpace(collected.String())
	if md == "" { writeJSON(w, http.StatusBadGateway, map[string]string{"error": "generation failed, retry"}); return }

	art, err := gw.upsertArtifactRevision(r.Context(), id, typ, md)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)}); return }
	_, _ = gw.db.Pool.Exec(r.Context(), `UPDATE project_briefs SET stage=$2, updated_at=now() WHERE id=$1`, id, typ)
	writeJSON(w, http.StatusOK, art)
}

// handleApproveArtifact approves the active artifact and advances the stage.
// POST /v1/project-briefs/{id}/artifacts/{type}/approve
func (gw *Gateway) handleApproveArtifact(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"}); return }
	id := chi.URLParam(r, "id")
	typ := chi.URLParam(r, "type")
	art, _ := gw.getActiveArtifact(r.Context(), id, typ)
	if art == nil { writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active artifact"}); return }
	approver := "user"
	if u := userFromContext(r.Context()); u != nil { approver = u.Username }
	if _, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE project_artifacts SET status='approved', approved_by=$2, approved_at=now() WHERE id=$1`,
		art.ID, approver); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)}); return
	}
	_, _ = gw.db.Pool.Exec(r.Context(), `UPDATE project_briefs SET stage=$2, updated_at=now() WHERE id=$1`, id, NextStage(typ))
	gw.commitArtifactToRepo(r.Context(), id, typ, art.ContentMD)
	updated, err := gw.getActiveArtifact(r.Context(), id, typ)
	if err != nil || updated == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approved but failed to reload artifact"})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleRequestChanges records feedback and cascades downstream artifacts to needs_review.
// POST /v1/project-briefs/{id}/artifacts/{type}/request-changes  {feedback}
func (gw *Gateway) handleRequestChanges(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"}); return }
	id := chi.URLParam(r, "id")
	typ := chi.URLParam(r, "type")
	var body struct{ Feedback string `json:"feedback"` }
	_ = json.NewDecoder(r.Body).Decode(&body)
	art, _ := gw.getActiveArtifact(r.Context(), id, typ)
	if art == nil { writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active artifact"}); return }
	_, _ = gw.db.Pool.Exec(r.Context(), `UPDATE project_artifacts SET status='needs_review' WHERE id=$1`, art.ID)
	// Store feedback appended to content_md so the next generate can incorporate it.
	if body.Feedback != "" {
		_, _ = gw.db.Pool.Exec(r.Context(),
			`UPDATE project_artifacts SET content_md = content_md || E'\n\n<!-- REVISION FEEDBACK: ' || $2 || E' -->' WHERE id=$1`,
			art.ID, body.Feedback)
	}
	for _, d := range DownstreamArtifacts(typ) {
		if da, _ := gw.getActiveArtifact(r.Context(), id, d); da != nil && da.Status == "approved" {
			_, _ = gw.db.Pool.Exec(r.Context(), `UPDATE project_artifacts SET status='needs_review' WHERE id=$1`, da.ID)
		}
	}
	_, _ = gw.db.Pool.Exec(r.Context(), `UPDATE project_briefs SET stage=$2, updated_at=now() WHERE id=$1`, id, typ)
	writeJSON(w, http.StatusOK, map[string]any{"status": "needs_review", "type": typ, "downstream_reopened": DownstreamArtifacts(typ)})
}

func (gw *Gateway) artifactStatusMap(ctx context.Context, briefID string) (map[string]string, error) {
	arts, err := gw.listArtifacts(ctx, briefID)
	m := map[string]string{}
	for _, a := range arts { m[a.Type] = a.Status }
	return m, err
}

// commitArtifactToRepo writes docs/<type>.md into the project's workspace repo
// and commits it. Best-effort: DB is the source of truth. No-op until a repo is
// connected (Org-mode GitHub connect lands in 8C). Marks repo_committed accordingly.
func (gw *Gateway) commitArtifactToRepo(ctx context.Context, briefID, typ, contentMD string) {
	// 8A: repo wiring lands with GitHub-required Org projects in 8C. For now this
	// is a no-op placeholder; the artifact stays repo_committed=false. Keeping the
	// call site here means 8C only fills the body.
	_ = ctx; _ = briefID; _ = typ; _ = contentMD
}

// handleListProjectArtifacts returns the active artifacts + the project's stage.
// GET /v1/project-briefs/{id}/artifacts
func (gw *Gateway) handleListProjectArtifacts(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"}); return }
	id := chi.URLParam(r, "id")
	brief, err := gw.readBrief(r.Context(), id)
	if err != nil { writeJSON(w, http.StatusNotFound, map[string]string{"error": "brief not found"}); return }
	arts, err := gw.listArtifacts(r.Context(), id)
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)}); return }
	writeJSON(w, http.StatusOK, map[string]any{"stage": brief.Stage, "mode": brief.Mode, "artifacts": arts})
}
