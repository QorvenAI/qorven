// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// OAuthProviderSpec defines the OAuth 2.0 parameters for a supported provider.
type OAuthProviderSpec struct {
	Name     string
	AuthURL  string
	TokenURL string
	Scopes   []string
	// PKCE=true means no client_secret is needed — the code_verifier proves
	// possession of the authorization code. Claude Code uses PKCE.
	PKCE bool
	// ClientIDEnvVar is the OS environment variable that holds the client_id.
	// If empty, users must supply their own client_id via settings.
	ClientIDEnvVar string
	// ProviderType is the internal LLM driver type to use after auth.
	ProviderType string
	// APIBase is the LLM API endpoint (blank = driver default).
	APIBase string
	// Icon is the icon stem served at /icons/providers/<icon>.webp.
	Icon string
}

// OAuthProviders is the registry of supported OAuth 2.0 providers.
var OAuthProviders = map[string]OAuthProviderSpec{
	"claude_code": {
		Name:         "Claude Code (Anthropic)",
		AuthURL:      "https://claude.ai/oauth/authorize",
		TokenURL:     "https://api.anthropic.com/oauth/token",
		Scopes:       []string{"chat:write", "models:read"},
		PKCE:         true,
		ProviderType: "anthropic_native",
		Icon:         "anthropic",
	},
	"github_copilot": {
		Name:           "GitHub Copilot",
		AuthURL:        "https://github.com/login/oauth/authorize",
		TokenURL:       "https://github.com/login/oauth/access_token",
		Scopes:         []string{"copilot"},
		PKCE:           false,
		ClientIDEnvVar: "GITHUB_COPILOT_CLIENT_ID",
		ProviderType:   "openai_compat",
		APIBase:        "https://api.githubcopilot.com",
		Icon:           "github",
	},
	"google_vertex": {
		Name:         "Google Vertex AI",
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
		PKCE:         false,
		ProviderType: "gemini_native",
		Icon:         "gemini",
	},
}

// oauthState holds the PKCE verifier and other in-flight state while the
// browser completes the OAuth redirect.
type oauthState struct {
	verifier    string
	challenge   string
	tenantID    string
	redirectURI string
	createdAt   time.Time
}

// OAuthManager handles the full OAuth 2.0 authorization code flow including
// PKCE support, token storage, and background refresh.
type OAuthManager struct {
	db         *pgxpool.Pool
	encryptKey string
	baseURL    string // e.g. "http://localhost:4200" — used to build redirect URIs

	mu     sync.Mutex
	states map[string]*oauthState // state param → flow state
}

// NewOAuthManager creates an OAuthManager. baseURL is the public URL of the
// backend (used to build the OAuth callback redirect URI).
func NewOAuthManager(db *pgxpool.Pool, encryptKey, baseURL string) *OAuthManager {
	m := &OAuthManager{
		db:         db,
		encryptKey: encryptKey,
		baseURL:    baseURL,
		states:     make(map[string]*oauthState),
	}
	go m.refreshWorker()
	return m
}

// StartURL returns the authorization URL to redirect the user to.
// For PKCE providers it generates a code_verifier / code_challenge pair and
// stores them in the in-flight state map. stateParam is returned for
// embedding in the redirect URI so the callback can retrieve the state.
func (m *OAuthManager) StartURL(tenantID, provider string) (redirectURL string, stateParam string, err error) {
	spec, ok := OAuthProviders[provider]
	if !ok {
		return "", "", fmt.Errorf("oauth: unknown provider %q", provider)
	}

	stateBytes := make([]byte, 16)
	if _, err = rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("oauth: failed to generate state: %w", err)
	}
	stateParam = hex.EncodeToString(stateBytes)

	flow := &oauthState{
		tenantID:    tenantID,
		redirectURI: m.callbackURI(provider),
		createdAt:   time.Now(),
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("state", stateParam)
	params.Set("redirect_uri", flow.redirectURI)
	params.Set("scope", strings.Join(spec.Scopes, " "))

	if spec.PKCE {
		verifier, challenge := generatePKCE()
		flow.verifier = verifier
		flow.challenge = challenge
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
		// PKCE: no client_id needed (Anthropic's PKCE flow is client-agnostic).
	}

	authURL, _ := url.Parse(spec.AuthURL)
	authURL.RawQuery = params.Encode()

	m.mu.Lock()
	// Expire states older than 10 minutes.
	for k, s := range m.states {
		if time.Since(s.createdAt) > 10*time.Minute {
			delete(m.states, k)
		}
	}
	m.states[stateParam] = flow
	m.mu.Unlock()

	return authURL.String(), stateParam, nil
}

// HandleCallback exchanges the authorization code for tokens and stores them
// encrypted in the oauth_tokens table. Returns the tenant ID that initiated
// the flow.
func (m *OAuthManager) HandleCallback(ctx context.Context, provider, code, stateParam string) (tenantID string, err error) {
	spec, ok := OAuthProviders[provider]
	if !ok {
		return "", fmt.Errorf("oauth: unknown provider %q", provider)
	}

	m.mu.Lock()
	flow, exists := m.states[stateParam]
	if exists {
		delete(m.states, stateParam)
	}
	m.mu.Unlock()

	if !exists {
		return "", fmt.Errorf("oauth: invalid or expired state parameter")
	}

	// Exchange authorization code for tokens.
	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("code", code)
	params.Set("redirect_uri", flow.redirectURI)
	if spec.PKCE {
		params.Set("code_verifier", flow.verifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.TokenURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return "", fmt.Errorf("oauth: failed to build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth: token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oauth: token exchange returned %d: %s", resp.StatusCode, body)
	}

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err = json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("oauth: failed to parse token response: %w", err)
	}

	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	if err = m.storeToken(ctx, flow.tenantID, provider, tok.AccessToken, tok.RefreshToken, expiresAt); err != nil {
		return "", err
	}
	return flow.tenantID, nil
}

