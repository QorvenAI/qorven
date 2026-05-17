// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/pii"
)

// PII redaction preference lives alongside the other services.* flags
// in user_preferences.preferences. Two keys:
//
//   "services.pii_redaction"        bool  — master on/off switch
//   "services.pii_redaction.kinds"  {email, phone, ssn, credit_card,
//                                     iban, ipv4, ipv6 → bool}
//
// Semantics:
//   - missing / false master            → redaction off
//   - true master + missing kinds blob  → redact everything (pii.All)
//   - true master + any kind=true       → redact those specific kinds
//   - true master + all kinds=false     → redaction off (user fiddled
//                                          without committing)
//
// Stored in user_preferences (not system_configs) to match the
// existing services.voice / services.web_search pattern the
// frontend toggles via usePrefs().

// loadPIIKinds reads the PII redaction preference for the default
// user in the given tenant. Returns the enabled kinds bitmask and a
// bool indicating whether the redactor should be activated.
//
// Silent-failure shape: any DB error or malformed JSON returns
// (0, false) so a broken pref row never prevents startup.
func loadPIIKinds(ctx context.Context, pool *pgxpool.Pool, tenantID string) (pii.Kind, bool) {
	var raw json.RawMessage
	err := pool.QueryRow(ctx,
		`SELECT preferences FROM user_preferences WHERE tenant_id = $1 AND user_id = 'default'`,
		tenantID,
	).Scan(&raw)
	if err != nil {
		// No prefs row = defaults = disabled.
		return 0, false
	}

	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(raw, &prefs); err != nil {
		slog.Warn("pii.prefs.parse_failed", "tenant", tenantID, "error", err)
		return 0, false
	}

	masterRaw, ok := prefs["services.pii_redaction"]
	if !ok {
		return 0, false
	}
	var master bool
	if err := json.Unmarshal(masterRaw, &master); err != nil || !master {
		return 0, false
	}

	kindsRaw, hasKinds := prefs["services.pii_redaction.kinds"]
	if !hasKinds {
		// Master on, no per-kind config = redact everything.
		return pii.All, true
	}
	var kinds map[string]bool
	if err := json.Unmarshal(kindsRaw, &kinds); err != nil {
		slog.Warn("pii.kinds.parse_failed", "tenant", tenantID, "error", err)
		return pii.All, true
	}

	var k pii.Kind
	if kinds["email"] {
		k |= pii.KindEmail
	}
	if kinds["phone"] {
		k |= pii.KindPhone
	}
	if kinds["ssn"] {
		k |= pii.KindSSN
	}
	if kinds["credit_card"] {
		k |= pii.KindCreditCard
	}
	if kinds["iban"] {
		k |= pii.KindIBAN
	}
	if kinds["ipv4"] {
		k |= pii.KindIPv4
	}
	if kinds["ipv6"] {
		k |= pii.KindIPv6
	}
	return k, k != 0
}
