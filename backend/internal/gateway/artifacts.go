// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
		 ORDER BY type`, briefID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []ProjectArtifact{}
	for rows.Next() {
		var a ProjectArtifact
		if err := rows.Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt); err != nil {
			continue
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
	if err != nil { return nil, nil }
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
