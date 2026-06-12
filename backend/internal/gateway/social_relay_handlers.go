// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

// validRelayProviders is the set of supported relay providers for key management.
var validRelayProviders = map[string]bool{
	"outstand":   true,
	"postforme":  true,
	"buffer":     true,
	"pipedream":  true,
	"n8n":        true,
	"triggerdev": true,
}

// newRelayClient creates a RelayClient for the given provider and API key.
func (gw *Gateway) newRelayClient(provider, apiKey string) socialqor.RelayClient {
	switch provider {
	case "outstand":
		return socialqor.NewOutstandClient(apiKey)
	case "postforme":
		return socialqor.NewPostForMeClient(apiKey)
	case "buffer":
		return socialqor.NewBufferClient(apiKey)
	default:
		return nil
	}
}

// --- Relay Key Management ---

// handleListRelayKeys returns all relay provider keys for the tenant.
// GET /v1/social/relay-providers
func (gw *Gateway) handleListRelayKeys(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	keys, err := gw.socialRelayStore.ListKeys(r.Context(), user.TenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if keys == nil {
		keys = []socialqor.RelayProviderRecord{}
	}
	writeJSON(w, http.StatusOK, keys)
}

// handleAddRelayKey adds a new relay provider key.
// POST /v1/social/relay-providers
func (gw *Gateway) handleAddRelayKey(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body struct {
		Provider string `json:"provider"`
		Label    string `json:"label"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !validRelayProviders[body.Provider] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider; must be one of: outstand, postforme, buffer, pipedream, n8n, triggerdev"})
		return
	}
	if body.APIKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "api_key is required"})
		return
	}

	// Test connection before saving (skip for pipedream — no RelayClient yet)
	client := gw.newRelayClient(body.Provider, body.APIKey)
	if client != nil {
		if err := client.TestConnection(r.Context()); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error":  "connection test failed",
				"detail": err.Error(),
			})
			return
		}
	}

	id, err := gw.socialRelayStore.AddKey(r.Context(), user.TenantID, body.Provider, body.Label, body.APIKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"id":       id,
		"provider": body.Provider,
		"label":    body.Label,
		"status":   "active",
	})
}

// handleUpdateRelayKey updates an existing relay key's label or status.
// PATCH /v1/social/relay-providers/{id}
func (gw *Gateway) handleUpdateRelayKey(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	keyID := chi.URLParam(r, "id")
	var body struct {
		Label  string `json:"label"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Status != "" && body.Status != "active" && body.Status != "disabled" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be 'active' or 'disabled'"})
		return
	}

	if err := gw.socialRelayStore.UpdateKey(r.Context(), keyID, body.Label, body.Status); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleDeleteSocialRelayKey removes a relay provider key.
// DELETE /v1/social/relay-providers/{id}
func (gw *Gateway) handleDeleteSocialRelayKey(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	keyID := chi.URLParam(r, "id")
	if err := gw.socialRelayStore.DeleteKey(r.Context(), keyID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleTestRelayKey tests connectivity for an existing relay key.
// POST /v1/social/relay-providers/{id}/test
func (gw *Gateway) handleTestRelayKey(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	keyID := chi.URLParam(r, "id")
	provider, apiKey, err := gw.socialRelayStore.GetKeyWithProvider(r.Context(), keyID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "relay key not found"})
		return
	}

	client := gw.newRelayClient(provider, apiKey)
	if client == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "no test available for provider"})
		return
	}

	if err := client.TestConnection(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleListRelayKeyAccounts lists accounts available through a relay key.
// GET /v1/social/relay-providers/{id}/accounts
func (gw *Gateway) handleListRelayKeyAccounts(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	keyID := chi.URLParam(r, "id")
	provider, apiKey, err := gw.socialRelayStore.GetKeyWithProvider(r.Context(), keyID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "relay key not found"})
		return
	}

	client := gw.newRelayClient(provider, apiKey)
	if client == nil {
		writeJSON(w, http.StatusOK, []socialqor.RelayAccount{})
		return
	}

	accounts, err := client.ListAccounts(r.Context(), user.TenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if accounts == nil {
		accounts = []socialqor.RelayAccount{}
	}
	writeJSON(w, http.StatusOK, accounts)
}

// --- OAuth Connection via Relay ---

// handleRelayConnect initiates an OAuth connection flow via a relay provider.
// POST /v1/social/connect
func (gw *Gateway) handleRelayConnect(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body struct {
		RelayKeyID string `json:"relay_key_id"`
		Platform   string `json:"platform"`
		AgentID    string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.RelayKeyID == "" || body.Platform == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "relay_key_id and platform are required"})
		return
	}

	provider, apiKey, err := gw.socialRelayStore.GetKeyWithProvider(r.Context(), body.RelayKeyID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "relay key not found"})
		return
	}

	client := gw.newRelayClient(provider, apiKey)
	if client == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support OAuth connection"})
		return
	}

	baseURL := gw.cfg.Server.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost"
	}
	redirectURL := baseURL + "/v1/social/connect/callback?relay_key_id=" + body.RelayKeyID

	authURL, err := client.GetAuthURL(r.Context(), body.Platform, user.TenantID, redirectURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"auth_url":     authURL,
		"relay_key_id": body.RelayKeyID,
	})
}

