// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/agent"
)

// Prompt-injection policy lives in user_preferences alongside the
// other services.* flags. Key:
//
//   services.prompt_guard = "off" | "warn" | "block" | "strict"
//
// Missing key = "off". Malformed value = "off" with a warning log.
// Recommended default for new installs: "block".

func loadPromptGuardPolicy(ctx context.Context, pool *pgxpool.Pool, tenantID string) agent.PromptInjectionPolicy {
	if pool == nil {
		return agent.PromptGuardOff
	}
	var raw json.RawMessage
	err := pool.QueryRow(ctx,
		`SELECT preferences FROM user_preferences WHERE tenant_id = $1 AND user_id = 'default'`,
		tenantID,
	).Scan(&raw)
	if err != nil {
		return agent.PromptGuardOff
	}
	var prefs map[string]json.RawMessage
	if err := json.Unmarshal(raw, &prefs); err != nil {
		slog.Warn("promptguard.prefs.parse_failed", "tenant", tenantID, "error", err)
		return agent.PromptGuardOff
	}
	valRaw, ok := prefs["services.prompt_guard"]
	if !ok {
		return agent.PromptGuardOff
	}
	var val string
	if err := json.Unmarshal(valRaw, &val); err != nil {
		return agent.PromptGuardOff
	}
	switch val {
	case "warn":
		return agent.PromptGuardWarn
	case "block":
		return agent.PromptGuardBlock
	case "strict":
		return agent.PromptGuardStrict
	default:
		return agent.PromptGuardOff
	}
}
