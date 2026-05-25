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

// SocialSet is a reusable content template for social posts.
type SocialSet struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AgentID     *string   `json:"agent_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Platforms   []string  `json:"platforms"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// handleListSocialSets handles GET /v1/social/sets
func (gw *Gateway) handleListSocialSets(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	agentID := r.URL.Query().Get("agent_id")

	query := `SELECT id, tenant_id, agent_id, name, description, content, platforms, created_at, updated_at
			  FROM social_sets WHERE tenant_id = $1 ORDER BY created_at DESC`
	args := []interface{}{user.TenantID}
	if agentID != "" {
		query = `SELECT id, tenant_id, agent_id, name, description, content, platforms, created_at, updated_at
				 FROM social_sets WHERE tenant_id = $1 AND agent_id = $2 ORDER BY created_at DESC`
		args = append(args, agentID)
	}
	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	result := []SocialSet{}
	for rows.Next() {
		var s SocialSet
		var platforms []string
		if scanErr := rows.Scan(&s.ID, &s.TenantID, &s.AgentID, &s.Name, &s.Description, &s.Content, &platforms, &s.CreatedAt, &s.UpdatedAt); scanErr == nil {
			s.Platforms = platforms
			if s.Platforms == nil {
				s.Platforms = []string{}
			}
			result = append(result, s)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCreateSocialSet handles POST /v1/social/sets
func (gw *Gateway) handleCreateSocialSet(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body struct {
		AgentID     *string  `json:"agent_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Content     string   `json:"content"`
		Platforms   []string `json:"platforms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	if body.Platforms == nil {
		body.Platforms = []string{}
	}

	now := time.Now()
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO social_sets (tenant_id, agent_id, name, description, content, platforms, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$7) RETURNING id`,
		user.TenantID, body.AgentID, body.Name, body.Description, body.Content, body.Platforms, now,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	s := SocialSet{
		ID:          id,
		TenantID:    user.TenantID,
		AgentID:     body.AgentID,
		Name:        body.Name,
		Description: body.Description,
		Content:     body.Content,
		Platforms:   body.Platforms,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	writeJSON(w, http.StatusCreated, s)
}

// handleUpdateSocialSet handles PATCH /v1/social/sets/{id}
func (gw *Gateway) handleUpdateSocialSet(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	setID := chi.URLParam(r, "id")

	var body struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Content     *string  `json:"content"`
		Platforms   []string `json:"platforms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}

	gw.db.Pool.Exec(r.Context(),
		`UPDATE social_sets SET
		  name        = COALESCE($1, name),
		  description = COALESCE($2, description),
		  content     = COALESCE($3, content),
		  platforms   = COALESCE($4, platforms),
		  updated_at  = NOW()
		 WHERE id = $5 AND tenant_id = $6`,
		body.Name, body.Description, body.Content, body.Platforms, setID, user.TenantID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteSocialSet handles DELETE /v1/social/sets/{id}
func (gw *Gateway) handleDeleteSocialSet(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	setID := chi.URLParam(r, "id")
	gw.db.Pool.Exec(r.Context(),
		`DELETE FROM social_sets WHERE id = $1 AND tenant_id = $2`, setID, user.TenantID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
