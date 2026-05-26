// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	gatewayllm "github.com/qorvenai/qorven/internal/gateway/llm"
	"github.com/qorvenai/qorven/internal/vault"
)

// handleOAuthProviderStart redirects the user to the provider's authorization
// URL. The OAuth state + PKCE verifier are stored server-side until callback.
//
// GET /v1/providers/oauth/{provider}/start
func (gw *Gateway) handleOAuthProviderStart(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.llmOAuthMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	if _, ok := gatewayllm.OAuthProviders[provider]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown OAuth provider"})
		return
	}

	mgr := gw.oauthProviderMgr()
	authURL, stateParam, err := mgr.StartURL(defaultTenant, provider)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Return URL for popup-based flows; callers can also follow the redirect.
	accept := r.Header.Get("Accept")
	if len(accept) > 0 && accept != "*/*" && !contains(accept, "text/html") {
		writeJSON(w, http.StatusOK, map[string]string{"url": authURL, "state": stateParam})
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOAuthProviderCallback receives the authorization code from the
// provider, exchanges it for tokens, and stores them encrypted.
//
// GET /v1/providers/oauth/{provider}/callback
func (gw *Gateway) handleOAuthProviderCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errParam := r.URL.Query().Get("error")

	if errParam != "" {
		writeHTML(w, http.StatusBadRequest,
			`<html><body><p>OAuth error: `+html.EscapeString(errParam)+`</p><script>window.close();</script></body></html>`)
		return
	}
	if code == "" || state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing code or state"})
		return
	}

	mgr := gw.oauthProviderMgr()
	_, err := mgr.HandleCallback(r.Context(), provider, code, state)
	if err != nil {
		writeHTML(w, http.StatusBadRequest,
			`<html><body><p>OAuth failed: `+html.EscapeString(err.Error())+`</p><script>window.close();</script></body></html>`)
		return
	}

	// Close the popup and signal success to the opener window.
	// provider is a path param from a fixed set of known IDs — escape anyway.
	safeProvider := html.EscapeString(provider)
	writeHTML(w, http.StatusOK, `<html><body>
<p>Connected! You can close this window.</p>
<script>
  if (window.opener) {
    window.opener.postMessage({ type: 'oauth_complete', provider: '`+safeProvider+`' }, '*');
  }
  setTimeout(() => window.close(), 1000);
</script>
</body></html>`)
}

// handleOAuthProviderStatus returns connection status for a provider.
//
// GET /v1/providers/oauth/{provider}/status
func (gw *Gateway) handleOAuthProviderStatus(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	provider := chi.URLParam(r, "provider")
	mgr := gw.oauthProviderMgr()
	writeJSON(w, http.StatusOK, mgr.Status(r.Context(), defaultTenant, provider))
}

// handleOAuthProviderRevoke deletes stored tokens for a provider.
//
// POST /v1/providers/oauth/{provider}/revoke
func (gw *Gateway) handleOAuthProviderRevoke(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	mgr := gw.oauthProviderMgr()
	if err := mgr.Revoke(r.Context(), defaultTenant, provider); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// handleOAuthProvidersList returns the registry of supported OAuth providers.
//
// GET /v1/providers/oauth
func (gw *Gateway) handleOAuthProvidersList(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		PKCE bool   `json:"pkce"`
		Icon string `json:"icon"`
	}
	var out []entry
	for id, spec := range gatewayllm.OAuthProviders {
		out = append(out, entry{ID: id, Name: spec.Name, PKCE: spec.PKCE, Icon: spec.Icon})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// oauthProviderMgr returns the OAuthManager (pre-initialized in gateway.go).
func (gw *Gateway) oauthProviderMgr() *gatewayllm.OAuthManager {
	return gw.llmOAuthMgr
}

// handleOAuthProviderAppGet returns whether OAuth app credentials are
// configured for a provider, plus the redirect URI to register.
//
// GET /v1/providers/oauth/{provider}/app
func (gw *Gateway) handleOAuthProviderAppGet(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	spec, ok := gatewayllm.OAuthProviders[provider]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}
	redirectURI := ""
	if gw.llmOAuthMgr != nil {
		redirectURI = gw.llmOAuthMgr.CallbackURI(provider)
	}
	hasCreds := false
	if gw.llmOAuthMgr != nil {
		hasCreds = gw.llmOAuthMgr.HasClientCreds(provider)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":     provider,
		"name":         spec.Name,
		"pkce":         spec.PKCE,
		"redirect_uri": redirectURI,
		"has_creds":    hasCreds,
	})
}

// handleOAuthProviderAppSet saves OAuth app credentials (client_id + client_secret)
// for a provider to the vault, then updates the in-memory OAuthManager.
//
// POST /v1/providers/oauth/{provider}/app
func (gw *Gateway) handleOAuthProviderAppSet(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.vault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	spec, ok := gatewayllm.OAuthProviders[provider]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}
	if spec.PKCE {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "PKCE providers do not use client credentials"})
		return
	}

	var body struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.ClientID = strings.TrimSpace(body.ClientID)
	body.ClientSecret = strings.TrimSpace(body.ClientSecret)
	if body.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id is required"})
		return
	}

	data := vault.CredentialData{ClientID: body.ClientID, ClientSecret: body.ClientSecret}
	if _, err := gw.vault.Save(r.Context(), defaultTenant, llmOAuthAppPlatformID(provider), "default", "llm_oauth_app", data, nil, nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("vault write: %v", err)})
		return
	}
	if gw.llmOAuthMgr != nil {
		gw.llmOAuthMgr.SetClientCreds(provider, body.ClientID, body.ClientSecret)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "provider": provider})
}

// handleOAuthProviderAppDelete removes stored OAuth app credentials for a provider.
//
// DELETE /v1/providers/oauth/{provider}/app
func (gw *Gateway) handleOAuthProviderAppDelete(w http.ResponseWriter, r *http.Request) {
	if gw.vault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vault not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	if _, ok := gatewayllm.OAuthProviders[provider]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}
	_ = gw.vault.Delete(r.Context(), defaultTenant, llmOAuthAppPlatformID(provider))
	if gw.llmOAuthMgr != nil {
		gw.llmOAuthMgr.SetClientCreds(provider, "", "")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// llmOAuthAppPlatformID returns the vault platform_id for an LLM provider's
// OAuth app credentials. Prefixed to avoid collisions with user tokens.
func llmOAuthAppPlatformID(provider string) string {
	return "__llm_oauth_app_" + provider + "__"
}

// hydrateLLMOAuthCredsFromVault loads any previously-saved LLM OAuth app
// credentials from the vault into llmOAuthMgr at startup.
func (gw *Gateway) hydrateLLMOAuthCredsFromVault(ctx context.Context) {
	if gw.llmOAuthMgr == nil || gw.vault == nil {
		return
	}
	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for provider := range gatewayllm.OAuthProviders {
		cred, err := gw.vault.Get(hctx, defaultTenant, llmOAuthAppPlatformID(provider))
		if err != nil || cred == nil || cred.Data.ClientID == "" {
			continue
		}
		gw.llmOAuthMgr.SetClientCreds(provider, cred.Data.ClientID, cred.Data.ClientSecret)
	}
}

// writeHTML writes an HTML response.
func writeHTML(w http.ResponseWriter, code int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	w.Write([]byte(html))
}

// contains checks if s contains substr (case-sensitive substring).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}