// Token returns the decrypted access token for a provider, if one is stored
// and not expired.
func (m *OAuthManager) Token(ctx context.Context, tenantID, provider string) (string, error) {
	if m.db == nil {
		return "", fmt.Errorf("oauth: no database")
	}
	var encAccess []byte
	var expiresAt *time.Time
	err := m.db.QueryRow(ctx,
		`SELECT access_token, expires_at FROM oauth_tokens WHERE tenant_id = $1 AND provider = $2`,
		tenantID, provider,
	).Scan(&encAccess, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("oauth: no token for provider %q", provider)
	}
	if expiresAt != nil && time.Now().After(*expiresAt) {
		return "", fmt.Errorf("oauth: token expired for provider %q", provider)
	}
	plain, err2 := crypto.Decrypt(encAccess, m.encryptKey)
	if err2 != nil {
		return "", err2
	}
	return string(plain), nil
}

// Status returns whether an active, non-expired token exists for the provider.
func (m *OAuthManager) Status(ctx context.Context, tenantID, provider string) map[string]any {
	if m.db == nil {
		return map[string]any{"connected": false}
	}
	var expiresAt *time.Time
	var updatedAt time.Time
	err := m.db.QueryRow(ctx,
		`SELECT expires_at, updated_at FROM oauth_tokens WHERE tenant_id = $1 AND provider = $2`,
		tenantID, provider,
	).Scan(&expiresAt, &updatedAt)
	if err != nil {
		return map[string]any{"connected": false}
	}
	expired := expiresAt != nil && time.Now().After(*expiresAt)
	return map[string]any{
		"connected":  !expired,
		"expires_at": expiresAt,
		"updated_at": updatedAt,
	}
}

// Revoke deletes the stored token for a provider.
func (m *OAuthManager) Revoke(ctx context.Context, tenantID, provider string) error {
	if m.db == nil {
		return nil
	}
	_, err := m.db.Exec(ctx,
		`DELETE FROM oauth_tokens WHERE tenant_id = $1 AND provider = $2`,
		tenantID, provider)
	return err
}

func (m *OAuthManager) storeToken(ctx context.Context, tenantID, provider, accessToken, refreshToken string, expiresAt *time.Time) error {
	encAccess, err := crypto.Encrypt([]byte(accessToken), m.encryptKey)
	if err != nil {
		return fmt.Errorf("oauth: failed to encrypt access token: %w", err)
	}
	var encRefresh []byte
	if refreshToken != "" {
		encRefresh, err = crypto.Encrypt([]byte(refreshToken), m.encryptKey)
		if err != nil {
			return fmt.Errorf("oauth: failed to encrypt refresh token: %w", err)
		}
	}
	_, err = m.db.Exec(ctx, `
		INSERT INTO oauth_tokens (tenant_id, provider, access_token, refresh_token, expires_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (tenant_id, provider) DO UPDATE SET
		    access_token  = $3,
		    refresh_token = COALESCE($4, oauth_tokens.refresh_token),
		    expires_at    = $5,
		    updated_at    = now()
	`, tenantID, provider, encAccess, encRefresh, expiresAt)
	return err
}

func (m *OAuthManager) callbackURI(provider string) string {
	return m.baseURL + "/v1/providers/oauth/" + provider + "/callback"
}

// refreshWorker runs every 15 minutes and refreshes any token that expires
// within 30 minutes. Errors are logged but do not stop the goroutine.
func (m *OAuthManager) refreshWorker() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.refreshExpiringTokens()
	}
}

func (m *OAuthManager) refreshExpiringTokens() {
	if m.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := m.db.Query(ctx, `
		SELECT tenant_id, provider, refresh_token
		FROM   oauth_tokens
		WHERE  refresh_token IS NOT NULL
		  AND  expires_at IS NOT NULL
		  AND  expires_at < now() + interval '30 minutes'
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tenantID, provider string
		var encRefresh []byte
		if err = rows.Scan(&tenantID, &provider, &encRefresh); err != nil {
			continue
		}
		refreshBytes, decErr := crypto.Decrypt(encRefresh, m.encryptKey)
		if decErr != nil {
			continue
		}
		refreshToken := string(refreshBytes)
		spec, ok := OAuthProviders[provider]
		if !ok {
			continue
		}
		if err = m.doRefresh(ctx, tenantID, provider, refreshToken, spec.TokenURL); err != nil {
			slog.Warn("oauth.refresh: failed", "provider", provider, "error", err)
		}
	}
}

func (m *OAuthManager) doRefresh(ctx context.Context, tenantID, provider, refreshToken, tokenURL string) error {
	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh returned %d: %s", resp.StatusCode, body)
	}

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err = json.Unmarshal(body, &tok); err != nil {
		return err
	}

	var expiresAt *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	// Use the new refresh token if provided, otherwise keep the old one.
	newRefresh := tok.RefreshToken
	if newRefresh == "" {
		newRefresh = refreshToken
	}
	return m.storeToken(ctx, tenantID, provider, tok.AccessToken, newRefresh, expiresAt)
}

// generatePKCE creates a code_verifier (43 random bytes base64url-encoded)
// and its SHA-256 code_challenge.
func generatePKCE() (verifier, challenge string) {
	b := make([]byte, 43)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}
