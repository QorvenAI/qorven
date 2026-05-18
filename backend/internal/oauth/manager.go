// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/vault"
)

// ProviderConfig defines an OAuth2 provider's endpoints and credentials.
type ProviderConfig struct {
	ID           string   `json:"id"`            // "google", "slack", "github"
	Name         string   `json:"name"`
	AuthURL      string   `json:"auth_url"`
	TokenURL     string   `json:"token_url"`
	Scopes       []string `json:"scopes"`
	ClientID     string   `json:"client_id"`     // from user config
	ClientSecret string   `json:"client_secret"` // from user config
	ExtraParams  map[string]string `json:"extra_params,omitempty"` // e.g. access_type=offline
}

// TokenResponse is the OAuth2 token endpoint response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// Manager handles OAuth2 flows for all providers.
type Manager struct {
	providers   map[string]ProviderConfig
	vault       *vault.Vault
	redirectBase string // e.g. "https://app.qorven.ai" or "http://localhost:4200"
	client      *http.Client
}

func NewManager(v *vault.Vault, redirectBase string) *Manager {
	return &Manager{
		providers:    make(map[string]ProviderConfig),
		vault:        v,
		redirectBase: strings.TrimRight(redirectBase, "/"),
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

// RegisterProvider adds or updates an OAuth provider config.
func (m *Manager) RegisterProvider(cfg ProviderConfig) {
	m.providers[cfg.ID] = cfg
}

// GetProvider returns a provider config.
func (m *Manager) GetProvider(id string) (ProviderConfig, bool) {
	p, ok := m.providers[id]
	return p, ok
}

// ListProviders returns all registered providers.
func (m *Manager) ListProviders() []ProviderConfig {
	out := make([]ProviderConfig, 0, len(m.providers))
	for _, p := range m.providers {
		out = append(out, p)
	}
	return out
}

// RedirectURL returns the callback URL for a provider.
func (m *Manager) RedirectURL(providerID string) string {
	return m.redirectBase + "/v1/oauth/" + providerID + "/callback"
}

// AuthorizeURL builds the OAuth2 authorization URL the user should visit.
func (m *Manager) AuthorizeURL(providerID, state string) (string, error) {
	p, ok := m.providers[providerID]
	if !ok {
		return "", fmt.Errorf("unknown provider: %s", providerID)
	}

	params := url.Values{
		"client_id":     {p.ClientID},
		"redirect_uri":  {m.RedirectURL(providerID)},
		"response_type": {"code"},
		"scope":         {strings.Join(p.Scopes, " ")},
		"state":         {state},
	}
	for k, v := range p.ExtraParams {
		params.Set(k, v)
	}
	return p.AuthURL + "?" + params.Encode(), nil
}

// HandleCallback exchanges the authorization code for tokens and saves to vault.
func (m *Manager) HandleCallback(ctx context.Context, providerID, tenantID, code string) (*vault.Credential, error) {
	p, ok := m.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", providerID)
	}

	tok, err := m.exchangeCode(p, code, m.RedirectURL(providerID))
	if err != nil {
		return nil, fmt.Errorf("exchange code for %s: %w", providerID, err)
	}

	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	data := vault.CredentialData{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
	}

	scopes := p.Scopes
	if tok.Scope != "" {
		scopes = strings.Split(tok.Scope, " ")
	}

	// Map provider ID to platform ID (google → gmail, google-sheets, etc.)
	// For now, store under the provider ID; the connector layer maps it
	platformID := providerID
	return m.vault.Save(ctx, tenantID, platformID, "default", "oauth2", data, scopes, expiresAt)
}

// RefreshToken refreshes an OAuth2 access token using the refresh token.
func (m *Manager) RefreshToken(providerID, refreshToken string) (*vault.CredentialData, *time.Time, error) {
	p, ok := m.providers[providerID]
	if !ok {
		return nil, nil, fmt.Errorf("unknown provider: %s", providerID)
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
	}

	resp, err := m.client.PostForm(p.TokenURL, form)
	if err != nil {
		return nil, nil, fmt.Errorf("refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, nil, fmt.Errorf("parse refresh response: %w", err)
	}

	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	data := &vault.CredentialData{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenType:    tok.TokenType,
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
	}
	return data, expiresAt, nil
}

// MakeRefreshFunc returns a refresh function bound to a provider, for use with vault.GetToken.
func (m *Manager) MakeRefreshFunc(providerID string) func(string) (*vault.CredentialData, *time.Time, error) {
	return func(refreshToken string) (*vault.CredentialData, *time.Time, error) {
		return m.RefreshToken(providerID, refreshToken)
	}
}

func (m *Manager) exchangeCode(p ProviderConfig, code, redirectURI string) (*TokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
	}

	resp, err := m.client.PostForm(p.TokenURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &tok, nil
}

// RegisterDefaults registers the well-known OAuth2 providers.
// Users must set client_id and client_secret via config.
func RegisterDefaults(m *Manager, configs map[string]struct{ ClientID, ClientSecret string }) {
	defaults := []ProviderConfig{
		{
			ID: "google", Name: "Google",
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes: []string{
				"https://www.googleapis.com/auth/gmail.modify",
				"https://www.googleapis.com/auth/spreadsheets",
				"https://www.googleapis.com/auth/drive",
				"https://www.googleapis.com/auth/calendar",
				"https://www.googleapis.com/auth/documents",
			},
			ExtraParams: map[string]string{"access_type": "offline", "prompt": "consent"},
		},
		{
			ID: "microsoft", Name: "Microsoft",
			AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:   []string{"offline_access", "Mail.ReadWrite", "Mail.Send", "Calendars.ReadWrite", "Files.ReadWrite.All", "Team.ReadBasic.All"},
		},
		{
			ID: "slack", Name: "Slack",
			AuthURL:  "https://slack.com/oauth/v2/authorize",
			TokenURL: "https://slack.com/api/oauth.v2.access",
			Scopes:   []string{"chat:write", "channels:read", "channels:history", "users:read"},
		},
		{
			ID: "github", Name: "GitHub",
			AuthURL:  "https://github.com/login/oauth/authorize",
			TokenURL: "https://github.com/login/oauth/access_token",
			Scopes:   []string{"repo", "read:org", "read:user"},
		},
		{
			ID: "twitter", Name: "Twitter / X",
			AuthURL:  "https://twitter.com/i/oauth2/authorize",
			TokenURL: "https://api.twitter.com/2/oauth2/token",
			Scopes:   []string{"tweet.read", "tweet.write", "users.read", "offline.access"},
			ExtraParams: map[string]string{"code_challenge_method": "S256"},
		},
		{
			ID: "linkedin", Name: "LinkedIn",
			AuthURL:  "https://www.linkedin.com/oauth/v2/authorization",
			TokenURL: "https://www.linkedin.com/oauth/v2/accessToken",
			Scopes:   []string{"openid", "profile", "w_member_social"},
		},
		{
			ID: "salesforce", Name: "Salesforce",
			AuthURL:  "https://login.salesforce.com/services/oauth2/authorize",
			TokenURL: "https://login.salesforce.com/services/oauth2/token",
			Scopes:   []string{"api", "refresh_token"},
		},
		{
			ID: "shopify", Name: "Shopify",
			AuthURL:  "https://{shop}.myshopify.com/admin/oauth/authorize",
			TokenURL: "https://{shop}.myshopify.com/admin/oauth/access_token",
			Scopes:   []string{"read_products", "write_products", "read_orders", "write_orders"},
		},
	}

	for _, d := range defaults {
		if cfg, ok := configs[d.ID]; ok {
			d.ClientID = cfg.ClientID
			d.ClientSecret = cfg.ClientSecret
		}
		m.RegisterProvider(d)
	}
}
