// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/qorvenai/qorven/internal/store"
)

// Storage-write preference lives in user_preferences alongside the
// services.* flags. Key: `services.storage_write_enabled`. Default
// false — destructive cloud ops are high-blast-radius and should be
// explicit opt-in, not a default.
//
// Shape: a bare boolean under the key, the same way services.voice
// is stored. Frontend toggle lives under Settings → Services.

// readStorageAllowWrite returns true if the default user's prefs
// enable cloud write operations. Any DB error or missing key returns
// false — we fail closed on storage.
func readStorageAllowWrite(ctx context.Context, db *store.DB, tenantID string) bool {
	if db == nil || db.Pool == nil {
		return false
	}
	var raw json.RawMessage
	err := db.Pool.QueryRow(ctx,
		`SELECT preferences FROM user_preferences WHERE tenant_id = $1 AND user_id = 'default'`,
		tenantID,
	).Scan(&raw)
	if err != nil {
		return false
	}
	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(raw, &prefs); err != nil {
		slog.Warn("storage.prefs.parse_failed", "tenant", tenantID, "error", err)
		return false
	}
	v, ok := prefs["services.storage_write_enabled"]
	if !ok {
		return false
	}
	var b bool
	if err := json.Unmarshal(v, &b); err != nil {
		return false
	}
	return b
}
