package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const pipedreamBaseURL = "https://api.pipedream.com/v1"

type PipedreamClient struct {
	apiKey     string
	projectID  string
	httpClient *http.Client
}

func NewPipedreamClient(apiKey, projectID string) *PipedreamClient {
	return &PipedreamClient{
		apiKey:    apiKey,
		projectID: projectID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type ConnectToken struct {
	Token          string `json:"token"`
	ConnectLinkURL string `json:"connect_link_url"`
	ExpiresAt      int64  `json:"expires_at"`
}

type PipedreamAccount struct {
	ID               string   `json:"id"`
	App              string   `json:"app"`
	Name             string   `json:"name"`
	Healthy          bool     `json:"healthy"`
	AuthorizedScopes []string `json:"authorized_scopes"`
}

type PipedreamResult struct {
	Exports map[string]any `json:"exports"`
	Return  any            `json:"$return_value"`
	Error   string         `json:"error,omitempty"`
}

func (c *PipedreamClient) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", pipedreamBaseURL+"/users/me", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pipedream connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pipedream auth failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *PipedreamClient) CreateConnectToken(
	ctx context.Context,
	externalUserID string,
	app string,
	successRedirect string,
	errorRedirect string,
	webhookURI string,
) (*ConnectToken, error) {
	payload := map[string]any{
		"external_user_id": externalUserID,
		"app":              app,
	}
	if successRedirect != "" {
		payload["success_redirect_uri"] = successRedirect
	}
	if errorRedirect != "" {
		payload["error_redirect_uri"] = errorRedirect
	}
	if webhookURI != "" {
		payload["webhook_uri"] = webhookURI
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", pipedreamBaseURL+"/connect/tokens", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create connect token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pipedream connect token failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result ConnectToken
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode connect token: %w", err)
	}
	return &result, nil
}

func (c *PipedreamClient) ListAccounts(ctx context.Context, externalUserID string) ([]PipedreamAccount, error) {
	url := fmt.Sprintf("%s/connect/users/%s/accounts", pipedreamBaseURL, externalUserID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list accounts failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []PipedreamAccount `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (c *PipedreamClient) DeleteAccount(ctx context.Context, accountID string) error {
	url := fmt.Sprintf("%s/connect/accounts/%s", pipedreamBaseURL, accountID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete account failed (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// DiscoveredAction represents a single action component found via the Pipedream registry.
type DiscoveredAction struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DiscoverActions fetches available action components for an app from Pipedream.
func (c *PipedreamClient) DiscoverActions(ctx context.Context, appSlug string) ([]DiscoveredAction, error) {
	url := fmt.Sprintf("%s/components?app=%s&component_type=action&limit=20", pipedreamBaseURL, appSlug)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover actions for %s: %w", appSlug, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("pipedream components API %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Data []struct {
			Key         string `json:"key"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Version     string `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode components: %w", err)
	}

	var actions []DiscoveredAction
	for _, comp := range result.Data {
		actions = append(actions, DiscoveredAction{
			Key:         comp.Key,
			Name:        comp.Name,
			Description: comp.Description,
		})
	}
	return actions, nil
}

func (c *PipedreamClient) RunAction(
	ctx context.Context,
	externalUserID string,
	actionID string,
	accountID string,
	props map[string]any,
) (*PipedreamResult, error) {
	payload := map[string]any{
		"external_user_id": externalUserID,
		"action_id":        actionID,
		"configured_props": props,
	}
	if accountID != "" {
		payload["account_id"] = accountID
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", pipedreamBaseURL+"/connect/actions/run", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("run action: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("pipedream action failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result PipedreamResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode action result: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("action error: %s", result.Error)
	}
	return &result, nil
}
