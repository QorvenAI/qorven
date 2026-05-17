// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/crypto"
)

// ProviderRow is one configured voice provider (one row in
// voice_providers). A single driver + credentials + settings blob. The
// gateway materialises this into a concrete TTSProvider / STTProvider
// at boot by calling BuildProvider in this package.
type ProviderRow struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id,omitempty"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind"`     // tts | stt | realtime
	Driver    string          `json:"driver"`   // catalog id
	APIBase   string          `json:"api_base"`
	APIKey    string          `json:"-"`        // never marshaled; decrypted on read
	Settings  json.RawMessage `json:"settings"`
	Enabled   bool            `json:"enabled"`
	IsDefault bool            `json:"is_default"`
}

// Store handles CRUD for voice_providers with encrypted API keys. The
// shape mirrors providers.Store so the gateway can apply the same
// load-at-boot pattern.
type Store struct {
	pool   *pgxpool.Pool
	encKey string
}

func NewStore(pool *pgxpool.Pool, encryptionKey string) *Store {
	if encryptionKey == "" {
		slog.Warn("voice store: API key encryption DISABLED — keys stored in plain text")
	}
	return &Store{pool: pool, encKey: encryptionKey}
}

func (s *Store) encrypt(plaintext string) (string, error) {
	if plaintext == "" || s.encKey == "" { return plaintext, nil }
	return crypto.EncryptString(plaintext, s.encKey)
}

func (s *Store) decrypt(ciphertext string) string {
	if ciphertext == "" || s.encKey == "" { return ciphertext }
	out, err := crypto.DecryptString(ciphertext, s.encKey)
	if err != nil { return "" }
	return out
}

// List returns every voice provider row for a tenant, ordered by kind
// then name. Used by the Settings UI and by loadVoiceProvidersFromDB.
func (s *Store) List(ctx context.Context, tenantID string) ([]ProviderRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, kind, driver, api_base, api_key, settings, enabled, is_default
		   FROM voice_providers
		  WHERE tenant_id = $1
		  ORDER BY kind, name`, tenantID)
	if err != nil { return nil, fmt.Errorf("voice.store.list: %w", err) }
	defer rows.Close()
	out := []ProviderRow{}
	for rows.Next() {
		var r ProviderRow
		var apiKey string
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &r.Driver, &r.APIBase,
			&apiKey, &r.Settings, &r.Enabled, &r.IsDefault); err != nil {
			return nil, err
		}
		r.APIKey = s.decrypt(apiKey)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get fetches a single provider by id within the tenant. APIKey is
