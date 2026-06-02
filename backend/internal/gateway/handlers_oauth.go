// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// handlers_oauth.go provides OAuth token management handlers that use the
// providers.OAuthTokenStore directly (as opposed to the gateway/llm.OAuthManager
// used by handlers_oauth_providers.go). These handlers complement the existing
// OAuth flow with a token-store–level API, and expose RefreshOAuthToken for
// internal callers (e.g. agent runners that need a fresh access token).

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/providers"
)

// handleOAuthStart returns the provider's authorization URL plus state param.
//
// GET /v1/providers/oauth/:provider/start
// (Registered as the store-backed variant alongside handleOAuthProviderStart.)
func (gw *Gateway) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	spec, ok := providers.OAuthProviders[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported OAuth provider: " + provider})
		return
	}

	store := providers.NewOAuthTokenStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	state, err := providers.GenerateState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate state"})
		return
	}

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		redirectURI = fmt.Sprintf("%s://%s/models-hub/generative?oauth_callback=1", scheme, r.Host)
	}

	params := url.Values{
		"response_type": {"code"},
		"redirect_uri":  {redirectURI},
		"scope":         {strings.Join(spec.Scopes, " ")},
		"state":         {state},
	}
	if spec.ClientID != "" {
		params.Set("client_id", spec.ClientID)
	}

	var verifier string
	if spec.PKCE {
		var challenge string
		verifier, challenge, err = providers.GeneratePKCE()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate PKCE"})
			return
		}
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
	}

	if err := store.SavePKCEState(r.Context(), state, provider, defaultTenant, verifier, redirectURI); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save OAuth state"})
		return
	}

	authURL := spec.AuthURL + "?" + params.Encode()
	writeJSON(w, http.StatusOK, map[string]string{"auth_url": authURL, "state": state})
}

// handleOAuthTokenExchange exchanges the authorization code for tokens using
// the providers.OAuthTokenStore. The route differs from handleOAuthCallback
// (which is a social OAuth callback) by operating under /providers/oauth/.
//
// GET /v1/providers/oauth/:provider/exchange
func (gw *Gateway) handleOAuthTokenExchange(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	spec, ok := providers.OAuthProviders[provider]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported OAuth provider"})
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing code or state"})
		return
	}

	store := providers.NewOAuthTokenStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	storedProvider, tenantID, verifier, redirectURI, err := store.GetPKCEState(r.Context(), state)
	if err != nil || storedProvider != provider {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired OAuth state"})
		return
	}

	formData := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	if spec.PKCE {
		formData.Set("code_verifier", verifier)
	}
	if spec.ClientID != "" {
		formData.Set("client_id", spec.ClientID)
	}

	resp, err := http.PostForm(spec.TokenURL, formData) //nolint:noctx
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "token exchange failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.AccessToken == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid token response from provider"})
		return
	}

	var expiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	var scopes []string
	if tokenResp.Scope != "" {
		scopes = strings.Split(tokenResp.Scope, " ")
	} else {
		scopes = spec.Scopes
	}

	tok := providers.OAuthToken{
		Provider:     provider,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    expiresAt,
		Scopes:       scopes,
		TokenType:    tokenResp.TokenType,
		ProviderType: spec.ProviderType,
		PKCEUsed:     spec.PKCE,
	}

	if err := store.Save(r.Context(), tenantID, provider, tok); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"provider":      provider,
		"provider_name": spec.Name,
		"expires_at":    expiresAt,
		"scopes":        scopes,
	})
}

// handleOAuthStatus returns connection status for a provider via OAuthTokenStore.
//
// GET /v1/providers/oauth/:provider/token-status
func (gw *Gateway) handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	if _, ok := providers.OAuthProviders[provider]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported OAuth provider"})
		return
	}
	store := providers.NewOAuthTokenStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	status := store.Status(r.Context(), defaultTenant, provider)

	resp := map[string]any{"provider": provider, "status": status}
	if status == "connected" {
		tok, _ := store.Get(r.Context(), defaultTenant, provider)
		if tok != nil {
			resp["expires_at"] = tok.ExpiresAt
			resp["scopes"] = tok.Scopes
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleOAuthRevoke deletes stored tokens for a provider via OAuthTokenStore.
//
// DELETE /v1/providers/oauth/:provider/token
func (gw *Gateway) handleOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	provider := chi.URLParam(r, "provider")
	store := providers.NewOAuthTokenStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	if err := store.Revoke(r.Context(), defaultTenant, provider); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "provider": provider})
}

// RefreshOAuthToken refreshes an expired token using the stored refresh_token.
// Called by agent runners before using an OAuth-backed provider.
func (gw *Gateway) RefreshOAuthToken(ctx context.Context, tenantID, provider string) error {
	if gw.db == nil {
		return fmt.Errorf("database not available")
	}
	spec, ok := providers.OAuthProviders[provider]
	if !ok {
		return fmt.Errorf("unsupported OAuth provider: %s", provider)
	}

	store := providers.NewOAuthTokenStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	tok, err := store.Get(ctx, tenantID, provider)
	if err != nil {
		return fmt.Errorf("token not found: %w", err)
	}
	if tok.RefreshToken == "" {
		return fmt.Errorf("no refresh token available for %s", provider)
	}

	formData := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
	}
	if spec.ClientID != "" {
		formData.Set("client_id", spec.ClientID)
	}

	resp, err := http.PostForm(spec.TokenURL, formData) //nolint:noctx
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.AccessToken == "" {
		return fmt.Errorf("invalid refresh response from %s", provider)
	}

	var expiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	newRefresh := tok.RefreshToken
	if tokenResp.RefreshToken != "" {
		newRefresh = tokenResp.RefreshToken
	}
	updated := *tok
	updated.AccessToken = tokenResp.AccessToken
	updated.RefreshToken = newRefresh
	updated.ExpiresAt = expiresAt
	return store.Save(ctx, tenantID, provider, updated)
}
