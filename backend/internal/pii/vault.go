// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package pii

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Vault stores encrypted PII originals with per-agent access control.
// When content is redacted for storage, the original value goes here
// so authorized agents can retrieve it when needed (e.g., to reply to
// a customer email).
type Vault struct {
	pool     *pgxpool.Pool
	aead     cipher.AEAD
	tenantID string
}

// NewVault creates a vault backed by PostgreSQL and AES-256-GCM.
func NewVault(pool *pgxpool.Pool, aead cipher.AEAD, tenantID string) *Vault {
	return &Vault{pool: pool, aead: aead, tenantID: tenantID}
}

// Store encrypts a PII value and returns a vault token.
func (v *Vault) Store(ctx context.Context, agentID string, kind Kind, plaintext string) (string, error) {
	if v.pool == nil || v.aead == nil {
		return "", fmt.Errorf("vault: not configured")
	}

	token := generateToken()
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("vault: nonce generation failed: %w", err)
	}
	ciphertext := v.aead.Seal(nonce, nonce, []byte(plaintext), nil)

	_, err := v.pool.Exec(ctx, `
		INSERT INTO pii_vault (id, tenant_id, agent_id, kind, ciphertext, access_list, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		token, v.tenantID, agentID, kind.String(), ciphertext,
		[]string{agentID},
		time.Now().Add(30*24*time.Hour),
	)
	if err != nil {
		return "", fmt.Errorf("vault: store failed: %w", err)
	}
	return token, nil
}

// Retrieve decrypts PII for an authorized agent, logging the access.
func (v *Vault) Retrieve(ctx context.Context, token, requestingAgentID, purpose string) (string, error) {
	if v.pool == nil || v.aead == nil {
		return "", fmt.Errorf("vault: not configured")
	}

	var ciphertext []byte
	var accessList []string
	err := v.pool.QueryRow(ctx, `
		SELECT ciphertext, access_list
		FROM pii_vault
		WHERE id = $1 AND tenant_id = $2 AND expires_at > now()`,
		token, v.tenantID).Scan(&ciphertext, &accessList)
	if err != nil {
		return "", fmt.Errorf("vault: token not found or expired")
	}

	authorized := false
	for _, allowed := range accessList {
		if allowed == requestingAgentID {
			authorized = true
			break
		}
	}
	if !authorized {
		return "", fmt.Errorf("vault: agent %s not authorized for token %s", requestingAgentID, token)
	}

	if len(ciphertext) < v.aead.NonceSize() {
		return "", fmt.Errorf("vault: ciphertext too short")
	}
	nonce := ciphertext[:v.aead.NonceSize()]
	plaintext, err := v.aead.Open(nil, nonce, ciphertext[v.aead.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("vault: decryption failed")
	}

	// Audit log
	v.pool.Exec(ctx, `
		INSERT INTO pii_access_log (vault_id, agent_id, purpose, accessed_at)
		VALUES ($1, $2, $3, now())`,
		token, requestingAgentID, purpose)

	return string(plaintext), nil
}

// GrantAccess adds an agent to a vault entry's access list.
func (v *Vault) GrantAccess(ctx context.Context, token, agentID string) error {
	_, err := v.pool.Exec(ctx, `
		UPDATE pii_vault SET access_list = array_append(access_list, $1)
		WHERE id = $2 AND tenant_id = $3
		  AND NOT ($1 = ANY(access_list))`,
		agentID, token, v.tenantID)
	return err
}

// Purge removes expired vault entries.
func (v *Vault) Purge(ctx context.Context) (int64, error) {
	tag, err := v.pool.Exec(ctx, `DELETE FROM pii_vault WHERE expires_at < now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func generateToken() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "vlt_" + hex.EncodeToString(b)
}
