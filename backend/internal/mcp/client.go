// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

// ServerConfig describes an MCP server to connect to.
type ServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"` // stdio, sse, streamable-http
	Command   string            `json:"command"`   // for stdio
	Args      []string          `json:"args"`      // for stdio
	URL       string            `json:"url"`       // for sse/http
	Headers   map[string]string `json:"headers"`
	Env       map[string]string `json:"env"`
	Prefix         string            `json:"tool_prefix"` // optional: prefix tool names
	Timeout        int               `json:"timeout_seconds"`
	Enabled        bool              `json:"enabled"`
	RequiredTools  []string          `json:"required_tools,omitempty"` // fail-closed: error if any of these are missing after connect
}

// DiscoveredTool is a tool discovered from an MCP server.
type DiscoveredTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	ServerName  string         `json:"server_name"`
}

// Client manages connections to MCP servers.
type Client struct {
	mu      sync.RWMutex
	servers map[string]*serverConn
}

type serverConn struct {
	name          string
	config        ServerConfig
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	scanner       *bufio.Scanner
	tools         []DiscoveredTool
	connected     atomic.Bool
	mu            sync.Mutex
	reqID         int
	httpTransport *HTTPTransport
}

func NewClient() *Client {
	return &Client{servers: make(map[string]*serverConn)}
}

// Connect starts an MCP server and discovers its tools.
func (c *Client) Connect(ctx context.Context, cfg ServerConfig) ([]DiscoveredTool, error) {
	if cfg.Transport != "stdio" {
		return nil, fmt.Errorf("transport %q not yet supported (only stdio)", cfg.Transport)
	}

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	// Build env
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil { return nil, fmt.Errorf("stdin pipe: %w", err) }
	stdout, err := cmd.StdoutPipe()
	if err != nil { return nil, fmt.Errorf("stdout pipe: %w", err) }

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", cfg.Command, err)
	}

	sc := &serverConn{
		name:    cfg.name(),
		config:  cfg,
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
	}
	sc.scanner.Buffer(make([]byte, 0, 1<<20), 1<<20) // 1MB buffer
	sc.connected.Store(true)

	// Initialize MCP handshake
	initResp, err := sc.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]string{"name": "qorven", "version": "1.0.0"},
		"capabilities":    map[string]any{},
	})
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("initialize: %w", err)
	}
	slog.Info("mcp.connected", "server", cfg.name(), "protocol", initResp)

	// Send initialized notification
	sc.notify("notifications/initialized", nil)

	// Discover tools
	toolsResp, err := sc.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("tools/list: %w", err)
	}

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
	sc.tools = discovered

	c.mu.Lock()
	c.servers[cfg.name()] = sc
	c.mu.Unlock()

	slog.Info("mcp.tools_discovered", "server", cfg.name(), "count", len(discovered))
	return discovered, nil
}

// CallTool invokes a tool on an MCP server.
func (c *Client) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (string, error) {
	c.mu.RLock()
	sc, ok := c.servers[serverName]
	c.mu.RUnlock()
	if !ok { return "", fmt.Errorf("server not connected: %s", serverName) }
	if !sc.connected.Load() { return "", fmt.Errorf("server disconnected: %s", serverName) }

	resp, err := sc.request(ctx, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": args,
	})
	if err != nil { return "", err }

	// Extract text content from response
	b, _ := json.Marshal(resp)
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	json.Unmarshal(b, &result)

	texts := []string{}
	for _, c := range result.Content {
		if c.Type == "text" { texts = append(texts, c.Text) }
	}
	return strings.Join(texts, "\n"), nil
}

// Disconnect stops an MCP server.
func (c *Client) Disconnect(name string) {
	c.mu.Lock()
	sc, ok := c.servers[name]
	if ok { delete(c.servers, name) }
	c.mu.Unlock()
	if ok && sc.cmd != nil && sc.cmd.Process != nil {
		sc.stdin.Close()
		sc.cmd.Process.Kill()
		sc.connected.Store(false)
	}
}

// DisconnectAll stops all MCP servers.
func (c *Client) DisconnectAll() {
	c.mu.Lock()
	names := make([]string, 0, len(c.servers))
	for n := range c.servers { names = append(names, n) }
	c.mu.Unlock()
	for _, n := range names { c.Disconnect(n) }
}

// ListServers returns connected server names and tool counts.
func (c *Client) ListServers() []map[string]any {
	c.mu.RLock(); defer c.mu.RUnlock()
	out := []map[string]any{}
	for _, sc := range c.servers {
		out = append(out, map[string]any{
			"name": sc.name, "transport": sc.config.Transport,
			"tools": len(sc.tools), "connected": sc.connected.Load(),
		})
	}
	return out
}

// VerifyTools checks that every name in required is available across all
// connected servers. Returns an error listing the missing tools so the
// caller can fail closed rather than silently degrade.
// If required is empty the call is always a no-op (backwards-compatible).
func (c *Client) VerifyTools(required []string) error {
	if len(required) == 0 {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	available := make(map[string]struct{})
	for _, sc := range c.servers {
		for _, t := range sc.tools {
			available[t.Name] = struct{}{}
		}
	}
	missing := []string{}
	for _, name := range required {
		if _, ok := available[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("mcp: required tools not available: %v", missing)
	}
	return nil
}

// --- JSON-RPC 2.0 over stdio ---

type jsonrpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (sc *serverConn) request(ctx context.Context, method string, params any) (any, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.reqID++
	req := jsonrpcRequest{JSONRPC: "2.0", ID: sc.reqID, Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')

	if _, err := sc.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	// Read response (skip notifications)
	for sc.scanner.Scan() {
		line := sc.scanner.Text()
		if line == "" { continue }
		var resp jsonrpcResponse
		if json.Unmarshal([]byte(line), &resp) != nil { continue }
		if resp.ID == 0 { continue } // notification, skip
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		var result any
		json.Unmarshal(resp.Result, &result)
		return result, nil
	}
	return nil, fmt.Errorf("connection closed")
}

func (sc *serverConn) notify(method string, params any) {
	req := jsonrpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	data, _ := json.Marshal(req)
	data = append(data, '\n')
	sc.stdin.Write(data)
}

func (cfg ServerConfig) name() string {
	if cfg.Name != "" { return cfg.Name }
	return cfg.Command
}

// ConnectAny routes to the appropriate transport, then verifies any
// required tools are available. If RequiredTools is non-empty and any
// tool is missing, the server is disconnected and an error is returned
// (fail-closed — the caller must not proceed with a degraded toolset).
func (c *Client) ConnectAny(ctx context.Context, cfg ServerConfig) ([]DiscoveredTool, error) {
	var (
		tools []DiscoveredTool
		err   error
	)
	switch cfg.Transport {
	case "stdio":
		tools, err = c.Connect(ctx, cfg)
	case "sse", "streamable-http", "http":
		tools, err = c.ConnectHTTP(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", cfg.Transport)
	}
	if err != nil {
		return nil, err
	}
	if verifyErr := c.VerifyTools(cfg.RequiredTools); verifyErr != nil {
		c.Disconnect(cfg.name())
		return nil, verifyErr
	}
	return tools, nil
}
