package gateway

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/connectors"
)

func (gw *Gateway) handleIntegrationsStatus(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	has, _ := gw.relayStore.HasRelay(r.Context(), defaultTenant)
	accounts, _ := gw.relayStore.ListAccounts(r.Context(), defaultTenant)
	count := 0
	if accounts != nil {
		count = len(accounts)
	}
	provider := ""
	if has {
		provider = "pipedream"
	}
	writeJSON(w, 200, map[string]any{
		"configured":     has,
		"provider":       provider,
		"accounts_count": count,
	})
}

func (gw *Gateway) handleSaveRelayKey(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if req.Provider == "" {
		req.Provider = "pipedream"
	}
	if req.APIKey == "" {
		writeJSON(w, 400, map[string]string{"error": "api_key required"})
		return
	}

	client := connectors.NewPipedreamClient(req.APIKey, "")
	if err := client.TestConnection(r.Context()); err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid API key", "detail": err.Error()})
		return
	}

	if err := gw.relayStore.SaveRelayKey(r.Context(), defaultTenant, req.Provider, req.APIKey); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save key"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "saved"})
}

func (gw *Gateway) handleDeleteRelayKey(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	if err := gw.relayStore.DeleteRelayKey(r.Context(), defaultTenant, "pipedream"); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to delete key"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleListConnectedAccounts(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	accounts, err := gw.relayStore.ListAccounts(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to list accounts"})
		return
	}
	if accounts == nil {
		accounts = []connectors.ConnectedAccountRecord{}
	}
	writeJSON(w, 200, accounts)
}

func (gw *Gateway) handleConnectPlatform(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	platformID := chi.URLParam(r, "platform")
	if platformID == "" {
		writeJSON(w, 400, map[string]string{"error": "platform required"})
		return
	}

	apiKey, err := gw.relayStore.GetRelayKey(r.Context(), defaultTenant, "pipedream")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Pipedream not configured. Add your API key first."})
		return
	}

	client := connectors.NewPipedreamClient(apiKey, "")

	baseURL := gw.cfg.Server.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:4200"
	}
	webhookURI := baseURL + "/v1/integrations/webhook"

	token, err := client.CreateConnectToken(r.Context(), defaultTenant, platformID, "", "", webhookURI)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "failed to create connect token", "detail": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"connect_link_url": token.ConnectLinkURL,
		"expires_at":       token.ExpiresAt,
	})
}

func (gw *Gateway) handleDisconnectAccount(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	accountID := chi.URLParam(r, "id")
	if accountID == "" {
		writeJSON(w, 400, map[string]string{"error": "account id required"})
		return
	}
	if err := gw.relayStore.DeleteAccount(r.Context(), defaultTenant, accountID); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to disconnect"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "disconnected"})
}

func (gw *Gateway) handlePipedreamWebhook(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		w.WriteHeader(503)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(400)
		return
	}

	_ = r.Header.Get("X-Pipedream-Signature")

	var payload connectors.PipedreamWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		w.WriteHeader(400)
		return
	}

	tenantID := payload.ExternalUserID
	if tenantID == "" {
		tenantID = defaultTenant
	}

	if err := connectors.HandlePipedreamWebhook(r.Context(), gw.relayStore, tenantID, payload); err != nil {
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(200)
}

func (gw *Gateway) handleIntegrationLog(w http.ResponseWriter, r *http.Request) {
	if gw.relayStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	entries, err := gw.relayStore.ListLog(r.Context(), defaultTenant, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to fetch log"})
		return
	}
	if entries == nil {
		entries = []connectors.ActionLogEntry{}
	}
	writeJSON(w, 200, entries)
}

func (gw *Gateway) handleListIntegrationPermissions(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, agent_id, COALESCE(platform_id, ''), COALESCE(action_key, ''), allowed
		 FROM integration_permissions WHERE tenant_id = $1 ORDER BY created_at DESC`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type Perm struct {
		ID         string `json:"id"`
		AgentID    string `json:"agent_id"`
		PlatformID string `json:"platform_id"`
		ActionKey  string `json:"action_key"`
		Allowed    bool   `json:"allowed"`
	}
	var perms []Perm
	for rows.Next() {
		var p Perm
		rows.Scan(&p.ID, &p.AgentID, &p.PlatformID, &p.ActionKey, &p.Allowed)
		perms = append(perms, p)
	}
	if perms == nil {
		perms = []Perm{}
	}
	writeJSON(w, 200, perms)
}

func (gw *Gateway) handleSetIntegrationPermission(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	var req struct {
		AgentID    string `json:"agent_id"`
		PlatformID string `json:"platform_id"`
		ActionKey  string `json:"action_key"`
		Allowed    bool   `json:"allowed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if req.AgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}

	_, err := gw.db.Pool.Exec(r.Context(),
		`INSERT INTO integration_permissions (tenant_id, agent_id, platform_id, action_key, allowed)
		 VALUES ($1, $2::uuid, NULLIF($3, ''), NULLIF($4, ''), $5)
		 ON CONFLICT (tenant_id, agent_id, COALESCE(platform_id, '__all__'), COALESCE(action_key, '__all__'))
		 DO UPDATE SET allowed = $5`,
		defaultTenant, req.AgentID, req.PlatformID, req.ActionKey, req.Allowed)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save permission"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "saved"})
}
