// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// Store handles provider CRUD in PostgreSQL with encrypted API keys.
type Store struct {
	pool      *pgxpool.Pool
	encKey    string // hex-encoded AES-256 key
}

func NewStore(pool *pgxpool.Pool, encryptionKey string) *Store {
	if encryptionKey != "" {
		slog.Info("provider store: API key encryption enabled")
	} else {
		slog.Warn("provider store: API key encryption DISABLED — keys stored in plain text")
	}
	return &Store{pool: pool, encKey: encryptionKey}
}

func (s *Store) Create(ctx context.Context, tenantID string, cfg ProviderConfig) (ProviderConfig, error) {
	encKey, err := s.encryptKey(cfg.APIKey)
	if err != nil {
		return cfg, fmt.Errorf("encrypt: %w", err)
	}

	settings := mergeAWSCredsIntoSettings(cfg)
	capsJSON, _ := json.Marshal(cfg.Capabilities)

	err = s.pool.QueryRow(ctx,
		`INSERT INTO providers (tenant_id, name, display_name, provider_type, api_base, api_key, enabled, settings, capabilities)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (tenant_id, name) DO UPDATE
		   SET display_name = EXCLUDED.display_name,
		       provider_type = EXCLUDED.provider_type,
		       api_base = EXCLUDED.api_base,
		       api_key = EXCLUDED.api_key,
		       enabled = EXCLUDED.enabled,
		       settings = EXCLUDED.settings,
		       capabilities = EXCLUDED.capabilities,
		       updated_at = now()
		 RETURNING id`,
		tenantID, cfg.Name, cfg.DisplayName, cfg.ProviderType, cfg.APIBase, encKey, cfg.Enabled, settings, capsJSON,
	).Scan(&cfg.ID)
	if err != nil {
		return cfg, fmt.Errorf("insert provider: %w", err)
	}
	cfg.APIKey = "" // never return raw key
	return cfg, nil
}

// mergeAWSCredsIntoSettings folds AWS static credentials into the settings JSONB blob.
// This avoids schema changes — credentials live in the flexible settings column.
func mergeAWSCredsIntoSettings(cfg ProviderConfig) json.RawMessage {
	base := map[string]any{}
	if len(cfg.Settings) > 0 {
		_ = json.Unmarshal(cfg.Settings, &base)
	}
	if cfg.AWSAccessKey != "" {
		base["aws_access_key"] = cfg.AWSAccessKey
		base["aws_secret_key"] = cfg.AWSSecretKey
		if cfg.AWSSessionToken != "" {
			base["aws_session_token"] = cfg.AWSSessionToken
		}
	}
	b, _ := json.Marshal(base)
	return b
}

// extractAWSCredsFromSettings reads AWS credentials back out of the settings JSONB.
func extractAWSCredsFromSettings(cfg *ProviderConfig) {
	if len(cfg.Settings) == 0 { return }
	var s struct {
		AccessKey    string `json:"aws_access_key"`
		SecretKey    string `json:"aws_secret_key"`
		SessionToken string `json:"aws_session_token"`
	}
	if json.Unmarshal(cfg.Settings, &s) == nil {
		cfg.AWSAccessKey = s.AccessKey
		cfg.AWSSecretKey = s.SecretKey
		cfg.AWSSessionToken = s.SessionToken
	}
}

