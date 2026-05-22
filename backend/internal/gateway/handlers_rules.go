// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type agentRule struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	Description string          `json:"description"`
	TriggerType string          `json:"trigger_type"`
	TriggerSpec json.RawMessage `json:"trigger_spec"`
	ActionType  string          `json:"action_type"`
	ActionSpec  json.RawMessage `json:"action_spec"`
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
}

// GET /v1/rules — list all rules for the tenant
func (gw *Gateway) handleListRules(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, agent_id, description, trigger_type, trigger_spec, action_type, action_spec, enabled, created_at
		 FROM agent_rules
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC`,
		defaultTenant,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	var rules []agentRule
	for rows.Next() {
		var rule agentRule
		if err := rows.Scan(&rule.ID, &rule.AgentID, &rule.Description,
			&rule.TriggerType, &rule.TriggerSpec,
			&rule.ActionType, &rule.ActionSpec,
			&rule.Enabled, &rule.CreatedAt); err != nil {
			continue
		}
		rules = append(rules, rule)
	}
	if rules == nil {
		rules = []agentRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// PUT /v1/rules/{id}/enabled — enable or disable a rule
func (gw *Gateway) handleToggleRule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	tag, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE agent_rules SET enabled = $1 WHERE id = $2 AND tenant_id = $3`,
		body.Enabled, id, defaultTenant,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "rule not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": body.Enabled})
}
