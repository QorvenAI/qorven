// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPTransport implements MCP over streamable-http.
type HTTPTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	reqID   int
}

func NewHTTPTransport(url string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{url: url, headers: headers, client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *HTTPTransport) Request(ctx context.Context, method string, params any) (any, error) {
	t.reqID++
	req := jsonrpcRequest{JSONRPC: "2.0", ID: t.reqID, Method: method, Params: params}
	body, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.url, bytes.NewReader(body))
	if err != nil { return nil, err }
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers { httpReq.Header.Set(k, v) }

	resp, err := t.client.Do(httpReq)
	if err != nil { return nil, fmt.Errorf("http request: %w", err) }
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	var rpcResp jsonrpcResponse
	if err := json.Unmarshal(b, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse response: %w (body: %s)", err, string(b[:min(200, len(b))]))
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	var result any
	json.Unmarshal(rpcResp.Result, &result)
	return result, nil
}

// ConnectHTTP connects to an MCP server over HTTP/SSE.
func (c *Client) ConnectHTTP(ctx context.Context, cfg ServerConfig) ([]DiscoveredTool, error) {
	transport := NewHTTPTransport(cfg.URL, cfg.Headers)

	// Initialize
	_, err := transport.Request(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]string{"name": "qorven", "version": "1.0.0"},
		"capabilities":    map[string]any{},
	})
	if err != nil { return nil, fmt.Errorf("initialize: %w", err) }

	// Discover tools
	toolsResp, err := transport.Request(ctx, "tools/list", map[string]any{})
	if err != nil { return nil, fmt.Errorf("tools/list: %w", err) }

	var toolsResult struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	b, _ := json.Marshal(toolsResp)
	json.Unmarshal(b, &toolsResult)

	discovered := []DiscoveredTool{}
	for _, t := range toolsResult.Tools {
		name := t.Name
		if cfg.Prefix != "" { name = cfg.Prefix + "__" + t.Name }
		discovered = append(discovered, DiscoveredTool{
			Name: name, Description: t.Description,
			InputSchema: t.InputSchema, ServerName: cfg.name(),
		})
	}

	// Store as HTTP connection
	sc := &serverConn{name: cfg.name(), config: cfg, tools: discovered}
	sc.connected.Store(true)
	sc.httpTransport = transport

	c.mu.Lock()
	c.servers[cfg.name()] = sc
	c.mu.Unlock()

	return discovered, nil
}

// CallToolAny routes to stdio or HTTP based on connection type.
func (c *Client) CallToolAny(ctx context.Context, serverName, toolName string, args map[string]any) (string, error) {
	c.mu.RLock()
	sc, ok := c.servers[serverName]
	c.mu.RUnlock()
	if !ok { return "", fmt.Errorf("server not connected: %s", serverName) }

	if sc.httpTransport != nil {
		resp, err := sc.httpTransport.Request(ctx, "tools/call", map[string]any{"name": toolName, "arguments": args})
		if err != nil { return "", err }
		b, _ := json.Marshal(resp)
		var result struct {
			Content []struct { Type string `json:"type"`; Text string `json:"text"` } `json:"content"`
		}
		json.Unmarshal(b, &result)
		texts := []string{}
		for _, ct := range result.Content { if ct.Type == "text" { texts = append(texts, ct.Text) } }
		return strings.Join(texts, "\n"), nil
	}
	return c.CallTool(ctx, serverName, toolName, args)
}

// GetAllTools returns all discovered tools across all servers.
func (c *Client) GetAllTools() []DiscoveredTool {
	c.mu.RLock(); defer c.mu.RUnlock()
	all := []DiscoveredTool{}
	for _, sc := range c.servers {
		all = append(all, sc.tools...)
	}
	return all
}

// AutoConnect connects to all enabled servers from config.
func (c *Client) AutoConnect(ctx context.Context, configs []ServerConfig) int {
	count := 0
	for _, cfg := range configs {
		if !cfg.Enabled { continue }
		var err error
		switch cfg.Transport {
		case "stdio":
			_, err = c.Connect(ctx, cfg)
		case "sse", "streamable-http", "http":
			_, err = c.ConnectHTTP(ctx, cfg)
		default:
			err = fmt.Errorf("unsupported transport: %s", cfg.Transport)
		}
		if err != nil {
			fmt.Printf("mcp: failed to connect %s: %v\n", cfg.name(), err)
			continue
		}
		count++
	}
	return count
}

func min(a, b int) int { if a < b { return a }; return b }
