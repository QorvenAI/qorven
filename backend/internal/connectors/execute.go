// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"log/slog"

	"github.com/qorvenai/qorven/internal/vault"
)

// Executor calls connected service APIs using knowledge + vault credentials.
type Executor struct {
	knowledge *KnowledgeStore
	vault     *vault.Vault
	tenantID  string
	client    *http.Client
	// RefreshFns maps platform ID → OAuth refresh function
	RefreshFns map[string]func(string) (*vault.CredentialData, *time.Time, error)
}

func NewExecutor(ks *KnowledgeStore, v *vault.Vault, tenantID string) *Executor {
	return &Executor{
		knowledge:  ks,
		vault:      v,
		tenantID:   tenantID,
		client:     &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil, // Ignore HTTP_PROXY/HTTPS_PROXY — defense against env injection
			},
		},
		RefreshFns: make(map[string]func(string) (*vault.CredentialData, *time.Time, error)),
	}
}

// Execute runs a connector action. Returns the API response as string.
func (e *Executor) Execute(ctx context.Context, platformID, actionKey string, params map[string]any) (string, error) {
	// 1. Look up action
	action, err := e.knowledge.GetAction(ctx, platformID, actionKey)
	if err != nil {
		return "", fmt.Errorf("unknown action %s.%s", platformID, actionKey)
	}

	// 2. Look up platform
	platform, err := e.knowledge.GetPlatform(ctx, platformID)
	if err != nil {
		return "", fmt.Errorf("platform %s not found", platformID)
	}

	// 3. Get credential
	refreshFn := e.RefreshFns[platformID]
	token, err := e.vault.GetToken(ctx, e.tenantID, platformID, refreshFn)
	if err != nil {
		return "", fmt.Errorf("not connected to %s — connect in Settings → Connections", platform.Name)
	}

	// 4. Build request
	fullURL := strings.TrimRight(platform.BaseURL, "/") + action.Path
	if params == nil {
		params = make(map[string]any)
	}

	// Substitute path params: /users/{user_id} → /users/me
	for k, v := range params {
		ph := "{" + k + "}"
		if strings.Contains(fullURL, ph) {
			fullURL = strings.ReplaceAll(fullURL, ph, fmt.Sprintf("%v", v))
			delete(params, k)
		}
	}

	// Query params for GET
	if action.Method == "GET" && len(params) > 0 {
		parts := make([]string, 0, len(params))
		for k, v := range params {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		sep := "?"
		if strings.Contains(fullURL, "?") {
			sep = "&"
		}
		fullURL += sep + strings.Join(parts, "&")
	}

	var bodyReader io.Reader
	if action.Method != "GET" && action.Method != "DELETE" && len(params) > 0 {
		b, _ := json.Marshal(params)
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, action.Method, fullURL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}

	// Auth header
	switch platform.AuthType {
	case "oauth2", "bearer":
		req.Header.Set("Authorization", "Bearer "+token)
	case "api_key":
		var cfg struct {
			HeaderName string `json:"header_name"`
			Prefix     string `json:"prefix"`
		}
		json.Unmarshal(platform.AuthConfig, &cfg)
		if cfg.HeaderName != "" {
			req.Header.Set(cfg.HeaderName, cfg.Prefix+token)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	case "basic":
		req.Header.Set("Authorization", "Basic "+token)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	var extra map[string]string
	json.Unmarshal(action.Headers, &extra)
	for k, v := range extra {
		req.Header.Set(k, v)
	}

	// 5. Execute
	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s request failed: %w", platform.Name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		msg := string(body)
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return "", fmt.Errorf("%s API %d: %s", platform.Name, resp.StatusCode, msg)
	}

	result := string(body)
	if len(result) > 4000 {
		result = result[:4000] + "\n...(truncated)"
	}
	return result, nil
}

// ExecuteSafe wraps Execute with per-action error recovery.
// Prevents one failed action from cascading to others in a pipeline.
func (e *Executor) ExecuteSafe(ctx context.Context, platformID, actionKey string, params map[string]any) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("connector.execute.panic", "platform", platformID, "action", actionKey, "panic", r)
		}
	}()
	return e.Execute(ctx, platformID, actionKey, params)
}
