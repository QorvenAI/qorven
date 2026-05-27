// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const bufferGraphQLEndpoint = "https://api.buffer.com"

// BufferClient implements RelayClient for the Buffer social publishing platform.
// Buffer uses GraphQL and requires accounts to be connected via buffer.com dashboard.
type BufferClient struct {
	apiKey string
	http   *http.Client
}

// NewBufferClient creates a new Buffer relay client with the given API key.
func NewBufferClient(apiKey string) *BufferClient {
	return &BufferClient{
		apiKey: apiKey,
		http:   &http.Client{},
	}
}

// graphqlResponse represents a standard GraphQL response envelope.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// graphql sends a GraphQL request to Buffer's API and returns the data portion.
func (c *BufferClient) graphql(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bufferGraphQLEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("buffer request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("buffer API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, fmt.Errorf("buffer graphql error: %s", gqlResp.Errors[0].Message)
	}

	return gqlResp.Data, nil
}

// TestConnection verifies the API key is valid by querying the account.
func (c *BufferClient) TestConnection(ctx context.Context) error {
	_, err := c.graphql(ctx, `{ account { id } }`, nil)
	if err != nil {
		return fmt.Errorf("buffer connection test failed: %w", err)
	}
	return nil
}

// GetAuthURL returns an error because Buffer does not support programmatic OAuth.
func (c *BufferClient) GetAuthURL(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("buffer requires accounts to be connected via buffer.com dashboard")
}

// FinalizeConnection returns an error because Buffer does not support programmatic OAuth.
func (c *BufferClient) FinalizeConnection(_ context.Context, _ string) (*RelayAccount, error) {
	return nil, fmt.Errorf("buffer does not support programmatic OAuth")
}

// ListAccounts retrieves all channels across organizations from the Buffer account.
func (c *BufferClient) ListAccounts(ctx context.Context, _ string) ([]RelayAccount, error) {
	query := `{ account { organizations { id channels { id name service } } } }`

	data, err := c.graphql(ctx, query, nil)
	if err != nil {
		return nil, fmt.Errorf("list buffer accounts: %w", err)
	}

	var result struct {
		Account struct {
			Organizations []struct {
				ID       string `json:"id"`
				Channels []struct {
					ID      string `json:"id"`
					Name    string `json:"name"`
					Service string `json:"service"`
				} `json:"channels"`
			} `json:"organizations"`
		} `json:"account"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode buffer accounts: %w", err)
	}

	var accounts []RelayAccount
	for _, org := range result.Account.Organizations {
		for _, ch := range org.Channels {
			accounts = append(accounts, RelayAccount{
				ID:          ch.ID,
				Platform:    ch.Service,
				AccountName: ch.Name,
				Username:    ch.Name,
				Healthy:     true,
			})
		}
	}

	return accounts, nil
}

// DeleteAccount returns an error because Buffer channels must be managed via buffer.com.
func (c *BufferClient) DeleteAccount(_ context.Context, _ string) error {
	return fmt.Errorf("buffer channels must be disconnected via buffer.com dashboard")
}

// Publish creates a post on the specified Buffer channel.
func (c *BufferClient) Publish(ctx context.Context, accountID string, content string, opts PublishOpts) (*PostResult, error) {
	mutation := `mutation CreatePost($input: CreatePostInput!) { createPost(input: $input) { id status } }`

	input := map[string]any{
		"channelId": accountID,
		"text":      content,
	}
	if opts.ScheduledAt != "" {
		input["scheduledAt"] = opts.ScheduledAt
	}

	variables := map[string]any{
		"input": input,
	}

	data, err := c.graphql(ctx, mutation, variables)
	if err != nil {
		return &PostResult{
			Platform: PlatformTwitter,
			Success:  false,
			Error:    err.Error(),
		}, err
	}

	var result struct {
		CreatePost struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"createPost"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return &PostResult{
			Success: false,
			Error:   fmt.Sprintf("decode publish response: %v", err),
		}, err
	}

	return &PostResult{
		Success: true,
		PostID:  result.CreatePost.ID,
	}, nil
}

// GetAccountHealth checks whether a Buffer channel still exists and is accessible.
func (c *BufferClient) GetAccountHealth(ctx context.Context, accountID string) (bool, error) {
	query := fmt.Sprintf(`{ channels(filter: {ids: ["%s"]}) { id } }`, accountID)

	data, err := c.graphql(ctx, query, nil)
	if err != nil {
		return false, err
	}

	var result struct {
		Channels []struct {
			ID string `json:"id"`
		} `json:"channels"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return false, fmt.Errorf("decode health response: %w", err)
	}

	return len(result.Channels) > 0, nil
}

// Compile-time interface check.
var _ RelayClient = (*BufferClient)(nil)
