// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// AgentMailProvider sends/receives email via AgentMail API (agentmail.to).
// Alternative to SMTP/IMAP — purpose-built for AI agents.
type AgentMailProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewAgentMailProvider(apiKey string) *AgentMailProvider {
	return &AgentMailProvider{
		apiKey:  apiKey,
		baseURL: "https://api.agentmail.to/v1",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// CreateInbox creates a new inbox for a Soul.
func (p *AgentMailProvider) CreateInbox(ctx context.Context, displayName string) (string, error) {
	body, _ := json.Marshal(map[string]string{"display_name": displayName})
	data, err := p.request(ctx, "POST", "/inboxes", body)
	if err != nil {
		return "", err
	}
	var resp struct{ ID string `json:"id"`; Address string `json:"address"` }
	json.Unmarshal(data, &resp)
	slog.Info("agentmail.inbox_created", "address", resp.Address)
	return resp.Address, nil
}

// Send sends an email via AgentMail.
func (p *AgentMailProvider) Send(ctx context.Context, inboxID, to, subject, body string) error {
	payload, _ := json.Marshal(map[string]any{
		"to":      []string{to},
		"subject": subject,
		"body":    body,
	})
	_, err := p.request(ctx, "POST", fmt.Sprintf("/inboxes/%s/messages", inboxID), payload)
	return err
}

// ListMessages lists messages in an inbox.
func (p *AgentMailProvider) ListMessages(ctx context.Context, inboxID string, limit int) ([]map[string]any, error) {
	data, err := p.request(ctx, "GET", fmt.Sprintf("/inboxes/%s/messages?limit=%d", inboxID, limit), nil)
	if err != nil {
		return nil, err
	}
	var resp struct{ Messages []map[string]any `json:"messages"` }
	json.Unmarshal(data, &resp)
	return resp.Messages, nil
}

// GetMessage gets a single message.
func (p *AgentMailProvider) GetMessage(ctx context.Context, inboxID, messageID string) (map[string]any, error) {
	data, err := p.request(ctx, "GET", fmt.Sprintf("/inboxes/%s/messages/%s", inboxID, messageID), nil)
	if err != nil {
		return nil, err
	}
	var msg map[string]any
	json.Unmarshal(data, &msg)
	return msg, nil
}

func (p *AgentMailProvider) request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, _ := http.NewRequestWithContext(ctx, method, p.baseURL+path, bodyReader)
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("agentmail %d: %s", resp.StatusCode, string(data[:min(len(data), 200)]))
	}
	return data, nil
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
