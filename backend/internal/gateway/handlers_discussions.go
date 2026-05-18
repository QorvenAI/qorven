// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/discussion"
)

// GET /v1/agents/{id}/discussions
func (gw *Gateway) handleListDiscussions(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	discs, err := gw.discussionStore.ListForAgent(r.Context(), defaultTenant, agentID, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if discs == nil {
		discs = []discussion.Discussion{}
	}
	writeJSON(w, 200, map[string]any{"discussions": discs})
}

// PUT /v1/agents/{id}/discussions/{discussionId}
func (gw *Gateway) handleUpdateDiscussion(w http.ResponseWriter, r *http.Request) {
	discussionID := chi.URLParam(r, "discussionId")
	var body struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" {
		writeJSON(w, 400, map[string]string{"error": "label required"})
		return
	}
	if err := gw.discussionStore.SetUserLabel(r.Context(), defaultTenant, discussionID, body.Label); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// GET /v1/agents/{id}/messages
// Returns merged conversation across all sessions for this agent, newest-first, paginated.
func (gw *Gateway) handleAgentMessages(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	before := r.URL.Query().Get("before") // ISO timestamp cursor, optional
	limit := 50

	query := `
        SELECT
            m.value->>'role'                         AS role,
            m.value->>'content'                      AS content,
            s.id                                     AS session_id,
            s.discussion_id::text                    AS discussion_id,
            s.source_channel                         AS source_channel,
            COALESCE(m.value->>'ts', s.created_at::text) AS ts
        FROM sessions s,
             jsonb_array_elements(s.messages) m
        WHERE s.tenant_id = $1
          AND s.agent_id = $2
          AND ($3::text IS NULL OR COALESCE(m.value->>'ts', s.created_at::text) < $3)
        ORDER BY ts DESC
        LIMIT $4`

	var beforeVal any
	if before != "" {
		beforeVal = before
	}

	rows, err := gw.db.Pool.Query(r.Context(), query, defaultTenant, agentID, beforeVal, limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type msgRow struct {
		Role          string  `json:"role"`
		Content       string  `json:"content"`
		SessionID     string  `json:"session_id"`
		DiscussionID  *string `json:"discussion_id,omitempty"`
		SourceChannel string  `json:"source_channel"`
		Ts            string  `json:"ts,omitempty"`
	}

	var msgs []msgRow
	for rows.Next() {
		var m msgRow
		if err := rows.Scan(&m.Role, &m.Content, &m.SessionID, &m.DiscussionID, &m.SourceChannel, &m.Ts); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []msgRow{}
	}
	writeJSON(w, 200, map[string]any{"messages": msgs})
}

// assignDiscussionAsync runs topic clustering for a session in the background.
// It is safe to call from a goroutine — all errors are silently swallowed.
func (gw *Gateway) assignDiscussionAsync(ctx context.Context, agentID, sessionID, excerpt string) {
	if gw.clusterer == nil || sessionID == "" || agentID == "" {
		return
	}
	clusterCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if len(excerpt) > 500 {
		excerpt = excerpt[:500]
	}
	_, _, _ = gw.clusterer.AssignDiscussion(clusterCtx, agentID, defaultTenant, sessionID, excerpt)
	// Also stamp the source_channel on the session row when it is not yet set.
	gw.db.Pool.Exec(clusterCtx,
		`UPDATE sessions SET source_channel = 'web' WHERE id = $1 AND (source_channel IS NULL OR source_channel = '')`,
		sessionID)
}
