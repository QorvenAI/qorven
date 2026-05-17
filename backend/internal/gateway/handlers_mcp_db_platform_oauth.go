// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/mcp"
	"github.com/qorvenai/qorven/internal/vault"
)

func (gw *Gateway) handleListMCPDB(w http.ResponseWriter, r *http.Request) {
	if gw.mcpManager == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	servers, _ := gw.mcpManager.List(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]any{"servers": servers})
}

func (gw *Gateway) handleAddMCPDB(w http.ResponseWriter, r *http.Request) {
	if gw.mcpManager == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	var s mcp.DBServer
	if json.NewDecoder(r.Body).Decode(&s) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	result, err := gw.mcpManager.Add(r.Context(), defaultTenant, s)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}

func (gw *Gateway) handleInstallMCPDB(w http.ResponseWriter, r *http.Request) {
	if gw.mcpManager == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	tools, err := gw.mcpManager.Install(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"tools": tools})
}

func (gw *Gateway) handleDeleteMCPDB(w http.ResponseWriter, r *http.Request) {
	if gw.mcpManager == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	gw.mcpManager.Delete(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleListPlatforms(w http.ResponseWriter, r *http.Request) {
	if gw.connKB == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	platforms, _ := gw.connKB.ListPlatforms(r.Context())
	// Mark connected ones
	if gw.vault != nil {
		for i := range platforms {
			platforms[i].Connected = gw.vault.IsConnected(r.Context(), defaultTenant, platforms[i].ID)
		}
	}
	writeJSON(w, 200, map[string]any{"platforms": platforms})
}

func (gw *Gateway) handleListPlatformActions(w http.ResponseWriter, r *http.Request) {
	if gw.connKB == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	actions, _ := gw.connKB.ListActions(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]any{"actions": actions})
}

func (gw *Gateway) handleListConnections(w http.ResponseWriter, r *http.Request) {
	if gw.vault == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	creds, _ := gw.vault.List(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]any{"connections": creds})
}

func (gw *Gateway) handleSaveConnection(w http.ResponseWriter, r *http.Request) {
	if gw.vault == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	platformID := chi.URLParam(r, "platform_id")
	var body struct {
		APIKey string `json:"api_key"`
		Token  string `json:"token"`
		Label  string `json:"label"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	platform, err := gw.connKB.GetPlatform(r.Context(), platformID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "platform not found"})
		return
	}

	data := vault.CredentialData{}
	switch platform.AuthType {
	case "api_key":
		data.APIKey = body.APIKey
	case "bearer":
		data.AccessToken = body.Token
	case "basic":
		data.AccessToken = body.Token
	default:
		writeJSON(w, 400, map[string]string{"error": "use OAuth flow for " + platform.AuthType})
		return
	}

	cred, err := gw.vault.Save(r.Context(), defaultTenant, platformID, body.Label, platform.AuthType, data, nil, nil)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, cred)
}

func (gw *Gateway) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	if gw.vault == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	gw.vault.Delete(r.Context(), defaultTenant, chi.URLParam(r, "platform_id"))
	writeJSON(w, 200, map[string]string{"status": "disconnected"})
}

func (gw *Gateway) handleExecuteAction(w http.ResponseWriter, r *http.Request) {
	if gw.connExec == nil {
		writeJSON(w, 503, map[string]string{"error": "db not configured"})
		return
	}
	var body struct {
		Platform string         `json:"platform"`
		Action   string         `json:"action"`
		Params   map[string]any `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	result, err := gw.connExec.Execute(r.Context(), body.Platform, body.Action, body.Params)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"result": json.RawMessage(result)})
}

func (gw *Gateway) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if gw.oauthMgr == nil {
		writeJSON(w, 503, map[string]string{"error": "oauth not configured"})
		return
	}
	provider := chi.URLParam(r, "provider")
	state := r.URL.Query().Get("state")
	if state == "" {
		state = defaultTenant
	}
	url, err := gw.oauthMgr.AuthorizeURL(provider, state)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (gw *Gateway) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if gw.oauthMgr == nil {
		writeJSON(w, 503, map[string]string{"error": "oauth not configured"})
		return
	}
	provider := chi.URLParam(r, "provider")
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, 400, map[string]string{"error": "missing code"})
		return
	}
	state := r.URL.Query().Get("state")
	tenantID := state
	if tenantID == "" {
		tenantID = defaultTenant
	}

	_, err := gw.oauthMgr.HandleCallback(r.Context(), provider, tenantID, code)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	// Redirect back to connections page
	http.Redirect(w, r, "/connections?connected="+provider, http.StatusTemporaryRedirect)
}
