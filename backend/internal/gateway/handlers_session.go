// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/session"
)

func (gw *Gateway) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		writeJSON(w, 200, map[string]any{"sessions": []any{}})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	sessions, err := gw.sessions.List(r.Context(), defaultTenant, agentID, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if sessions == nil {
		sessions = []*session.Session{}
	}
	writeJSON(w, 200, map[string]any{"sessions": sessions})
}

func (gw *Gateway) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var in struct {
		AgentID string `json:"agent_id"`
		Channel string `json:"channel"`
		Label   string `json:"label"`
	}
	json.NewDecoder(r.Body).Decode(&in)
	if in.AgentID == "" {
		in.AgentID = "default"
	}
	if in.Channel == "" {
		in.Channel = "web"
	}

	// One-Qor-one-chat: for the chat-family channels (web, tui, telegram,
	// whatsapp, slack DM, discord DM) we return the Qor's existing
	// canonical session if one is active. POST /v1/sessions is the old
	// API shape; keeping it idempotent lets every caller stop worrying
	// about "did I create a duplicate thread?" without a protocol
	// rename. Email and anything explicitly non-chat fall through to the
	// old create path and keep their own sessions.
	if isChatFamilyChannel(in.Channel) {
		if existing, err := gw.sessions.FindByAgentAndChannel(r.Context(), in.AgentID, in.Channel); err == nil && existing != nil {
			writeJSON(w, 200, existing)
			return
		}
	}

	// record the authenticated actor as the canonical owner.
	// userID is kept as "operator" for backward compatibility with older
	// reads; OwnerActorID is the one the OwnerCheck consults.
	ownerActorID := actorFromContext(r.Context())
	s, err := gw.sessions.CreateWithOwner(r.Context(), defaultTenant, in.AgentID, "operator", ownerActorID, in.Channel)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, s)
}

func isChatFamilyChannel(c string) bool {
	switch c {
	case "web", "tui", "telegram", "whatsapp", "slack_dm", "discord_dm":
		return true
	}
	return false
}

func (gw *Gateway) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	s, err := gw.sessions.GetByID(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, 200, s)
}

func (gw *Gateway) handleGetSessionFiles(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	files, err := gw.sessions.GetFiles(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(files)
}

func (gw *Gateway) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	gw.sessions.Delete(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleGetDreaming(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	agentID := chi.URLParam(r, "id")
	var cfg struct {
		Enabled       bool       `json:"enabled"`
		IntervalHours int        `json:"interval_hours"`
		Mode          string     `json:"mode"`
		LastDreamAt   *time.Time `json:"last_dream_at,omitempty"`
		NextDreamAt   *time.Time `json:"next_dream_at,omitempty"`
	}
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(dreaming_enabled,true), COALESCE(dreaming_interval_hours,6),
		 COALESCE(dreaming_mode,'balanced'), last_dream_at, next_dream_at
		 FROM agents WHERE id=$1`, agentID,
	).Scan(&cfg.Enabled, &cfg.IntervalHours, &cfg.Mode, &cfg.LastDreamAt, &cfg.NextDreamAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, 200, cfg)
}

func (gw *Gateway) handleUpdateDreaming(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	agentID := chi.URLParam(r, "id")
	var body struct {
		Enabled       bool   `json:"enabled"`
		IntervalHours int    `json:"interval_hours"`
		Mode          string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if body.IntervalHours < 1 {
		body.IntervalHours = 6
	}
	if body.Mode == "" {
		body.Mode = "balanced"
	}
	gw.db.Pool.Exec(r.Context(),
		`UPDATE agents SET dreaming_enabled=$1, dreaming_interval_hours=$2, dreaming_mode=$3 WHERE id=$4`,
		body.Enabled, body.IntervalHours, body.Mode, agentID)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (gw *Gateway) handleTriggerDream(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	agentID := chi.URLParam(r, "id")
	// Stamp last_dream_at immediately
	gw.db.Pool.Exec(r.Context(),
		`UPDATE agents SET last_dream_at=now() WHERE id=$1`, agentID)

	// Run dreamer in background — dreamer.RunOnce consolidates memories across the tenant
	if gw.dreamer != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			stats := gw.dreamer.RunOnce(ctx)
			slog.Info("dream.complete", "agent_id", agentID,
				"merged", stats.MemoriesMerged, "decayed", stats.MemoriesDecayed,
				"digests", stats.DigestsCreated, "duration", stats.LastRunDuration)
			// Update next_dream_at
			gw.db.Pool.Exec(ctx,
				`UPDATE agents SET last_dream_at=now(), next_dream_at=now() + interval '6 hours' WHERE id=$1`,
				agentID)
		}()
		writeJSON(w, 202, map[string]string{"status": "dream_triggered", "agent_id": agentID})
		return
	}
	writeJSON(w, 503, map[string]string{"error": "dreamer not available"})
}
