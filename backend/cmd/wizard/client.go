// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package wizard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── HTTP client ───────────────────────────────────────────────────────────────

type Client struct {
	base  string
	token string
	http  *http.Client
}

func NewClient(base, token string) *Client {
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) SetToken(t string) { c.token = t }

func (c *Client) do(method, path string, body any) ([]byte, error) {
	var bodyR io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyR = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, bodyR)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return nil, fmt.Errorf("%s", e.Error)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// ── Endpoint methods ─────────────────────────────────────────────────────────

func (c *Client) Health() error {
	_, err := c.do("GET", "/health", nil)
	return err
}

type SetupCheckResp struct {
	SetupRequired bool `json:"setup_required"`
}

func (c *Client) SetupCheck() (SetupCheckResp, error) {
	data, err := c.do("GET", "/auth/setup-check", nil)
	if err != nil {
		return SetupCheckResp{}, err
	}
	var r SetupCheckResp
	json.Unmarshal(data, &r)
	return r, nil
}

func (c *Client) CreateAdmin(username, password, displayName string) error {
	_, err := c.do("POST", "/auth/setup", map[string]string{
		"username": username, "password": password, "display_name": displayName,
	})
	return err
}

type LoginResp struct {
	Token string `json:"token"`
}

func (c *Client) Login(username, password string) (string, error) {
	data, err := c.do("POST", "/auth/login", map[string]string{
		"username": username, "password": password,
	})
	if err != nil {
		return "", err
	}
	var r LoginResp
	json.Unmarshal(data, &r)
	return r.Token, nil
}

type ProviderManifest struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Category       string                   `json:"category"`
	AuthType       string                   `json:"auth_type"`
	DefaultAPIBase string                   `json:"default_api_base"`
	DefaultModel   string                   `json:"default_model"`
	Models         []string                 `json:"models"`
	Fields         []map[string]interface{} `json:"fields"`
}

func (c *Client) ProviderCatalog() ([]ProviderManifest, error) {
	data, err := c.do("GET", "/v1/providers/catalog", nil)
	if err != nil {
		return nil, err
	}
	var list []ProviderManifest
	json.Unmarshal(data, &list)
	return list, nil
}

type ProviderTestReq struct {
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	APIBase      string `json:"api_base,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	Region       string `json:"region,omitempty"`
	AWSAccessKey string `json:"aws_access_key,omitempty"`
	AWSSecretKey string `json:"aws_secret_key,omitempty"`
}

type ProviderTestResp struct {
	Success bool     `json:"success"`
	Sample  string   `json:"sample"`
	Models  []string `json:"models"`
	Error   string   `json:"error"`
}

func (c *Client) TestProvider(req ProviderTestReq) (ProviderTestResp, error) {
	data, err := c.do("POST", "/v1/providers/test", req)
	if err != nil {
		return ProviderTestResp{}, err
	}
	var r ProviderTestResp
	json.Unmarshal(data, &r)
	if !r.Success {
		msg := r.Error
		if msg == "" {
			msg = "provider test failed"
		}
		return r, fmt.Errorf("%s", msg)
	}
	return r, nil
}

type CreateProviderReq struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	ProviderType string `json:"provider_type"`
	APIBase      string `json:"api_base"`
	APIKey       string `json:"api_key"`
	AWSAccessKey string `json:"aws_access_key,omitempty"`
	AWSSecretKey string `json:"aws_secret_key,omitempty"`
	Region       string `json:"region,omitempty"`
	Enabled      bool   `json:"enabled"`
}

type ProviderResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) CreateProvider(req CreateProviderReq) (ProviderResp, error) {
	data, err := c.do("POST", "/v1/providers", req)
	if err != nil {
		return ProviderResp{}, err
	}
	var r ProviderResp
	json.Unmarshal(data, &r)
	return r, nil
}

func (c *Client) ListProviders() ([]ProviderResp, error) {
	data, err := c.do("GET", "/v1/providers", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Providers []ProviderResp `json:"providers"`
	}
	json.Unmarshal(data, &resp)
	return resp.Providers, nil
}

func (c *Client) ProviderModels(providerID string) ([]string, error) {
	data, err := c.do("GET", "/v1/providers/"+providerID+"/models", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Models []string `json:"models"`
	}
	json.Unmarshal(data, &resp)
	return resp.Models, nil
}

type AgentSummary struct {
	ID        string `json:"id"`
	AgentKey  string `json:"agent_key"`
	Name      string `json:"display_name"`
	Model     string `json:"model"`
	Role      string `json:"role"`
}

func (c *Client) ListAgents() ([]AgentSummary, error) {
	data, err := c.do("GET", "/v1/agents", nil)
	if err != nil {
		return nil, err
	}
	var list []AgentSummary
	if json.Unmarshal(data, &list) != nil {
		// Some endpoints wrap in object
		var resp struct {
			Agents []AgentSummary `json:"agents"`
		}
		json.Unmarshal(data, &resp)
		return resp.Agents, nil
	}
	return list, nil
}

func (c *Client) UpdateAgent(id string, body map[string]interface{}) error {
	_, err := c.do("PUT", "/v1/agents/"+id, body)
	return err
}

type CreateChannelReq struct {
	AgentID     string      `json:"agent_id"`
	ChannelType string      `json:"channel_type"`
	Name        string      `json:"name"`
	Config      interface{} `json:"config"`
}

func (c *Client) CreateChannel(req CreateChannelReq) error {
	_, err := c.do("POST", "/v1/channels", req)
	return err
}

type VoiceProviderReq struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // tts | stt
	Driver    string `json:"driver"`
	APIKey    string `json:"api_key,omitempty"`
	IsDefault bool   `json:"is_default"`
	Enabled   bool   `json:"enabled"`
}

func (c *Client) CreateVoiceProvider(req VoiceProviderReq) error {
	_, err := c.do("POST", "/v1/voice/providers", req)
	return err
}

type FinalizeReq struct {
	InstanceName string `json:"instance_name"`
	PrimeName    string `json:"prime_name"`
	PrimeIcon    string `json:"prime_icon"`
	Style        string `json:"style"`
	Language     string `json:"language"`
	TLSMode      string `json:"tls_mode"`
	TLSDomain    string `json:"tls_domain"`
	WebPort      string `json:"web_port"`
}

func (c *Client) Finalize(req FinalizeReq) error {
	_, err := c.do("POST", "/v1/setup/finalize", req)
	return err
}

func (c *Client) NetworkStatus() (map[string]interface{}, error) {
	data, err := c.do("GET", "/v1/network/status", nil)
	if err != nil {
		return nil, err
	}
	var r map[string]interface{}
	json.Unmarshal(data, &r)
	return r, nil
}
