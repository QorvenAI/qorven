// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/tools"
)

// SQL connections are configured via Settings → Connections → Database
// and stored in user_preferences under the key `services.sql_connections`.
//
// Shape:
//   services.sql_connections = [
//     {
//       "name": "prod",
//       "driver": "pgx",
//       "dsn_encrypted": "<base64>",    ← encrypted at rest
//       "description": "orders + users",
//       "read_only": true,
//       "timeout_seconds": 30
//     },
//     ...
//   ]
//
// Each DSN is encrypted with the gateway's encryption key so secrets
// never land in plaintext JSON on disk. The user types the DSN once
// in the UI; the backend encrypts + stores.

type sqlConnectionPref struct {
	Name           string `json:"name"`
	Driver         string `json:"driver"`
	DSNEncrypted   string `json:"dsn_encrypted"` // base64 of encrypted bytes
	Description    string `json:"description"`
	ReadOnly       bool   `json:"read_only"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// loadSQLConnections reads the pref, decrypts each DSN, and registers
// every entry. Errors on individual connections are logged and
// skipped — a single bad DSN shouldn't prevent the gateway from
// booting or other DBs from being available.
func loadSQLConnections(
	ctx context.Context,
	db *store.DB,
	tenantID string,
	encryptionKey string,
	registry *tools.SQLConnectionRegistry,
) {
	if db == nil || db.Pool == nil {
		return
	}
	var raw json.RawMessage
	err := db.Pool.QueryRow(ctx,
		`SELECT preferences FROM user_preferences WHERE tenant_id = $1 AND user_id = 'default'`,
		tenantID,
	).Scan(&raw)
	if err != nil {
		// No prefs = no connections. Not an error.
		return
	}
	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(raw, &prefs); err != nil {
		slog.Warn("sql_connections.prefs.parse_failed", "tenant", tenantID, "error", err)
		return
	}
	connsRaw, ok := prefs["services.sql_connections"]
	if !ok {
		return
	}
	var entries []sqlConnectionPref
	if err := json.Unmarshal(connsRaw, &entries); err != nil {
		slog.Warn("sql_connections.decode_failed", "tenant", tenantID, "error", err)
		return
	}

	for _, e := range entries {
		if e.Name == "" || e.DSNEncrypted == "" {
			slog.Warn("sql_connections.skip_invalid", "name", e.Name)
			continue
		}
		// Decrypt via the same path provider credentials use. If the
		// key is wrong or the payload is corrupt, log and skip — we
		// don't want a single bad entry to take out the whole
		// registry.
		plain, err := providers.DecryptKeyBytes([]byte(e.DSNEncrypted), encryptionKey)
		if err != nil {
			slog.Warn("sql_connections.decrypt_failed", "name", e.Name, "error", err)
			continue
		}
		driver := e.Driver
		if driver == "" {
			driver = "pgx"
		}
		if err := registry.Register(tools.SQLConnection{
			Name:        e.Name,
			Driver:      driver,
			DSN:         string(plain),
			Description: e.Description,
			ReadOnly:    e.ReadOnly,
		}); err != nil {
			slog.Warn("sql_connections.register_failed", "name", e.Name, "error", err)
			continue
		}
		slog.Info("sql_connections.registered",
			"name", e.Name, "driver", driver, "read_only", e.ReadOnly)
	}
}
