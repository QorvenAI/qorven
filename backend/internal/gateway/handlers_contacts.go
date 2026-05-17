// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type contactRow struct {
	ID            string    `json:"id"`
	ExternalID    string    `json:"external_id"`
	Channel       string    `json:"channel"`
	DisplayName   string    `json:"display_name"`
	Company       string    `json:"company"`
	Notes         string    `json:"notes"`
	PipelineStage string    `json:"pipeline_stage"`
	Tags          []string  `json:"tags"`
	Email         string    `json:"email,omitempty"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	MessageCount  int64     `json:"message_count"`
}

func (gw *Gateway) handleListContacts(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	q := r.URL.Query()
	stage := q.Get("stage")   // optional filter
	search := q.Get("search") // optional name/id search

	query := `SELECT id, external_id, channel, COALESCE(display_name,''), COALESCE(company,''), COALESCE(notes,''),
	                 pipeline_stage, COALESCE(tags,'{}'), COALESCE(email,''), first_seen, last_seen, message_count
	          FROM contacts WHERE tenant_id = $1`
	args := []any{defaultTenant}

	if stage != "" {
		args = append(args, stage)
		query += ` AND pipeline_stage = $` + itoa(len(args))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		query += ` AND (display_name ILIKE $` + itoa(len(args)) + ` OR external_id ILIKE $` + itoa(len(args)) + `)`
	}
	query += ` ORDER BY last_seen DESC LIMIT 200`

	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	result := []contactRow{}
	for rows.Next() {
		var c contactRow
		if err := rows.Scan(&c.ID, &c.ExternalID, &c.Channel, &c.DisplayName, &c.Company, &c.Notes,
			&c.PipelineStage, &c.Tags, &c.Email, &c.FirstSeen, &c.LastSeen, &c.MessageCount); err != nil {
			continue
		}
		result = append(result, c)
	}
	writeJSON(w, http.StatusOK, result)
}

func (gw *Gateway) handleGetContact(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var c contactRow
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, external_id, channel, COALESCE(display_name,''), COALESCE(company,''), COALESCE(notes,''),
		        pipeline_stage, COALESCE(tags,'{}'), COALESCE(email,''), first_seen, last_seen, message_count
		 FROM contacts WHERE id = $1 AND tenant_id = $2`, id, defaultTenant).
		Scan(&c.ID, &c.ExternalID, &c.Channel, &c.DisplayName, &c.Company, &c.Notes,
			&c.PipelineStage, &c.Tags, &c.Email, &c.FirstSeen, &c.LastSeen, &c.MessageCount)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "contact not found"})
		return
	}

	// Attach recent sessions for this sender.
	type sessionSnippet struct {
		ID        string    `json:"id"`
		AgentID   string    `json:"agent_id"`
		Summary   string    `json:"summary"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	var sessions []sessionSnippet
	sRows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, agent_id, COALESCE(summary,''), updated_at
		 FROM sessions WHERE tenant_id = $1 AND user_id = $2
		 ORDER BY updated_at DESC LIMIT 20`,
		defaultTenant, c.ExternalID)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var s sessionSnippet
			if sRows.Scan(&s.ID, &s.AgentID, &s.Summary, &s.UpdatedAt) == nil {
				sessions = append(sessions, s)
			}
		}
	}
	if sessions == nil {
		sessions = []sessionSnippet{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"contact":  c,
		"sessions": sessions,
	})
}

