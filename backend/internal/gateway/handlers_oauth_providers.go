// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	gatewayllm "github.com/qorvenai/qorven/internal/gateway/llm"
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
			`<html><body><p>OAuth error: `+errParam+`</p><script>window.close();</script></body></html>`)
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
			`<html><body><p>OAuth failed: `+err.Error()+`</p><script>window.close();</script></body></html>`)
		return
	}

	// Close the popup and signal success to the opener window.
	writeHTML(w, http.StatusOK, `<html><body>
<p>Connected! You can close this window.</p>
<script>
  if (window.opener) {
    window.opener.postMessage({ type: 'oauth_complete', provider: '`+provider+`' }, '*');
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
