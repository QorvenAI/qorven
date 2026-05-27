package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

var ErrRelayNotConfigured = errors.New("no relay provider configured")

type RelayStore struct {
	pool   *pgxpool.Pool
	encKey string
}

func NewRelayStore(pool *pgxpool.Pool, encKey string) *RelayStore {
	return &RelayStore{pool: pool, encKey: encKey}
}

// ── Relay API Key (stored in provider_configs, provider_type='pipedream') ───

func (s *RelayStore) SaveRelayKey(ctx context.Context, tenantID, provider, apiKey string) error {
	encrypted, err := crypto.Encrypt([]byte(apiKey), s.encKey)
	if err != nil {
		return fmt.Errorf("encrypt relay key: %w", err)
	}
	cfg, _ := json.Marshal(map[string]string{"api_key_enc": string(encrypted)})
	_, err = s.pool.Exec(ctx,
		`INSERT INTO provider_configs (tenant_id, provider_type, config)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, provider_type) DO UPDATE SET config = $3, updated_at = now()`,
		tenantID, provider, cfg)
	return err
}

func (s *RelayStore) GetRelayKey(ctx context.Context, tenantID, provider string) (string, error) {
	var cfgRaw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT config FROM provider_configs WHERE tenant_id = $1 AND provider_type = $2`,
		tenantID, provider).Scan(&cfgRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrRelayNotConfigured
		}
		return "", err
	}

	var cfg struct {
		APIKeyEnc string `json:"api_key_enc"`
	}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil || cfg.APIKeyEnc == "" {
		return "", ErrRelayNotConfigured
	}

	decrypted, err := crypto.Decrypt([]byte(cfg.APIKeyEnc), s.encKey)
	if err != nil {
		return "", fmt.Errorf("decrypt relay key: %w", err)
	}
	return string(decrypted), nil
}

func (s *RelayStore) DeleteRelayKey(ctx context.Context, tenantID, provider string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM provider_configs WHERE tenant_id = $1 AND provider_type = $2`,
		tenantID, provider)
	return err
}

func (s *RelayStore) HasRelay(ctx context.Context, tenantID string) (bool, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM provider_configs WHERE tenant_id = $1 AND provider_type = 'pipedream'`,
		tenantID).Scan(&count)
	return count > 0, err
}

// ── Connected Accounts ──────────────────────────────────────────────────────

type ConnectedAccountRecord struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	RelayProvider     string    `json:"relay_provider"`
	ExternalAccountID string    `json:"external_account_id"`
	PlatformID        string    `json:"platform_id"`
	DisplayName       string    `json:"display_name"`
	AuthorizedScopes  []string  `json:"authorized_scopes"`
	Healthy           bool      `json:"healthy"`
	ConnectedAt       time.Time `json:"connected_at"`
}

func (s *RelayStore) UpsertAccount(ctx context.Context, tenantID string, acc ConnectedAccountRecord) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO connected_accounts (tenant_id, relay_provider, external_account_id, platform_id, display_name, authorized_scopes, healthy)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (tenant_id, external_account_id) DO UPDATE SET
		   platform_id = $4, display_name = $5, authorized_scopes = $6, healthy = $7, last_checked_at = now()`,
		tenantID, acc.RelayProvider, acc.ExternalAccountID, acc.PlatformID, acc.DisplayName, acc.AuthorizedScopes, acc.Healthy)
	return err
}

func (s *RelayStore) ListAccounts(ctx context.Context, tenantID string) ([]ConnectedAccountRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, relay_provider, external_account_id, platform_id, display_name, authorized_scopes, healthy, connected_at
		 FROM connected_accounts WHERE tenant_id = $1 ORDER BY connected_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConnectedAccountRecord
	for rows.Next() {
		var a ConnectedAccountRecord
		if err := rows.Scan(&a.ID, &a.TenantID, &a.RelayProvider, &a.ExternalAccountID, &a.PlatformID, &a.DisplayName, &a.AuthorizedScopes, &a.Healthy, &a.ConnectedAt); err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *RelayStore) GetAccountForPlatform(ctx context.Context, tenantID, platformID string) (*ConnectedAccountRecord, error) {
	var a ConnectedAccountRecord
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, relay_provider, external_account_id, platform_id, display_name, authorized_scopes, healthy, connected_at
		 FROM connected_accounts WHERE tenant_id = $1 AND platform_id = $2 AND healthy = true
		 ORDER BY connected_at DESC LIMIT 1`, tenantID, platformID,
	).Scan(&a.ID, &a.TenantID, &a.RelayProvider, &a.ExternalAccountID, &a.PlatformID, &a.DisplayName, &a.AuthorizedScopes, &a.Healthy, &a.ConnectedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (s *RelayStore) DeleteAccount(ctx context.Context, tenantID, accountID string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM connected_accounts WHERE tenant_id = $1 AND id = $2::uuid`,
		tenantID, accountID)
	return err
}

// ── Permissions ─────────────────────────────────────────────────────────────

func (s *RelayStore) CheckPermission(ctx context.Context, tenantID, agentID, platformID, actionKey string) (bool, error) {
	if agentID == "" {
		return true, nil
	}
	var allowed bool
	err := s.pool.QueryRow(ctx,
		`SELECT allowed FROM integration_permissions
		 WHERE tenant_id = $1 AND agent_id = $2::uuid
		   AND (platform_id IS NULL OR platform_id = $3)
		   AND (action_key IS NULL OR action_key = $4)
		 ORDER BY
		   CASE WHEN platform_id IS NOT NULL AND action_key IS NOT NULL THEN 1
		        WHEN platform_id IS NOT NULL THEN 2
		        ELSE 3 END
		 LIMIT 1`,
		tenantID, agentID, platformID, actionKey).Scan(&allowed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return allowed, nil
}

// ── Audit Log ───────────────────────────────────────────────────────────────

type ActionLogEntry struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id,omitempty"`
	AgentID      string    `json:"agent_id,omitempty"`
	SessionID    string    `json:"session_id,omitempty"`
	PlatformID   string    `json:"platform_id"`
	ActionKey    string    `json:"action_key"`
	BackendUsed  string    `json:"backend_used"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	ExecutionID  string    `json:"execution_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *RelayStore) LogAction(ctx context.Context, entry ActionLogEntry) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO integration_action_log (tenant_id, agent_id, session_id, platform_id, action_key, backend_used, success, error_message, execution_id)
		 VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9)`,
		entry.TenantID, entry.AgentID, entry.SessionID, entry.PlatformID, entry.ActionKey, entry.BackendUsed, entry.Success, entry.ErrorMessage, entry.ExecutionID)
	return err
}

func (s *RelayStore) ListLog(ctx context.Context, tenantID string, limit int) ([]ActionLogEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, COALESCE(agent_id::text, ''), COALESCE(session_id::text, ''), platform_id, action_key, backend_used, success, COALESCE(error_message, ''), COALESCE(execution_id, ''), created_at
		 FROM integration_action_log WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActionLogEntry
	for rows.Next() {
		var e ActionLogEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.AgentID, &e.SessionID, &e.PlatformID, &e.ActionKey, &e.BackendUsed, &e.Success, &e.ErrorMessage, &e.ExecutionID, &e.CreatedAt); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