func (gw *Gateway) handlePatchContact(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		DisplayName   *string  `json:"display_name"`
		Company       *string  `json:"company"`
		Notes         *string  `json:"notes"`
		PipelineStage *string  `json:"pipeline_stage"`
		Tags          []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	if req.DisplayName != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE contacts SET display_name = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.DisplayName, id, defaultTenant)
	}
	if req.Company != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE contacts SET company = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.Company, id, defaultTenant)
	}
	if req.Notes != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE contacts SET notes = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.Notes, id, defaultTenant)
	}
	if req.PipelineStage != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE contacts SET pipeline_stage = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			*req.PipelineStage, id, defaultTenant)
	}
	if req.Tags != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE contacts SET tags = $1, updated_at = now() WHERE id = $2 AND tenant_id = $3`,
			req.Tags, id, defaultTenant)
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (gw *Gateway) handleCreateContact(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	var req struct {
		ExternalID  string `json:"external_id"`
		Channel     string `json:"channel"`
		DisplayName string `json:"display_name"`
		Company     string `json:"company"`
		Notes       string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExternalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "external_id required"})
		return
	}
	if req.Channel == "" {
		req.Channel = "email"
	}
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO contacts (tenant_id, external_id, channel, display_name, company, notes)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, external_id, channel) DO UPDATE
		   SET display_name = EXCLUDED.display_name,
		       company = EXCLUDED.company,
		       updated_at = now()
		 RETURNING id`,
		defaultTenant, req.ExternalID, req.Channel, req.DisplayName, req.Company, req.Notes,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

type contactAgentPrefs struct {
	ContactID   string    `json:"contact_id"`
	AgentID     string    `json:"agent_id"`
	RoutingMode string    `json:"routing_mode"`
	TrustLevel  string    `json:"trust_level"`
	AgentNotes  string    `json:"agent_notes"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (gw *Gateway) handleGetContactPrefs(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	contactID := chi.URLParam(r, "id")
	agentID := chi.URLParam(r, "agentId")
	var p contactAgentPrefs
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT contact_id, agent_id, routing_mode, trust_level, COALESCE(agent_notes,''), updated_at
		 FROM contact_agent_prefs WHERE contact_id = $1 AND agent_id = $2`,
		contactID, agentID,
	).Scan(&p.ContactID, &p.AgentID, &p.RoutingMode, &p.TrustLevel, &p.AgentNotes, &p.UpdatedAt)
	if err != nil {
		// Return defaults if no prefs row yet
		writeJSON(w, http.StatusOK, contactAgentPrefs{
			ContactID: contactID, AgentID: agentID,
			RoutingMode: "inherit", TrustLevel: "unknown",
		})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (gw *Gateway) handlePutContactPrefs(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	contactID := chi.URLParam(r, "id")
	agentID := chi.URLParam(r, "agentId")
	var req struct {
		RoutingMode string `json:"routing_mode"`
		TrustLevel  string `json:"trust_level"`
		AgentNotes  string `json:"agent_notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	_, err := gw.db.Pool.Exec(r.Context(),
		`INSERT INTO contact_agent_prefs (contact_id, agent_id, routing_mode, trust_level, agent_notes)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (contact_id, agent_id) DO UPDATE
		   SET routing_mode = EXCLUDED.routing_mode,
		       trust_level = EXCLUDED.trust_level,
		       agent_notes = EXCLUDED.agent_notes,
		       updated_at = now()`,
		contactID, agentID, req.RoutingMode, req.TrustLevel, req.AgentNotes,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (gw *Gateway) handleConfirmInboundRule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	ruleID := chi.URLParam(r, "ruleId")
	tag, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE inbound_rules SET status = 'active', updated_at = now()
		 WHERE id = $1 AND agent_id = $2 AND status = 'pending_confirmation'`,
		ruleID, agentID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "rule not found or already confirmed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (gw *Gateway) handleDiscardInboundRule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	ruleID := chi.URLParam(r, "ruleId")
	gw.db.Pool.Exec(r.Context(),
		`DELETE FROM inbound_rules WHERE id = $1 AND agent_id = $2 AND status = 'pending_confirmation'`,
		ruleID, agentID,
	)
	writeJSON(w, http.StatusOK, map[string]string{"status": "discarded"})
}

// itoa is a minimal int-to-string helper used for dynamic SQL $N params.
func itoa(n int) string {
	digits := []byte("0123456789")
	if n < 10 {
		return string(digits[n])
	}
	return string(digits[n/10]) + string(digits[n%10])
}