func (s *Store) Get(ctx context.Context, tenantID, id string) (ProviderConfig, error) {
	var cfg ProviderConfig
	encKey := []byte{}
	capsJSON := []byte{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, display_name, provider_type, api_base, api_key, enabled, settings, capabilities
		 FROM providers WHERE tenant_id = $1 AND id = $2`,
		tenantID, id,
	).Scan(&cfg.ID, &cfg.Name, &cfg.DisplayName, &cfg.ProviderType, &cfg.APIBase, &encKey, &cfg.Enabled, &cfg.Settings, &capsJSON)
	if err != nil {
		return cfg, fmt.Errorf("get provider: %w", err)
	}
	cfg.APIKey, _ = s.decryptKey(encKey)
	extractAWSCredsFromSettings(&cfg)
	if len(capsJSON) > 0 { json.Unmarshal(capsJSON, &cfg.Capabilities) }
	return cfg, nil
}

func (s *Store) List(ctx context.Context, tenantID string) ([]ProviderConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, display_name, provider_type, api_base, enabled, settings, capabilities
		 FROM providers WHERE tenant_id = $1 ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	out := []ProviderConfig{}
	for rows.Next() {
		var cfg ProviderConfig
		capsJSON := []byte{}
		if err := rows.Scan(&cfg.ID, &cfg.Name, &cfg.DisplayName, &cfg.ProviderType, &cfg.APIBase, &cfg.Enabled, &cfg.Settings, &capsJSON); err != nil {
			return nil, err
		}
		if len(capsJSON) > 0 { json.Unmarshal(capsJSON, &cfg.Capabilities) }
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (s *Store) Update(ctx context.Context, tenantID, id string, cfg ProviderConfig) error {
	settings := mergeAWSCredsIntoSettings(cfg)
	capsJSON, _ := json.Marshal(cfg.Capabilities)
	// If API key provided, encrypt it; otherwise keep existing
	if cfg.APIKey != "" {
		encKey, err := s.encryptKey(cfg.APIKey)
		if err != nil {
			return fmt.Errorf("encrypt: %w", err)
		}
		_, err = s.pool.Exec(ctx,
			`UPDATE providers SET name=$3, display_name=$4, provider_type=$5, api_base=$6, api_key=$7, enabled=$8, settings=$9, capabilities=$10, updated_at=NOW()
			 WHERE tenant_id=$1 AND id=$2`,
			tenantID, id, cfg.Name, cfg.DisplayName, cfg.ProviderType, cfg.APIBase, encKey, cfg.Enabled, settings, capsJSON,
		)
		return err
	}

	_, err := s.pool.Exec(ctx,
		`UPDATE providers SET name=$3, display_name=$4, provider_type=$5, api_base=$6, enabled=$7, settings=$8, capabilities=$9, updated_at=NOW()
		 WHERE tenant_id=$1 AND id=$2`,
		tenantID, id, cfg.Name, cfg.DisplayName, cfg.ProviderType, cfg.APIBase, cfg.Enabled, settings, capsJSON,
	)
	return err
}

func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM providers WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	return err
}

// ListWithKeys returns configs with decrypted keys (for registry loading on boot).
func (s *Store) ListWithKeys(ctx context.Context, tenantID string) ([]ProviderConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, display_name, provider_type, api_base, api_key, enabled, settings, capabilities
		 FROM providers WHERE tenant_id = $1 AND enabled = true ORDER BY created_at`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ProviderConfig{}
	for rows.Next() {
		var cfg ProviderConfig
		var encKey, capsJSON []byte
		if err := rows.Scan(&cfg.ID, &cfg.Name, &cfg.DisplayName, &cfg.ProviderType, &cfg.APIBase, &encKey, &cfg.Enabled, &cfg.Settings, &capsJSON); err != nil {
			return nil, err
		}
		cfg.APIKey, _ = s.decryptKey(encKey)
		extractAWSCredsFromSettings(&cfg)
		if len(capsJSON) > 0 { json.Unmarshal(capsJSON, &cfg.Capabilities) }
		out = append(out, cfg)
	}
	return out, rows.Err()
}

// UpdateCapabilities sets capability flags for a provider.
func (s *Store) UpdateCapabilities(ctx context.Context, tenantID, id string, caps ProviderCapabilityFlags) error {
	capsJSON, _ := json.Marshal(caps)
	_, err := s.pool.Exec(ctx,
		`UPDATE providers SET capabilities = $1, updated_at = NOW() WHERE tenant_id = $2 AND id = $3`,
		capsJSON, tenantID, id,
	)
	return err
}

func (s *Store) encryptKey(key string) ([]byte, error) {
	if key == "" {
		return nil, nil
	}
	if s.encKey == "" {
		return []byte(key), nil // no encryption key configured — store plaintext
	}
	return crypto.Encrypt([]byte(key), s.encKey)
}

func (s *Store) decryptKey(enc []byte) (string, error) {
	if len(enc) == 0 {
		return "", nil
	}
	if s.encKey == "" {
		return string(enc), nil // no encryption key — stored as plaintext
	}
	b, err := crypto.Decrypt(enc, s.encKey)
	if err != nil {
		slog.Warn("failed to decrypt provider API key", "error", err)
		return string(enc), nil // fallback to raw value
	}
	return string(b), nil
}

// GetByName returns a provider by name within a tenant.
func (s *Store) GetByName(ctx context.Context, tenantID, name string) (ProviderConfig, error) {
	var p ProviderConfig
	encKey := []byte{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, provider_type, COALESCE(api_base,''), api_key, enabled
		 FROM providers WHERE tenant_id = $1 AND name = $2`, tenantID, name,
	).Scan(&p.ID, &p.Name, &p.ProviderType, &p.APIBase, &encKey, &p.Enabled)
	if err != nil {
		return p, err
	}
	p.APIKey, _ = s.decryptKey(encKey)
	return p, nil
}

// ListAllUnscoped returns all providers across all tenants (admin only).
func (s *Store) ListAllUnscoped(ctx context.Context) ([]ProviderConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, provider_type, COALESCE(api_base,''), api_key, enabled
		 FROM providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	providers := []ProviderConfig{}
	for rows.Next() {
		var p ProviderConfig
		encKey := []byte{}
		rows.Scan(&p.ID, &p.Name, &p.ProviderType, &p.APIBase, &encKey, &p.Enabled)
		p.APIKey, _ = s.decryptKey(encKey)
		providers = append(providers, p)
	}
	return providers, nil
}
