// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// CredentialData holds the decrypted credential fields.
type CredentialData struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// Credential is a stored credential record.
type Credential struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	PlatformID string         `json:"platform_id"`
	Label      string         `json:"label"`
	AuthType   string         `json:"auth_type"`
	Scopes     []string       `json:"scopes"`
	ExpiresAt  *time.Time     `json:"expires_at,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Data       CredentialData `json:"-"` // never serialized directly
}

// Vault manages encrypted credential storage.
type Vault struct {
	pool      *pgxpool.Pool
	encKey    string // hex-encoded 32-byte AES key
}

func New(pool *pgxpool.Pool, encryptionKey string) *Vault {
	return &Vault{pool: pool, encKey: encryptionKey}
}

// Save encrypts and stores a credential. Upserts on (tenant_id, platform_id, label).
func (v *Vault) Save(ctx context.Context, tenantID, platformID, label, authType string, data CredentialData, scopes []string, expiresAt *time.Time) (*Credential, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal credential: %w", err)
	}
	encrypted, err := crypto.Encrypt(plaintext, v.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt credential: %w", err)
	}
	if label == "" {
		label = "default"
	}
	if scopes == nil {
		scopes = []string{}
	}

	var id string
	err = v.pool.QueryRow(ctx,
		`INSERT INTO credentials (tenant_id, platform_id, label, auth_type, data_encrypted, scopes, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, platform_id, label) DO UPDATE
		 SET data_encrypted = $5, scopes = $6, expires_at = $7, auth_type = $4, updated_at = NOW()
		 RETURNING id`,
		tenantID, platformID, label, authType, encrypted, scopes, expiresAt,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("save credential: %w", err)
	}

	return &Credential{
		ID: id, TenantID: tenantID, PlatformID: platformID,
		Label: label, AuthType: authType, Scopes: scopes, ExpiresAt: expiresAt,
	}, nil
}

// Get retrieves and decrypts a credential for a platform.
func (v *Vault) Get(ctx context.Context, tenantID, platformID string) (*Credential, error) {
	var c Credential
	encrypted := []byte{}
	err := v.pool.QueryRow(ctx,
		`SELECT id, tenant_id, platform_id, label, auth_type, data_encrypted, scopes, expires_at, created_at, updated_at
		 FROM credentials WHERE tenant_id = $1 AND platform_id = $2
		 ORDER BY updated_at DESC LIMIT 1`,
		tenantID, platformID,
	).Scan(&c.ID, &c.TenantID, &c.PlatformID, &c.Label, &c.AuthType, &encrypted, &c.Scopes, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("credential not found for %s: %w", platformID, err)
	}

	plaintext, err := crypto.Decrypt(encrypted, v.encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt credential: %w", err)
	}
	if err := json.Unmarshal(plaintext, &c.Data); err != nil {
		return nil, fmt.Errorf("unmarshal credential: %w", err)
	}
	return &c, nil
}

// GetToken returns a ready-to-use auth header value. Auto-refreshes OAuth if expired.
func (v *Vault) GetToken(ctx context.Context, tenantID, platformID string, refreshFn func(refreshToken string) (*CredentialData, *time.Time, error)) (string, error) {
	cred, err := v.Get(ctx, tenantID, platformID)
	if err != nil {
		return "", err
	}

	// API key — never expires
	if cred.AuthType == "api_key" {
		return cred.Data.APIKey, nil
	}

	// Bearer token — check expiry
	if cred.ExpiresAt != nil && time.Now().After(*cred.ExpiresAt) && cred.Data.RefreshToken != "" && refreshFn != nil {
		slog.Info("vault.refreshing_token", "platform", platformID)
		newData, newExpiry, err := refreshFn(cred.Data.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("refresh token for %s: %w", platformID, err)
		}
		// Preserve refresh token if new one not provided
		if newData.RefreshToken == "" {
			newData.RefreshToken = cred.Data.RefreshToken
		}
		if _, err := v.Save(ctx, tenantID, platformID, cred.Label, cred.AuthType, *newData, cred.Scopes, newExpiry); err != nil {
			return "", fmt.Errorf("save refreshed token: %w", err)
		}
		return newData.AccessToken, nil
	}

	return cred.Data.AccessToken, nil
}

// Delete removes a credential.
func (v *Vault) Delete(ctx context.Context, tenantID, platformID string) error {
	_, err := v.pool.Exec(ctx, `DELETE FROM credentials WHERE tenant_id = $1 AND platform_id = $2`, tenantID, platformID)
	return err
}

// List returns all credentials for a tenant (without decrypted data).
func (v *Vault) List(ctx context.Context, tenantID string) ([]Credential, error) {
	rows, err := v.pool.Query(ctx,
		`SELECT id, tenant_id, platform_id, label, auth_type, scopes, expires_at, created_at, updated_at
		 FROM credentials WHERE tenant_id = $1 ORDER BY platform_id`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	creds := []Credential{}
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.TenantID, &c.PlatformID, &c.Label, &c.AuthType, &c.Scopes, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		creds = append(creds, c)
	}
	return creds, nil
}

// IsConnected checks if a credential exists for a platform.
func (v *Vault) IsConnected(ctx context.Context, tenantID, platformID string) bool {
	var exists bool
	v.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM credentials WHERE tenant_id = $1 AND platform_id = $2)`,
		tenantID, platformID,
	).Scan(&exists)
	return exists
}
