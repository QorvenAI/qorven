package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OutstandClient implements RelayClient using the Outstand relay API.
type OutstandClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewOutstandClient creates a new Outstand relay client.
func NewOutstandClient(apiKey string) *OutstandClient {
	return &OutstandClient{
		apiKey:  apiKey,
		baseURL: "https://api.outstand.so/v1",
		http:    &http.Client{},
	}
}

func (c *OutstandClient) doRequest(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// TestConnection verifies the API key is valid by calling GET /v1/usage.
func (c *OutstandClient) TestConnection(ctx context.Context) error {
	_, status, err := c.doRequest(ctx, http.MethodGet, "/usage", nil)
	if err != nil {
		return fmt.Errorf("outstand test connection: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("outstand test connection: unexpected status %d", status)
	}
	return nil
}

// GetAuthURL requests an OAuth authorization URL for the given platform.
func (c *OutstandClient) GetAuthURL(ctx context.Context, platform, tenantID, redirectURL string) (string, error) {
	payload := map[string]string{
		"tenant_id":    tenantID,
		"redirect_uri": redirectURL,
	}

	body, status, err := c.doRequest(ctx, http.MethodPost, "/social-networks/"+platform+"/auth-url", payload)
	if err != nil {
		return "", fmt.Errorf("outstand get auth url: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("outstand get auth url: unexpected status %d: %s", status, string(body))
	}

	var result struct {
		AuthURL string `json:"auth_url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("outstand get auth url: decode response: %w", err)
	}

	return result.AuthURL, nil
}

// FinalizeConnection completes the OAuth flow by selecting the first available page.
func (c *OutstandClient) FinalizeConnection(ctx context.Context, sessionToken string) (*RelayAccount, error) {
	// First, get pending pages.
	body, status, err := c.doRequest(ctx, http.MethodGet, "/social-accounts/pending/"+sessionToken, nil)
	if err != nil {
		return nil, fmt.Errorf("outstand finalize: get pending: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("outstand finalize: get pending: unexpected status %d: %s", status, string(body))
	}

	var pages []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &pages); err != nil {
		return nil, fmt.Errorf("outstand finalize: decode pending pages: %w", err)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("outstand finalize: no pages available")
	}

	// Finalize with the first page.
	finalizePayload := map[string][]string{
		"selectedPageIds": {pages[0].ID},
	}

	body, status, err = c.doRequest(ctx, http.MethodPost, "/social-accounts/pending/"+sessionToken+"/finalize", finalizePayload)
	if err != nil {
		return nil, fmt.Errorf("outstand finalize: %w", err)
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, fmt.Errorf("outstand finalize: unexpected status %d: %s", status, string(body))
	}

	var account struct {
		ID          string `json:"id"`
		Platform    string `json:"platform"`
		AccountName string `json:"account_name"`
		Username    string `json:"username"`
	}
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, fmt.Errorf("outstand finalize: decode account: %w", err)
	}

	return &RelayAccount{
		ID:          account.ID,
		Platform:    account.Platform,
		AccountName: account.AccountName,
		Username:    account.Username,
		Healthy:     true,
	}, nil
}

// ListAccounts retrieves all connected social accounts for a tenant.
func (c *OutstandClient) ListAccounts(ctx context.Context, tenantID string) ([]RelayAccount, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, "/social-accounts?tenantId="+tenantID, nil)
	if err != nil {
		return nil, fmt.Errorf("outstand list accounts: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("outstand list accounts: unexpected status %d: %s", status, string(body))
	}

	var accounts []RelayAccount
	if err := json.Unmarshal(body, &accounts); err != nil {
		return nil, fmt.Errorf("outstand list accounts: decode response: %w", err)
	}

	return accounts, nil
}

// DeleteAccount removes a connected social account.
func (c *OutstandClient) DeleteAccount(ctx context.Context, accountID string) error {
	_, status, err := c.doRequest(ctx, http.MethodDelete, "/social-accounts/"+accountID, nil)
	if err != nil {
		return fmt.Errorf("outstand delete account: %w", err)
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("outstand delete account: unexpected status %d", status)
	}
	return nil
}

// Publish posts content to a social account through the relay.
func (c *OutstandClient) Publish(ctx context.Context, accountID string, content string, opts PublishOpts) (*PostResult, error) {
	payload := map[string]any{
		"content":  content,
		"accounts": []string{accountID},
	}
	if opts.ScheduledAt != "" {
		payload["scheduledAt"] = opts.ScheduledAt
	}
	if len(opts.MediaURLs) > 0 {
		payload["media"] = opts.MediaURLs
	}

	body, status, err := c.doRequest(ctx, http.MethodPost, "/posts/", payload)
	if err != nil {
		return &PostResult{
			Success: false,
			Error:   fmt.Sprintf("outstand publish: %v", err),
		}, nil
	}

	if status != http.StatusOK && status != http.StatusCreated {
		return &PostResult{
			Success: false,
			Error:   fmt.Sprintf("outstand publish: unexpected status %d: %s", status, string(body)),
		}, nil
	}

	var response struct {
		ID       string `json:"id"`
		Accounts []struct {
			AccountID string `json:"account_id"`
			Status    string `json:"status"`
			PostURL   string `json:"post_url"`
			PostID    string `json:"post_id"`
			Error     string `json:"error"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return &PostResult{
			Success: false,
			Error:   fmt.Sprintf("outstand publish: decode response: %v", err),
		}, nil
	}

	result := &PostResult{
		Success: true,
		PostID:  response.ID,
	}

	// Check per-account status for our target account.
	for _, acct := range response.Accounts {
		if acct.AccountID == accountID {
			if acct.Status == "failed" || acct.Error != "" {
				result.Success = false
				result.Error = acct.Error
			} else {
				result.PostURL = acct.PostURL
				if acct.PostID != "" {
					result.PostID = acct.PostID
				}
			}
			break
		}
	}

	return result, nil
}

// GetAccountHealth checks whether a connected account is healthy.
func (c *OutstandClient) GetAccountHealth(ctx context.Context, accountID string) (bool, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, "/social-accounts/"+accountID+"/health", nil)
	if err != nil {
		return false, fmt.Errorf("outstand get account health: %w", err)
	}
	if status != http.StatusOK {
		return false, fmt.Errorf("outstand get account health: unexpected status %d: %s", status, string(body))
	}

	var health struct {
		Healthy bool `json:"healthy"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return false, fmt.Errorf("outstand get account health: decode response: %w", err)
	}

	return health.Healthy, nil
}

// Compile-time interface check.
var _ RelayClient = (*OutstandClient)(nil)
