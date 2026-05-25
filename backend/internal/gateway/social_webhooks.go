// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

// SocialWebhook represents an outgoing webhook for social events.
type SocialWebhook struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AgentID   *string   `json:"agent_id,omitempty"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// SocialWebhookPayload is the JSON body sent to the webhook URL.
type SocialWebhookPayload struct {
	Event     string         `json:"event"`
	Timestamp time.Time      `json:"timestamp"`
	AgentID   string         `json:"agent_id,omitempty"`
	Post      map[string]any `json:"post,omitempty"`
	Results   []socialqor.PostResult `json:"results,omitempty"`
}

// handleListSocialWebhooks handles GET /v1/social/webhooks
func (gw *Gateway) handleListSocialWebhooks(w http.ResponseWriter, r *http.Request) {
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

	query := `SELECT id, tenant_id, agent_id, name, url, secret, events, active, created_at
			  FROM social_webhooks WHERE tenant_id = $1 ORDER BY created_at DESC`
	args := []interface{}{user.TenantID}
	if agentID != "" {
		query = `SELECT id, tenant_id, agent_id, name, url, secret, events, active, created_at
				 FROM social_webhooks WHERE tenant_id = $1 AND (agent_id = $2 OR agent_id IS NULL) ORDER BY created_at DESC`
		args = append(args, agentID)
	}
	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	result := []SocialWebhook{}
	for rows.Next() {
		var wh SocialWebhook
		var events []string
		if scanErr := rows.Scan(&wh.ID, &wh.TenantID, &wh.AgentID, &wh.Name, &wh.URL, &wh.Secret, &events, &wh.Active, &wh.CreatedAt); scanErr == nil {
			wh.Events = events
			if wh.Events == nil {
				wh.Events = []string{}
			}
			wh.Secret = "" // never expose secret in list
			result = append(result, wh)
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCreateSocialWebhook handles POST /v1/social/webhooks
func (gw *Gateway) handleCreateSocialWebhook(w http.ResponseWriter, r *http.Request) {
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
		AgentID *string  `json:"agent_id"`
		Name    string   `json:"name"`
		URL     string   `json:"url"`
		Secret  string   `json:"secret"`
		Events  []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url required"})
		return
	}
	if len(body.Events) == 0 {
		body.Events = []string{"post.published", "post.failed"}
	}

	now := time.Now()
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO social_webhooks (tenant_id, agent_id, name, url, secret, events, active, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,true,$7,$7) RETURNING id`,
		user.TenantID, body.AgentID, body.Name, body.URL, body.Secret, body.Events, now,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	wh := SocialWebhook{
		ID:        id,
		TenantID:  user.TenantID,
		AgentID:   body.AgentID,
		Name:      body.Name,
		URL:       body.URL,
		Events:    body.Events,
		Active:    true,
		CreatedAt: now,
	}
	writeJSON(w, http.StatusCreated, wh)
}

// handleDeleteSocialWebhook handles DELETE /v1/social/webhooks/{id}
func (gw *Gateway) handleDeleteSocialWebhook(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	gw.db.Pool.Exec(r.Context(),
		`DELETE FROM social_webhooks WHERE id = $1 AND tenant_id = $2`,
		chi.URLParam(r, "id"), user.TenantID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleToggleSocialWebhook handles PATCH /v1/social/webhooks/{id}/toggle
func (gw *Gateway) handleToggleSocialWebhook(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	gw.db.Pool.Exec(r.Context(),
		`UPDATE social_webhooks SET active = NOT active, updated_at = NOW() WHERE id = $1 AND tenant_id = $2`,
		chi.URLParam(r, "id"), user.TenantID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "toggled"})
}

// handleTestSocialWebhook handles POST /v1/social/webhooks/{id}/test — sends a test ping
func (gw *Gateway) handleTestSocialWebhook(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	webhookID := chi.URLParam(r, "id")
	var url, secret string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT url, secret FROM social_webhooks WHERE id = $1 AND tenant_id = $2`,
		webhookID, user.TenantID).Scan(&url, &secret)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "webhook not found"})
		return
	}

	payload := SocialWebhookPayload{
		Event:     "test.ping",
		Timestamp: time.Now(),
	}
	if err := dispatchWebhook(context.Background(), url, secret, payload); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "delivery failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
}

// fireSocialWebhooks dispatches outgoing webhooks for a publish event (async, best-effort).
func (gw *Gateway) fireSocialWebhooks(ctx context.Context, agentID, tenantID, eventName string, payload SocialWebhookPayload) {
	if gw.db == nil {
		return
	}
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT url, secret FROM social_webhooks
		 WHERE tenant_id = $1 AND active = true AND $2 = ANY(events)
		   AND (agent_id = $3 OR agent_id IS NULL)`,
		tenantID, eventName, agentID)
	if err != nil {
		return
	}
	defer rows.Close()

	type hook struct{ url, secret string }
	var hooks []hook
	for rows.Next() {
		var h hook
		rows.Scan(&h.url, &h.secret)
		hooks = append(hooks, h)
	}

	for _, h := range hooks {
		go func(url, secret string) {
			if err := dispatchWebhook(context.Background(), url, secret, payload); err != nil {
				slog.Warn("social webhook delivery failed", "url", url, "err", err)
			}
		}(h.url, h.secret)
	}
}

// dispatchWebhook sends a JSON POST to the webhook URL, signing with HMAC-SHA256 if secret is set.
func dispatchWebhook(ctx context.Context, url, secret string, payload SocialWebhookPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Qorven-Social-Webhook/1.0")
	req.Header.Set("X-Qorven-Event", payload.Event)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Qorven-Signature", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("endpoint returned %d", resp.StatusCode)
	}
	return nil
}