// decrypted so the caller can instantiate the driver.
func (s *Store) Get(ctx context.Context, tenantID, id string) (ProviderRow, error) {
	var r ProviderRow
	var apiKey string
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, kind, driver, api_base, api_key, settings, enabled, is_default
		   FROM voice_providers
		  WHERE tenant_id = $1 AND id = $2`, tenantID, id,
	).Scan(&r.ID, &r.Name, &r.Kind, &r.Driver, &r.APIBase, &apiKey, &r.Settings, &r.Enabled, &r.IsDefault)
	if err != nil { return r, fmt.Errorf("voice.store.get: %w", err) }
	r.APIKey = s.decrypt(apiKey)
	r.TenantID = tenantID
	return r, nil
}

// Create inserts a row. Returns the new row including generated id.
// Settings is required — empty JSONB '{}' is safe.
func (s *Store) Create(ctx context.Context, tenantID string, r ProviderRow) (ProviderRow, error) {
	if r.Kind != "tts" && r.Kind != "stt" && r.Kind != "realtime" {
		return r, fmt.Errorf("voice.store.create: kind must be tts|stt|realtime, got %q", r.Kind)
	}
	if r.Name == "" || r.Driver == "" {
		return r, fmt.Errorf("voice.store.create: name and driver required")
	}
	enc, err := s.encrypt(r.APIKey)
	if err != nil { return r, fmt.Errorf("voice.store.create.encrypt: %w", err) }
	if r.Settings == nil { r.Settings = json.RawMessage("{}") }

	err = s.pool.QueryRow(ctx,
		`INSERT INTO voice_providers (tenant_id, name, kind, driver, api_base, api_key, settings, enabled, is_default)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 RETURNING id`,
		tenantID, r.Name, r.Kind, r.Driver, r.APIBase, enc, r.Settings, r.Enabled, r.IsDefault,
	).Scan(&r.ID)
	if err != nil { return r, fmt.Errorf("voice.store.create: %w", err) }

	// If this row was flagged default, demote siblings in the same kind.
	if r.IsDefault {
		if err := s.setDefaultTx(ctx, tenantID, r.Kind, r.ID); err != nil {
			return r, err
		}
	}
	return r, nil
}

// Update modifies a row. APIKey="" keeps the existing encrypted value
// — so the UI can edit settings without forcing the user to re-paste
// their API key every time.
func (s *Store) Update(ctx context.Context, tenantID, id string, r ProviderRow) error {
	if r.APIKey == "" {
		// Keep existing API key.
		_, err := s.pool.Exec(ctx,
			`UPDATE voice_providers
			    SET name=$1, kind=$2, driver=$3, api_base=$4, settings=$5, enabled=$6, is_default=$7, updated_at=now()
			  WHERE tenant_id=$8 AND id=$9`,
			r.Name, r.Kind, r.Driver, r.APIBase, r.Settings, r.Enabled, r.IsDefault, tenantID, id)
		if err != nil { return fmt.Errorf("voice.store.update: %w", err) }
	} else {
		enc, err := s.encrypt(r.APIKey)
		if err != nil { return fmt.Errorf("voice.store.update.encrypt: %w", err) }
		_, err = s.pool.Exec(ctx,
			`UPDATE voice_providers
			    SET name=$1, kind=$2, driver=$3, api_base=$4, api_key=$5, settings=$6, enabled=$7, is_default=$8, updated_at=now()
			  WHERE tenant_id=$9 AND id=$10`,
			r.Name, r.Kind, r.Driver, r.APIBase, enc, r.Settings, r.Enabled, r.IsDefault, tenantID, id)
		if err != nil { return fmt.Errorf("voice.store.update: %w", err) }
	}
	if r.IsDefault {
		return s.setDefaultTx(ctx, tenantID, r.Kind, id)
	}
	return nil
}

// Delete removes a row. If it was the default for its kind, no
// automatic promotion — the UI surfaces that and asks the user to
// pick a new default.
func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM voice_providers WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil { return fmt.Errorf("voice.store.delete: %w", err) }
	return nil
}

// SetDefault marks one provider as the default for its kind within the
// tenant, demoting any sibling that was previously default. This is
// the single-transaction flip-and-set that avoids a window where
// nobody is default.
func (s *Store) SetDefault(ctx context.Context, tenantID, id string) error {
	// Fetch kind first so we can demote the right siblings.
	var kind string
	err := s.pool.QueryRow(ctx,
		`SELECT kind FROM voice_providers WHERE tenant_id=$1 AND id=$2`, tenantID, id,
	).Scan(&kind)
	if err != nil { return fmt.Errorf("voice.store.set_default.lookup: %w", err) }
	return s.setDefaultTx(ctx, tenantID, kind, id)
}

func (s *Store) setDefaultTx(ctx context.Context, tenantID, kind, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil { return fmt.Errorf("voice.store.set_default.begin: %w", err) }
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE voice_providers SET is_default=false, updated_at=now()
		   WHERE tenant_id=$1 AND kind=$2 AND id<>$3`, tenantID, kind, id); err != nil {
		return fmt.Errorf("voice.store.set_default.demote: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE voice_providers SET is_default=true, updated_at=now()
		   WHERE tenant_id=$1 AND id=$2`, tenantID, id); err != nil {
		return fmt.Errorf("voice.store.set_default.promote: %w", err)
	}
	return tx.Commit(ctx)
}
