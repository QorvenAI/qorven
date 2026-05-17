// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

var providerSlugRegexp = regexp.MustCompile(`^[a-z][a-z0-9\-]{0,61}$`)

// StoreCredentialTool lets an agent store an API credential in the encrypted
// vault so connector binaries can read it at execution time via the
// CONNECTOR_<SLUG>_KEY environment variable injected by AppManager.
type StoreCredentialTool struct {
	keyStore *providers.KeyPoolStore
	tenantID string
}

func NewStoreCredentialTool(keyStore *providers.KeyPoolStore, tenantID string) *StoreCredentialTool {
	return &StoreCredentialTool{keyStore: keyStore, tenantID: tenantID}
}

func (t *StoreCredentialTool) Name() string { return "store_credential" }

func (t *StoreCredentialTool) Description() string {
	return "Store an API credential in the encrypted vault so connectors can use it. " +
		"The key is stored under the given provider name and injected automatically when the connector runs."
}

func (t *StoreCredentialTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider": map[string]any{
				"type":        "string",
				"description": "Provider/connector slug the key belongs to (e.g. 'binance', 'aramex').",
			},
			"key": map[string]any{
				"type":        "string",
				"description": "The API key or secret to store.",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Optional human-readable label for this credential.",
			},
		},
		"required": []string{"provider", "key"},
	}
}

func (t *StoreCredentialTool) Execute(ctx context.Context, args map[string]any) *Result {
	provider, _ := args["provider"].(string)
	key, _ := args["key"].(string)
	label, _ := args["label"].(string)

	if provider == "" {
		return ErrorResult("provider is required")
	}
	if key == "" {
		return ErrorResult("key is required")
	}

	// Normalize provider to lowercase so "Binance" and "binance" resolve to the
	// same manifest slug, which is always lowercase.
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !providerSlugRegexp.MatchString(provider) {
		return ErrorResult("provider must be a slug: lowercase letters, digits, hyphens, starting with a letter (e.g. 'binance', 'my-api')")
	}

	if label == "" {
		label = provider + " API key"
	}

	kr, err := t.keyStore.AddKey(ctx, t.tenantID, provider, label, key)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to store credential: %v", err))
	}
	if err := t.keyStore.VerifyKey(ctx, kr.ID); err != nil {
		// Retire the orphaned unverified row so it doesn't pollute future ListKeys results.
		if retireErr := t.keyStore.RetireKey(ctx, kr.ID); retireErr != nil {
			slog.Warn("store_credential.retire_failed", "id", kr.ID, "provider", provider, "error", retireErr)
		}
		return ErrorResult(fmt.Sprintf("credential stored but verification failed: %v", err))
	}

	slog.Info("store_credential.stored", "provider", provider, "id", kr.ID, "tenant", t.tenantID)

	msg := fmt.Sprintf("Credential stored for provider %q (id: %s). It will be injected automatically when the %s connector runs.", provider, kr.ID, provider)
	return &Result{ForLLM: msg, ForUser: fmt.Sprintf("Credential stored for `%s`", provider)}
}
