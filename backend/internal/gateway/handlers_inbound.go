// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/inbound"
)

// --- Inbound config ---

func (gw *Gateway) handleGetInboundConfig(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")

	cfg := inbound.AgentConfig{
		AgentID:           agentID,
		DefaultMode:       inbound.ModeDraftAndApprove,
		UnknownSenderMode: inbound.ModeContextOnly,
		SpamAction:        inbound.ModeDrop,
		BriefingTime:      "08:00",
		BriefingTimezone:  "Asia/Shanghai",
	}
	var defMode, unknownMode, spamAct string
	_ = gw.db.Pool.QueryRow(r.Context(),
		`SELECT tenant_id, default_mode, unknown_sender_mode, spam_action,
		        notification_channel, notification_target,
		        briefing_enabled, briefing_time, briefing_timezone
		 FROM inbound_agent_config WHERE agent_id = $1`, agentID,
	).Scan(&cfg.TenantID, &defMode, &unknownMode, &spamAct,
		&cfg.NotificationChannel, &cfg.NotificationTarget,
		&cfg.BriefingEnabled, &cfg.BriefingTime, &cfg.BriefingTimezone)
	if defMode != "" {
		cfg.DefaultMode = inbound.ActionMode(defMode)
	}
	if unknownMode != "" {
		cfg.UnknownSenderMode = inbound.ActionMode(unknownMode)
	}
	if spamAct != "" {
		cfg.SpamAction = inbound.ActionMode(spamAct)
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (gw *Gateway) handlePutInboundConfig(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	var req inbound.AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	_, err := gw.db.Pool.Exec(r.Context(),
		`INSERT INTO inbound_agent_config
		    (agent_id, tenant_id, default_mode, unknown_sender_mode, spam_action,
		     notification_channel, notification_target,
		     briefing_enabled, briefing_time, briefing_timezone, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (agent_id) DO UPDATE SET
		    default_mode=$3, unknown_sender_mode=$4, spam_action=$5,
		    notification_channel=$6, notification_target=$7,
		    briefing_enabled=$8, briefing_time=$9, briefing_timezone=$10, updated_at=$11`,
		agentID, defaultTenant,
		string(req.DefaultMode), string(req.UnknownSenderMode), string(req.SpamAction),
		req.NotificationChannel, req.NotificationTarget,
		req.BriefingEnabled, req.BriefingTime, req.BriefingTimezone, time.Now(),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Re-register briefing cron job with updated schedule.
	if gw.briefingSched != nil {
		if req.BriefingEnabled {
			gw.briefingSched.RegisterAgent(r.Context(), agentID, defaultTenant, req.BriefingTime, req.BriefingTimezone)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// --- Inbound rules ---

func (gw *Gateway) handleListInboundRules(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, priority, match_type, match_value, mode, created_at
		 FROM inbound_rules WHERE agent_id = $1 ORDER BY priority ASC`, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	type RuleRow struct {
		ID         string    `json:"id"`
		Priority   int       `json:"priority"`
		MatchType  string    `json:"match_type"`
		MatchValue string    `json:"match_value"`
		Mode       string    `json:"mode"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var result []RuleRow
	for rows.Next() {
		var row RuleRow
		if err := rows.Scan(&row.ID, &row.Priority, &row.MatchType, &row.MatchValue, &row.Mode, &row.CreatedAt); err != nil {
			continue
		}
		result = append(result, row)
	}
	if result == nil {
		result = []RuleRow{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (gw *Gateway) handleCreateInboundRule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	var req struct {
		Priority   int    `json:"priority"`
		MatchType  string `json:"match_type"`
		MatchValue string `json:"match_value"`
		Mode       string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO inbound_rules (tenant_id, agent_id, priority, match_type, match_value, mode)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		defaultTenant, agentID, req.Priority, req.MatchType, req.MatchValue, req.Mode,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (gw *Gateway) handleUpdateInboundRule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	ruleID := chi.URLParam(r, "ruleId")
	var req struct {
		Priority   int    `json:"priority"`
		MatchType  string `json:"match_type"`
		MatchValue string `json:"match_value"`
		Mode       string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	_, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE inbound_rules SET priority=$1, match_type=$2, match_value=$3, mode=$4, updated_at=now()
		 WHERE id=$5 AND agent_id=$6 AND tenant_id=$7`,
		req.Priority, req.MatchType, req.MatchValue, req.Mode, ruleID, agentID, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (gw *Gateway) handleDeleteInboundRule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	ruleID := chi.URLParam(r, "ruleId")
	_, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM inbound_rules WHERE id = $1 AND agent_id = $2 AND tenant_id = $3`,
		ruleID, agentID, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// --- Drafts ---

func (gw *Gateway) handleListDrafts(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.inbound == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	tenantUUID := uuid.MustParse(defaultTenant)
	drafts, err := gw.inbound.DraftQueue().List(r.Context(), tenantUUID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if drafts == nil {
		drafts = []inbound.DraftReply{}
	}
	writeJSON(w, http.StatusOK, drafts)
}

func (gw *Gateway) handleGetDraft(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	draftID := chi.URLParam(r, "id")
	var d inbound.DraftReply
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, agent_id, sender_id, sender_name, channel,
		        original_message, history_summary, draft_content, status, created_at
		 FROM draft_replies WHERE id = $1 AND tenant_id = $2`, draftID, defaultTenant,
	).Scan(&d.ID, &d.AgentID, &d.SenderID, &d.SenderName, &d.Channel,
		&d.OriginalMessage, &d.HistorySummary, &d.DraftContent, &d.Status, &d.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (gw *Gateway) handleSendDraft(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.inbound == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "inbound not available"})
		return
	}
	draftID := chi.URLParam(r, "id")
	if err := gw.inbound.DraftQueue().Transition(r.Context(), uuid.MustParse(defaultTenant), draftID, "sent", "user"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (gw *Gateway) handleDiscardDraft(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.inbound == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "inbound not available"})
		return
	}
	draftID := chi.URLParam(r, "id")
	if err := gw.inbound.DraftQueue().Transition(r.Context(), uuid.MustParse(defaultTenant), draftID, "discarded", "user"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (gw *Gateway) handleEditDraft(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if gw.inbound == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "inbound not available"})
		return
	}
	draftID := chi.URLParam(r, "id")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := gw.inbound.DraftQueue().UpdateContent(r.Context(), uuid.MustParse(defaultTenant), draftID, req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
