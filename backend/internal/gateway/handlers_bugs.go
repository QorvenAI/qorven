// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// handleReportBug lets a user file a bug against a deployed project; it funnels
// into the autonomous fix-loop (auto-issue → CTO triage → fix).
//
//	POST /v1/projects/{id}/bugs  {title, body}
func (gw *Gateway) handleReportBug(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if userFromContext(r.Context()) == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	projectID := chi.URLParam(r, "id")
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Title) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}

	// Resolve the brief that backs this project. CodeProject.InceptionBriefID is
	// stamped at creation time and is the canonical project→brief mapping.
	briefID := ""
	if gw.projectReg != nil {
		if p := gw.projectReg.Get(projectID); p != nil {
			briefID = p.InceptionBriefID
		}
	}
	if briefID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project not linked to a repo-backed brief"})
		return
	}

	// Dedup key from a slug of the title so duplicate reports of the same bug coalesce.
	ref := "bug-" + slugifyBug(body.Title)
	// Use context.Background() so an early client disconnect does not cancel the
	// GitHub API call inside triggerFixLoop.
	gw.triggerFixLoop(r.Context(), briefID, "bug", ref, "Bug: "+body.Title, body.Body)
	writeJSON(w, http.StatusOK, map[string]string{"status": "filed"})
}

// slugifyBug makes a stable dedup ref from a bug title.
func slugifyBug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' && b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	out := b.String()
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