// handleRelayConnectFinalize completes the OAuth flow and saves the integration.
// POST /v1/social/connect/finalize
func (gw *Gateway) handleRelayConnectFinalize(w http.ResponseWriter, r *http.Request) {
	if gw.socialRelayStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	var body struct {
		RelayKeyID   string `json:"relay_key_id"`
		SessionToken string `json:"session_token"`
		AgentID      string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.RelayKeyID == "" || body.SessionToken == "" || body.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "relay_key_id, session_token, and agent_id are required"})
		return
	}

	provider, apiKey, err := gw.socialRelayStore.GetKeyWithProvider(r.Context(), body.RelayKeyID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "relay key not found"})
		return
	}

	client := gw.newRelayClient(provider, apiKey)
	if client == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider does not support OAuth connection"})
		return
	}

	account, err := client.FinalizeConnection(r.Context(), body.SessionToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Save as a social integration
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	integration := socialqor.Integration{
		Platform:           socialqor.Platform(account.Platform),
		AccountName:        account.AccountName,
		AccountID:          account.ID,
		AgentID:            body.AgentID,
		Active:             true,
		RelayProvider:      provider,
		RelayProviderKeyID: body.RelayKeyID,
		RelayAccountID:     account.ID,
	}

	id, err := store.SaveIntegration(r.Context(), integration)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":               id,
		"platform":         account.Platform,
		"account_name":     account.AccountName,
		"relay_provider":   provider,
		"relay_key_id":     body.RelayKeyID,
		"relay_account_id": account.ID,
	})
}

// --- Account Rules ---

// handleGetAccountRules returns content rules for a social integration.
// GET /v1/social/integrations/{id}/rules
func (gw *Gateway) handleGetAccountRules(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	integrationID := chi.URLParam(r, "id")
	rules, err := store.GetAccountRules(r.Context(), integrationID)
	if err != nil {
		// Return empty rules if none exist yet
		writeJSON(w, http.StatusOK, map[string]any{
			"integration_id":    integrationID,
			"voice_style":       "",
			"content_rules":     "",
			"knowledge_context": "",
			"hashtag_sets":      map[string][]string{},
			"posting_guidelines": "",
		})
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleSetAccountRules creates or updates content rules for a social integration.
// PUT /v1/social/integrations/{id}/rules
func (gw *Gateway) handleSetAccountRules(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	integrationID := chi.URLParam(r, "id")
	var rules socialqor.AccountRules
	if err := json.NewDecoder(r.Body).Decode(&rules); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	rules.IntegrationID = integrationID
	rules.TenantID = user.TenantID

	// AgentID is required for the upsert
	if rules.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}

	if err := store.UpsertAccountRules(r.Context(), &rules); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// --- Platform Matrix ---

// handlePlatformMatrix returns the platform relay support matrix enriched with user's active keys.
// GET /v1/social/platforms
func (gw *Gateway) handlePlatformMatrix(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	// Determine which providers the user has active keys for
	userProviders := map[string]bool{}
	if gw.socialRelayStore != nil {
		keys, err := gw.socialRelayStore.ListKeys(r.Context(), user.TenantID)
		if err == nil {
			for _, k := range keys {
				if k.Status == "active" {
					userProviders[k.Provider] = true
				}
			}
		}
	}

	type platformEntry struct {
		Relays      []string          `json:"relays"`
		Warnings    map[string]string `json:"warnings"`
		UserHasKeys []string          `json:"user_has_keys"`
	}

	result := map[string]platformEntry{}
	for platform, pr := range socialqor.PlatformRelayMatrix {
		var hasKeys []string
		for _, relay := range pr.Relays {
			if userProviders[relay] {
				hasKeys = append(hasKeys, relay)
			}
		}
		if hasKeys == nil {
			hasKeys = []string{}
		}
		warnings := pr.Warnings
		if warnings == nil {
			warnings = map[string]string{}
		}
		result[string(platform)] = platformEntry{
			Relays:      pr.Relays,
			Warnings:    warnings,
			UserHasKeys: hasKeys,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

// handleRelayConnectCallback handles the OAuth redirect from relay providers.
// GET /v1/social/connect/callback?session_token=...&relay_key_id=...
func (gw *Gateway) handleRelayConnectCallback(w http.ResponseWriter, r *http.Request) {
	sessionToken := r.URL.Query().Get("session_token")
	relayKeyID := r.URL.Query().Get("relay_key_id")
	errMsg := r.URL.Query().Get("error")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Connecting...</title></head><body>
<script>
(function(){
  var data = {type:"relay_connect_callback", session_token:%q, relay_key_id:%q, error:%q};
  if(window.opener){window.opener.postMessage(data,"*");window.close();}
  else{document.body.innerText="Connection complete. You can close this window.";}
})();
</script>
<p>Connecting your account...</p>
</body></html>`, sessionToken, relayKeyID, errMsg)
}
