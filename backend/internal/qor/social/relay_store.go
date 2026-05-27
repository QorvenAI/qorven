// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// RelayStore manages multi-key relay provider credentials in the relay_providers table.
// A tenant can store multiple API keys per relay provider (e.g. 2 Outstand keys, 3 PostForMe keys).
type RelayStore struct {
	pool   *pgxpool.Pool
	encKey []byte
}

// NewRelayStore creates a RelayStore. encryptionKey must be 32 raw bytes (used as AES-256 key).
func NewRelayStore(pool *pgxpool.Pool, encryptionKey []byte) *RelayStore {
	return &RelayStore{pool: pool, encKey: encryptionKey}
}

// keyHex returns the encryption key as a 64-char hex string for the crypto package.
func (s *RelayStore) keyHex() string {
	return hex.EncodeToString(s.encKey)
}

// RelayProviderRecord represents a relay provider key row with joined accounts count.
type RelayProviderRecord struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	Provider      string     `json:"provider"`
	Label         string     `json:"label"`
	Status        string     `json:"status"`
	TotalPosts    int64      `json:"total_posts"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	AccountsCount int        `json:"accounts_count"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AddKey adds a new relay key (always INSERT, never upsert — multi-key).
// Returns the new row's UUID.
func (s *RelayStore) AddKey(ctx context.Context, tenantID, provider, label, apiKey string) (string, error) {
	encrypted, err := crypto.Encrypt([]byte(apiKey), s.keyHex())
	if err != nil {
		return "", fmt.Errorf("encrypt relay key: %w", err)
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO relay_providers (tenant_id, provider, label, api_key, status)
		 VALUES ($1::uuid, $2, $3, $4, 'active')
		 RETURNING id`,
		tenantID, provider, label, encrypted).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert relay key: %w", err)
	}
	return id, nil
}

// GetKeyByID retrieves a decrypted API key by relay_providers.id.
func (s *RelayStore) GetKeyByID(ctx context.Context, keyID string) (string, error) {
	var ciphertext []byte
	err := s.pool.QueryRow(ctx,
		`SELECT api_key FROM relay_providers WHERE id = $1::uuid`, keyID).Scan(&ciphertext)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("relay key not found: %s", keyID)
		}
		return "", err
	}

	plaintext, err := crypto.Decrypt(ciphertext, s.keyHex())
	if err != nil {
		return "", fmt.Errorf("decrypt relay key: %w", err)
	}
	return string(plaintext), nil
}

// GetKeyWithProvider retrieves the provider name and decrypted API key by relay_providers.id.
func (s *RelayStore) GetKeyWithProvider(ctx context.Context, keyID string) (provider string, apiKey string, err error) {
	var ciphertext []byte
	err = s.pool.QueryRow(ctx,
		`SELECT provider, api_key FROM relay_providers WHERE id = $1::uuid`, keyID).Scan(&provider, &ciphertext)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", fmt.Errorf("relay key not found: %s", keyID)
		}
		return "", "", err
	}

	plaintext, decErr := crypto.Decrypt(ciphertext, s.keyHex())
	if decErr != nil {
		return "", "", fmt.Errorf("decrypt relay key: %w", decErr)
	}
	return provider, string(plaintext), nil
}

// GetFirstKey gets the first active key for a provider type (fallback).
// Returns (keyID, decryptedKey, error).
func (s *RelayStore) GetFirstKey(ctx context.Context, tenantID, provider string) (string, string, error) {
	var id string
	var ciphertext []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, api_key FROM relay_providers
		 WHERE tenant_id = $1::uuid AND provider = $2 AND status = 'active'
		 ORDER BY created_at ASC LIMIT 1`,
		tenantID, provider).Scan(&id, &ciphertext)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", fmt.Errorf("no active relay key for provider %s", provider)
		}
		return "", "", err
	}

	plaintext, err := crypto.Decrypt(ciphertext, s.keyHex())
	if err != nil {
		return "", "", fmt.Errorf("decrypt relay key: %w", err)
	}
	return id, string(plaintext), nil
}

// UpdateKey updates label or status of an existing key.
func (s *RelayStore) UpdateKey(ctx context.Context, keyID, label, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE relay_providers SET label = $1, status = $2, updated_at = now()
		 WHERE id = $3::uuid`,
		label, status, keyID)
	return err
}

// DeleteKey removes a specific key by ID.
func (s *RelayStore) DeleteKey(ctx context.Context, keyID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM relay_providers WHERE id = $1::uuid`, keyID)
	return err
}

// ListKeys lists all relay keys for a tenant with accounts_count
// (LEFT JOIN to social_integrations on relay_provider_key_id).
func (s *RelayStore) ListKeys(ctx context.Context, tenantID string) ([]RelayProviderRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT rp.id, rp.tenant_id, rp.provider, COALESCE(rp.label, ''), rp.status,
		        rp.total_posts, rp.last_used_at, COUNT(si.id)::int AS accounts_count, rp.created_at
		 FROM relay_providers rp
		 LEFT JOIN social_integrations si ON si.relay_provider_key_id = rp.id
		 WHERE rp.tenant_id = $1::uuid
		 GROUP BY rp.id
		 ORDER BY rp.provider, rp.created_at`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RelayProviderRecord
	for rows.Next() {
		var r RelayProviderRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Provider, &r.Label, &r.Status,
			&r.TotalPosts, &r.LastUsedAt, &r.AccountsCount, &r.CreatedAt); err != nil {
			continue
		}
		records = append(records, r)
	}
	return records, nil
}

// HasAnyProvider checks if tenant has at least one active key for a provider type.
func (s *RelayStore) HasAnyProvider(ctx context.Context, tenantID, provider string) bool {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM relay_providers
		 WHERE tenant_id = $1::uuid AND provider = $2 AND status = 'active'`,
		tenantID, provider).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// IncrementPostCount bumps total_posts and last_used_at. Fire-and-forget (ignores errors).
func (s *RelayStore) IncrementPostCount(ctx context.Context, keyID string) {
	//nolint:errcheck
	s.pool.Exec(ctx,
		`UPDATE relay_providers SET total_posts = total_posts + 1, last_used_at = now(), updated_at = now()
		 WHERE id = $1::uuid`, keyID)
}
