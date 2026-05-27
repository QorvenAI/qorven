package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// PostForMeClient implements RelayClient using the PostForMe relay API.
type PostForMeClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// NewPostForMeClient creates a new PostForMe relay client.
func NewPostForMeClient(apiKey string) *PostForMeClient {
	return &PostForMeClient{
		apiKey:  apiKey,
		baseURL: "https://api.postforme.dev/v1",
		http:    &http.Client{},
	}
}

// doRequest performs an authenticated HTTP request and returns the response body, status code, and any error.
func (c *PostForMeClient) doRequest(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
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

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response body: %w", err)
	}

	return data, resp.StatusCode, nil
}

// TestConnection verifies the API key is valid by listing social accounts.
func (c *PostForMeClient) TestConnection(ctx context.Context) error {
	_, status, err := c.doRequest(ctx, http.MethodGet, "/social-accounts", nil)
	if err != nil {
		return fmt.Errorf("postforme: test connection failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("postforme: test connection returned status %d", status)
	}
	return nil
}

// GetAuthURL requests an OAuth authorization URL for the given platform.
func (c *PostForMeClient) GetAuthURL(ctx context.Context, platform, tenantID, redirectURL string) (string, error) {
	payload := map[string]any{
		"platform":              platform,
		"external_id":           tenantID,
		"redirect_url_override": redirectURL,
		"permissions":           []string{"posts"},
	}

	data, status, err := c.doRequest(ctx, http.MethodPost, "/social-accounts/auth-url", payload)
	if err != nil {
		return "", fmt.Errorf("postforme: get auth url: %w", err)
	}
	if status < 200 || status >= 300 {
		return "", fmt.Errorf("postforme: get auth url returned status %d: %s", status, string(data))
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("postforme: decode auth url response: %w", err)
	}

	return result.URL, nil
}

// FinalizeConnection retrieves the most recently created account for the given external_id.
// PostForMe auto-finalizes after the OAuth callback; this fetches the resulting account.
func (c *PostForMeClient) FinalizeConnection(ctx context.Context, sessionToken string) (*RelayAccount, error) {
	path := "/social-accounts?external_id=" + url.QueryEscape(sessionToken)

	data, status, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("postforme: finalize connection: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("postforme: finalize connection returned status %d: %s", status, string(data))
	}

	var accounts []struct {
		ID       string `json:"id"`
		Platform string `json:"platform"`
		Username string `json:"username"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("postforme: decode accounts response: %w", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("postforme: no accounts found for session %s", sessionToken)
	}

	// Return the last (most recently created) account.
	acct := accounts[len(accounts)-1]
	return &RelayAccount{
		ID:          acct.ID,
		Platform:    acct.Platform,
		AccountName: acct.Username,
		Username:    acct.Username,
		Healthy:     acct.Status == "active",
	}, nil
}

// ListAccounts returns all social accounts associated with the given tenant.
func (c *PostForMeClient) ListAccounts(ctx context.Context, tenantID string) ([]RelayAccount, error) {
	path := "/social-accounts?external_id=" + url.QueryEscape(tenantID)

	data, status, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("postforme: list accounts: %w", err)
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("postforme: list accounts returned status %d: %s", status, string(data))
	}

	var accounts []struct {
		ID       string `json:"id"`
		Platform string `json:"platform"`
		Username string `json:"username"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("postforme: decode accounts response: %w", err)
	}

	result := make([]RelayAccount, 0, len(accounts))
	for _, acct := range accounts {
		result = append(result, RelayAccount{
			ID:          acct.ID,
			Platform:    acct.Platform,
			AccountName: acct.Username,
			Username:    acct.Username,
			Healthy:     acct.Status == "active",
		})
	}

	return result, nil
}

// DeleteAccount disconnects a social account from the relay.
func (c *PostForMeClient) DeleteAccount(ctx context.Context, accountID string) error {
	path := "/social-accounts/" + url.PathEscape(accountID) + "/disconnect"

	data, status, err := c.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("postforme: delete account: %w", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("postforme: delete account returned status %d: %s", status, string(data))
	}

	return nil
}

// Publish posts content to the specified social account.
func (c *PostForMeClient) Publish(ctx context.Context, accountID string, content string, opts PublishOpts) (*PostResult, error) {
	payload := map[string]any{
		"content":            content,
		"social_account_ids": []string{accountID},
	}
	if opts.ScheduledAt != "" {
		payload["scheduled_at"] = opts.ScheduledAt
	}
	if len(opts.MediaURLs) > 0 {
		payload["media_urls"] = opts.MediaURLs
	}

	data, status, err := c.doRequest(ctx, http.MethodPost, "/social-posts", payload)
	if err != nil {
		return nil, fmt.Errorf("postforme: publish: %w", err)
	}
	if status < 200 || status >= 300 {
		return &PostResult{
			Success: false,
			Error:   fmt.Sprintf("postforme: publish returned status %d: %s", status, string(data)),
		}, nil
	}

	var postResp struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Platform string `json:"platform"`
		PostURL  string `json:"post_url"`
	}
	if err := json.Unmarshal(data, &postResp); err != nil {
		return nil, fmt.Errorf("postforme: decode publish response: %w", err)
	}

	return &PostResult{
		Platform: Platform(postResp.Platform),
		Success:  postResp.Status != "failed",
		PostURL:  postResp.PostURL,
		PostID:   postResp.ID,
	}, nil
}

// GetAccountHealth checks whether the specified account is active and healthy.
func (c *PostForMeClient) GetAccountHealth(ctx context.Context, accountID string) (bool, error) {
	path := "/social-accounts/" + url.PathEscape(accountID)

	data, status, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, fmt.Errorf("postforme: get account health: %w", err)
	}
	if status < 200 || status >= 300 {
		return false, fmt.Errorf("postforme: get account health returned status %d: %s", status, string(data))
	}

	var acct struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &acct); err != nil {
		return false, fmt.Errorf("postforme: decode account health response: %w", err)
	}

	return acct.Status == "active", nil
}

// Compile-time interface assertion.
var _ RelayClient = (*PostForMeClient)(nil)
