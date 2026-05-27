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
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/vault"
)

// Executor calls connected service APIs using knowledge + vault credentials.
type Executor struct {
	knowledge *KnowledgeStore
	vault     *vault.Vault
	relay     *RelayStore
	guard     *ExportGuard
	tenantID  string
	client    *http.Client
	// RefreshFns maps platform ID → OAuth refresh function
	RefreshFns map[string]func(string) (*vault.CredentialData, *time.Time, error)
}

func NewExecutor(ks *KnowledgeStore, v *vault.Vault, tenantID string) *Executor {
	return &Executor{
		knowledge: ks,
		vault:     v,
		tenantID:  tenantID,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil,
			},
		},
		RefreshFns: make(map[string]func(string) (*vault.CredentialData, *time.Time, error)),
	}
}

func (e *Executor) SetRelay(rs *RelayStore) {
	e.relay = rs
}

func (e *Executor) SetExportGuard(eg *ExportGuard) {
	e.guard = eg
}

// Execute runs a connector action. Returns the API response as string.
func (e *Executor) Execute(ctx context.Context, platformID, actionKey string, params map[string]any) (string, error) {
	action, err := e.knowledge.GetAction(ctx, platformID, actionKey)
	if err != nil {
		return "", fmt.Errorf("unknown action %s.%s", platformID, actionKey)
	}

	platform, err := e.knowledge.GetPlatform(ctx, platformID)
	if err != nil {
		return "", fmt.Errorf("platform %s not found", platformID)
	}

	if e.guard != nil {
		agentID := agentIDFromContext(ctx)
		if err := e.guard.Check(ctx, e.tenantID, agentID, platformID, actionKey, platform.Category, params); err != nil {
			return "", err
		}
	}

	var result string
	var backendUsed string

	if e.shouldUsePipedream(ctx, platformID, action) {
		result, err = e.executePipedream(ctx, platformID, action, params)
		backendUsed = "pipedream"
	} else {
		result, err = e.executeDirect(ctx, platform, action, params)
		backendUsed = "direct"
	}

	if e.relay != nil {
		agentID := agentIDFromContext(ctx)
		go func() {
			_ = e.relay.LogAction(context.Background(), ActionLogEntry{
				TenantID:     e.tenantID,
				AgentID:      agentID,
				PlatformID:   platformID,
				ActionKey:    actionKey,
				BackendUsed:  backendUsed,
				Success:      err == nil,
				ErrorMessage: errStr(err),
			})
		}()
	}

	return result, err
}

func (e *Executor) shouldUsePipedream(ctx context.Context, platformID string, action *ActionDef) bool {
	if action.ExecutionBackend == "pipedream" {
		return true
	}
	refreshFn := e.RefreshFns[platformID]
	_, vaultErr := e.vault.GetToken(ctx, e.tenantID, platformID, refreshFn)
	if vaultErr == nil {
		return false
	}
	if e.relay == nil || action.PipedreamActionID == "" {
		return false
	}
	acc, _ := e.relay.GetAccountForPlatform(ctx, e.tenantID, platformID)
	return acc != nil
}

func (e *Executor) executePipedream(ctx context.Context, platformID string, action *ActionDef, params map[string]any) (string, error) {
	if e.relay == nil {
		return "", fmt.Errorf("no relay configured — add Pipedream API key in Settings → Integrations")
	}
	apiKey, err := e.relay.GetRelayKey(ctx, e.tenantID, "pipedream")
	if err != nil {
		return "", fmt.Errorf("pipedream not configured — add API key in Settings → Integrations")
	}
	acc, err := e.relay.GetAccountForPlatform(ctx, e.tenantID, platformID)
	if err != nil || acc == nil {
		return "", fmt.Errorf("no %s account connected — connect in Settings → Integrations", platformID)
	}

	client := NewPipedreamClient(apiKey, "")
	result, err := client.RunAction(ctx, e.tenantID, action.PipedreamActionID, acc.ExternalAccountID, params)
	if err != nil {
		return "", err
	}

	out, _ := json.Marshal(result.Exports)
	s := string(out)
	if len(s) > 4000 {
		s = s[:4000] + "\n...(truncated)"
	}
	return s, nil
}

func (e *Executor) executeDirect(ctx context.Context, platform *Platform, action *ActionDef, params map[string]any) (string, error) {
	refreshFn := e.RefreshFns[platform.ID]
	token, err := e.vault.GetToken(ctx, e.tenantID, platform.ID, refreshFn)
	if err != nil {
		return "", fmt.Errorf("not connected to %s — connect in Settings → Connections", platform.Name)
	}

	fullURL := strings.TrimRight(platform.BaseURL, "/") + action.Path
	if params == nil {
		params = make(map[string]any)
	}

	for k, v := range params {
		ph := "{" + k + "}"
		if strings.Contains(fullURL, ph) {
			fullURL = strings.ReplaceAll(fullURL, ph, fmt.Sprintf("%v", v))
			delete(params, k)
		}
	}

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
func (e *Executor) ExecuteSafe(ctx context.Context, platformID, actionKey string, params map[string]any) (string, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("connector.execute.panic", "platform", platformID, "action", actionKey, "panic", r)
		}
	}()
	return e.Execute(ctx, platformID, actionKey, params)
}

func agentIDFromContext(ctx context.Context) string {
	if v := ctx.Value("agent_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
