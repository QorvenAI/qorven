// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// DBCredentialStore stores encrypted credentials in connector_credentials table.
type DBCredentialStore struct {
	pool          *pgxpool.Pool
	encryptionKey string
}

func NewDBCredentialStore(pool *pgxpool.Pool, encryptionKey string) *DBCredentialStore {
	return &DBCredentialStore{pool: pool, encryptionKey: encryptionKey}
}

// GetCredentials retrieves and decrypts credentials for a connector.
func (s *DBCredentialStore) GetCredentials(ctx context.Context, agentID, connectorID string) (map[string]string, error) {
	var encrypted []byte
	err := s.pool.QueryRow(ctx,
		`SELECT credentials FROM connector_credentials WHERE agent_id = $1 AND connector_id = $2 AND status = 'active'`,
		agentID, connectorID).Scan(&encrypted)
	if err != nil {
		return nil, fmt.Errorf("no credentials for %s", connectorID)
	}

	// If encryption key is set, decrypt
	var raw []byte
	if s.encryptionKey != "" && len(encrypted) > 0 {
		raw, err = crypto.Decrypt(encrypted, s.encryptionKey)
		if err != nil {
			// Fallback: try as plain JSON (for migration from unencrypted)
			raw = encrypted
		}
	} else {
		raw = encrypted
	}

	var creds map[string]string
	if err := json.Unmarshal(raw, &creds); err != nil {
		return nil, fmt.Errorf("invalid credentials format")
	}
	return creds, nil
}

// SaveCredentials encrypts and stores credentials.
func (s *DBCredentialStore) SaveCredentials(ctx context.Context, tenantID, agentID, connectorID string, creds map[string]string) error {
	raw, _ := json.Marshal(creds)

	var toStore []byte
	if s.encryptionKey != "" {
		encrypted, err := crypto.Encrypt(raw, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
		toStore = encrypted
	} else {
		toStore = raw
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO connector_credentials (tenant_id, agent_id, connector_id, credentials)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (agent_id, connector_id) DO UPDATE SET credentials = $4, updated_at = now()`,
		tenantID, agentID, connectorID, toStore)
	return err
}

// DeleteCredentials removes stored credentials.
func (s *DBCredentialStore) DeleteCredentials(ctx context.Context, agentID, connectorID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM connector_credentials WHERE agent_id = $1 AND connector_id = $2`,
		agentID, connectorID)
	return err
}
