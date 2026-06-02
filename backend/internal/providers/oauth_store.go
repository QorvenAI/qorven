// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package providers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// OAuthProviderSpec defines the OAuth endpoints for a provider.
type OAuthProviderSpec struct {
	Name         string
	AuthURL      string
	TokenURL     string
	Scopes       []string
	PKCE         bool   // true = PKCE flow, no client_secret needed
	ClientID     string // empty = user must provide or use PKCE
	APIBase      string // optional: override API base URL for this provider
	ProviderType string // maps to existing provider type constants
}

// OAuthProviders lists supported OAuth providers.
var OAuthProviders = map[string]OAuthProviderSpec{
	"claude_code": {
		Name:         "Claude Code",
		AuthURL:      "https://claude.ai/oauth/authorize",
		TokenURL:     "https://api.anthropic.com/oauth/token",
		Scopes:       []string{"chat:write", "models:read"},
		PKCE:         true,
		ProviderType: "anthropic_native",
	},
	"github_copilot": {
		Name:         "GitHub Copilot",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		Scopes:       []string{"copilot"},
		PKCE:         false,
		APIBase:      "https://api.githubcopilot.com",
		ProviderType: "openai_compat",
	},
	"gemini_oauth": {
		Name:         "Google (Gemini)",
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"https://www.googleapis.com/auth/generative-language"},
		PKCE:         false,
		ProviderType: "gemini_native",
	},
}

// OAuthTokenStore saves and retrieves encrypted OAuth tokens from the database.
type OAuthTokenStore struct {
	pool   *pgxpool.Pool
	encKey string
}

// NewOAuthTokenStore creates an OAuthTokenStore backed by the given pool.
func NewOAuthTokenStore(pool *pgxpool.Pool, encKey string) *OAuthTokenStore {
	return &OAuthTokenStore{pool: pool, encKey: encKey}
}

// GeneratePKCE creates a code_verifier and code_challenge (S256 method).
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

// GenerateState creates a random OAuth state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SavePKCEState stores the PKCE state for the OAuth callback.
func (s *OAuthTokenStore) SavePKCEState(ctx context.Context, state, provider, tenantID, verifier, redirectURI string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_pkce_state (state, provider, tenant_id, code_verifier, redirect_uri)
         VALUES ($1, $2, $3::uuid, $4, $5)
         ON CONFLICT (state) DO UPDATE SET code_verifier = $4, expires_at = now() + interval '10 minutes'`,
		state, provider, tenantID, verifier, redirectURI)
	return err
}

// GetPKCEState retrieves and deletes the PKCE state (single-use).
func (s *OAuthTokenStore) GetPKCEState(ctx context.Context, state string) (provider, tenantID, verifier, redirectURI string, err error) {
	err = s.pool.QueryRow(ctx,
		`DELETE FROM oauth_pkce_state WHERE state = $1 AND expires_at > now()
         RETURNING provider, tenant_id::text, code_verifier, redirect_uri`,
		state).Scan(&provider, &tenantID, &verifier, &redirectURI)
	return
}

// OAuthToken holds a decrypted token pair.
type OAuthToken struct {
	Provider     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
	Scopes       []string
	TokenType    string
	ProviderType string
	PKCEUsed     bool
}

// Save stores an OAuth token pair (encrypting both tokens).
func (s *OAuthTokenStore) Save(ctx context.Context, tenantID, provider string, tok OAuthToken) error {
	encAccess, err := crypto.Encrypt([]byte(tok.AccessToken), s.encKey)
	if err != nil {
		return err
	}
	var encRefresh []byte
	if tok.RefreshToken != "" {
		encRefresh, err = crypto.Encrypt([]byte(tok.RefreshToken), s.encKey)
		if err != nil {
			return err
		}
	}
	// scopes is TEXT[] — pass []string directly; pgx serialises it as a Postgres array.
	scopes := tok.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO oauth_tokens
             (tenant_id, provider, access_token, refresh_token, expires_at, scopes, token_type, provider_type, pkce_used, updated_at)
         VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, now())
         ON CONFLICT (tenant_id, provider) DO UPDATE SET
             access_token  = $3,
             refresh_token = COALESCE($4, oauth_tokens.refresh_token),
             expires_at    = $5,
             scopes        = $6,
             token_type    = $7,
             provider_type = $8,
             pkce_used     = $9,
             updated_at    = now()`,
		tenantID, provider, encAccess, encRefresh, tok.ExpiresAt,
		scopes, tok.TokenType, tok.ProviderType, tok.PKCEUsed)
	return err
}

// Get retrieves and decrypts the stored token for a provider.
func (s *OAuthTokenStore) Get(ctx context.Context, tenantID, provider string) (*OAuthToken, error) {
	var encAccess, encRefresh []byte
	var tok OAuthToken
	err := s.pool.QueryRow(ctx,
		`SELECT access_token, refresh_token, expires_at, scopes, token_type, provider_type, pkce_used
         FROM oauth_tokens WHERE tenant_id = $1::uuid AND provider = $2`,
		tenantID, provider).Scan(
		&encAccess, &encRefresh, &tok.ExpiresAt, &tok.Scopes,
		&tok.TokenType, &tok.ProviderType, &tok.PKCEUsed)
	if err != nil {
		return nil, err
	}
	plain, err := crypto.Decrypt(encAccess, s.encKey)
	if err != nil {
		return nil, err
	}
	tok.AccessToken = string(plain)
	if len(encRefresh) > 0 {
		refreshPlain, decErr := crypto.Decrypt(encRefresh, s.encKey)
		if decErr != nil {
			return nil, decErr
		}
		tok.RefreshToken = string(refreshPlain)
	}
	tok.Provider = provider
	return &tok, nil
}

// IsExpired returns true if the token is expired or expiring in <5 minutes.
func (tok *OAuthToken) IsExpired() bool {
	if tok.ExpiresAt == nil {
		return false // no expiry = long-lived token (e.g. GitHub PAT)
	}
	return time.Until(*tok.ExpiresAt) < 5*time.Minute
}

// Revoke deletes the stored token.
func (s *OAuthTokenStore) Revoke(ctx context.Context, tenantID, provider string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM oauth_tokens WHERE tenant_id = $1::uuid AND provider = $2`,
		tenantID, provider)
	return err
}

// Status returns connection status for a provider: "connected", "expired", "not_connected"
func (s *OAuthTokenStore) Status(ctx context.Context, tenantID, provider string) string {
	tok, err := s.Get(ctx, tenantID, provider)
	if err != nil {
		return "not_connected"
	}
	if tok.IsExpired() {
		return "expired"
	}
	return "connected"
}

// CleanExpiredPKCEStates removes expired PKCE state rows (call periodically).
func (s *OAuthTokenStore) CleanExpiredPKCEStates(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM oauth_pkce_state WHERE expires_at <= now()`)
	return err
}
