// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	healthCheckInterval   = 30 * time.Second
	healthFailThreshold   = 3
	initialBackoff        = 2 * time.Second
	maxBackoff            = 60 * time.Second
	maxReconnectAttempts  = 10
	mcpToolInlineMaxCount = 40
)

// ServerStatus reports the connection status of an MCP server.
type ServerStatus struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Connected bool   `json:"connected"`
	ToolCount int    `json:"tool_count"`
	Error     string `json:"error,omitempty"`
}

// serverState tracks a single MCP server connection.
type serverState struct {
	name       string
	transport  string
	client     *Client
	connected  atomic.Bool
	toolNames  []string
	timeoutSec int
	cancel     context.CancelFunc

	mu             sync.Mutex
	reconnAttempts int
	healthFailures int
	lastErr        string
}

// DBServer is an MCP server stored in the database.
type DBServer struct {
	ID              string            `json:"id"`
	TenantID        string            `json:"tenant_id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Transport       string            `json:"transport"`
	Command         string            `json:"command"`
	Args            []string          `json:"args"`
	URL             string            `json:"url"`
	Env             map[string]string `json:"env"`
	Headers         map[string]string `json:"headers"`
	ToolPrefix      string            `json:"tool_prefix"`
	TimeoutSec      int               `json:"timeout_sec"`
	ToolsDiscovered json.RawMessage   `json:"tools_discovered"`
	AssignedAgents  []string          `json:"assigned_agents"`
	Enabled         bool              `json:"enabled"`
	Installed       bool              `json:"installed"`
}

// Manager orchestrates MCP server connections and tool registration.
type Manager struct {
	mu       sync.RWMutex
	servers  map[string]*serverState
	pool     *pgxpool.Pool
	client   *Client
	stopCh   chan struct{}

	// Search mode: deferred tools not registered inline
	deferredTools  map[string]*DiscoveredTool
	activatedTools map[string]struct{}
	searchMode     bool
}

// NewManager creates a new MCP Manager.
func NewManager(pool *pgxpool.Pool, client *Client) *Manager {
	return &Manager{
		servers:        make(map[string]*serverState),
		pool:           pool,
		client:         client,
		stopCh:         make(chan struct{}),
		deferredTools:  make(map[string]*DiscoveredTool),
		activatedTools: make(map[string]struct{}),
	}
}

func (m *Manager) Client() *Client { return m.client }

// Start connects to all enabled MCP servers.
func (m *Manager) Start(ctx context.Context, tenantID string) error {
	servers, err := m.List(ctx, tenantID)
	if err != nil {
		return err
	}

	errs := []string{}
	for _, s := range servers {
		if !s.Enabled || !s.Installed {
			continue
		}
		if err := m.connectServer(ctx, s); err != nil {
			slog.Warn("mcp.server.connect_failed", "server", s.Name, "error", err)
			errs = append(errs, fmt.Sprintf("%s: %v", s.Name, err))
		}
	}

	// Start health check goroutine
	go m.healthCheckLoop()

	if len(errs) > 0 {
		return fmt.Errorf("some MCP servers failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// connectServer establishes connection to a single MCP server.
func (m *Manager) connectServer(ctx context.Context, s DBServer) error {
	cfg := ServerConfig{
		Name:      s.Name,
		Transport: s.Transport,
		Command:   s.Command,
		Args:      s.Args,
		URL:       s.URL,
		Env:       s.Env,
		Headers:   s.Headers,
		Prefix:    s.ToolPrefix,
		Timeout:   s.TimeoutSec,
		Enabled:   true,
	}

	tools, err := m.client.ConnectAny(ctx, cfg)
	if err != nil {
		return err
	}

	_, cancel := context.WithCancel(context.Background())
	ss := &serverState{
		name:       s.Name,
		transport:  s.Transport,
		client:     m.client,
		timeoutSec: s.TimeoutSec,
		cancel:     cancel,
	}
	ss.connected.Store(true)

	// Register tools
	for _, t := range tools {
		toolName := t.Name
		if s.ToolPrefix != "" {
			toolName = s.ToolPrefix + "_" + t.Name
		}
		ss.toolNames = append(ss.toolNames, toolName)
	}

	m.mu.Lock()
	m.servers[s.Name] = ss
	m.mu.Unlock()

	slog.Info("mcp.server.connected", "server", s.Name, "tools", len(tools))
	return nil
}

// healthCheckLoop periodically pings connected servers.
func (m *Manager) healthCheckLoop() {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkAllServers()
		}
	}
}

func (m *Manager) checkAllServers() {
	m.mu.RLock()
	servers := make([]*serverState, 0, len(m.servers))
	for _, ss := range m.servers {
		servers = append(servers, ss)
	}
	m.mu.RUnlock()

	for _, ss := range servers {
		if !ss.connected.Load() {
			continue
		}
		// Check if server is still responsive
		// For now, just check if the connection is still marked as connected
		// A full implementation would send a ping/list request
		ss.mu.Lock()
		if ss.healthFailures >= healthFailThreshold {
			ss.connected.Store(false)
			slog.Warn("mcp.server.disconnected", "server", ss.name, "failures", ss.healthFailures)
		}
		ss.mu.Unlock()
	}
}

// Stop shuts down all MCP server connections.
func (m *Manager) Stop() {
	close(m.stopCh)

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, ss := range m.servers {
		if ss.cancel != nil {
			ss.cancel()
		}
		slog.Info("mcp.server.stopped", "server", name)
	}
	m.servers = make(map[string]*serverState)
}

// ServerStatus returns the status of all connected MCP servers.
func (m *Manager) ServerStatus() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]ServerStatus, 0, len(m.servers))
	for _, ss := range m.servers {
		statuses = append(statuses, ServerStatus{
			Name:      ss.name,
			Transport: ss.transport,
			Connected: ss.connected.Load(),
			ToolCount: len(ss.toolNames),
			Error:     ss.lastErr,
		})
	}
	return statuses
}

// ToolNames returns all registered MCP tool names.
func (m *Manager) ToolNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := []string{}
	for _, ss := range m.servers {
		names = append(names, ss.toolNames...)
	}
	return names
}

// IsSearchMode reports whether the manager is in deferred/search mode.
func (m *Manager) IsSearchMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.searchMode
}

// ActivateTools moves named deferred tools into active state.
func (m *Manager) ActivateTools(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, name := range names {
		if _, ok := m.deferredTools[name]; ok {
			m.activatedTools[name] = struct{}{}
			delete(m.deferredTools, name)
		}
	}

	slog.Info("mcp.tools.activated", "tools", names)
}

// DB operations

func (m *Manager) Add(ctx context.Context, tenantID string, s DBServer) (*DBServer, error) {
	s.TenantID = tenantID
	envJSON, _ := json.Marshal(s.Env)
	headersJSON, _ := json.Marshal(s.Headers)
	err := m.pool.QueryRow(ctx,
		`INSERT INTO mcp_servers (tenant_id, name, description, transport, command, args, url, env_encrypted, headers, tool_prefix, timeout_sec, assigned_agents)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT (tenant_id, name) DO UPDATE SET description=$3, transport=$4, command=$5, args=$6, url=$7, env_encrypted=$8, headers=$9, tool_prefix=$10, timeout_sec=$11, assigned_agents=$12, updated_at=NOW()
		 RETURNING id`, tenantID, s.Name, s.Description, s.Transport, s.Command, s.Args, s.URL, envJSON, headersJSON, s.ToolPrefix, s.TimeoutSec, s.AssignedAgents,
	).Scan(&s.ID)
	return &s, err
}

func (m *Manager) List(ctx context.Context, tenantID string) ([]DBServer, error) {
	rows, err := m.pool.Query(ctx,
		`SELECT id, name, description, transport, command, args, url, env_encrypted, COALESCE(headers, '{}'), COALESCE(tool_prefix, ''), COALESCE(timeout_sec, 30), tools_discovered, assigned_agents, enabled, installed
		 FROM mcp_servers WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []DBServer{}
	for rows.Next() {
		var s DBServer
		var envJSON, headersJSON json.RawMessage
		s.TenantID = tenantID
		rows.Scan(&s.ID, &s.Name, &s.Description, &s.Transport, &s.Command, &s.Args, &s.URL, &envJSON, &headersJSON, &s.ToolPrefix, &s.TimeoutSec, &s.ToolsDiscovered, &s.AssignedAgents, &s.Enabled, &s.Installed)
		json.Unmarshal(envJSON, &s.Env)
		json.Unmarshal(headersJSON, &s.Headers)
		out = append(out, s)
	}
	return out, nil
}

func (m *Manager) Get(ctx context.Context, id string) (*DBServer, error) {
	var s DBServer
	var envJSON, headersJSON json.RawMessage
	err := m.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, description, transport, command, args, url, env_encrypted, COALESCE(headers, '{}'), COALESCE(tool_prefix, ''), COALESCE(timeout_sec, 30), tools_discovered, assigned_agents, enabled, installed
		 FROM mcp_servers WHERE id=$1`, id,
	).Scan(&s.ID, &s.TenantID, &s.Name, &s.Description, &s.Transport, &s.Command, &s.Args, &s.URL, &envJSON, &headersJSON, &s.ToolPrefix, &s.TimeoutSec, &s.ToolsDiscovered, &s.AssignedAgents, &s.Enabled, &s.Installed)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(envJSON, &s.Env)
	json.Unmarshal(headersJSON, &s.Headers)
	return &s, nil
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	_, err := m.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id=$1`, id)
	return err
}

func (m *Manager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := m.pool.Exec(ctx, `UPDATE mcp_servers SET enabled=$1, updated_at=NOW() WHERE id=$2`, enabled, id)
	return err
}

// Install connects to the MCP server, discovers tools, saves to DB.
func (m *Manager) Install(ctx context.Context, id string) ([]DiscoveredTool, error) {
	s, err := m.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	cfg := ServerConfig{
		Name:      s.Name,
		Transport: s.Transport,
		Command:   s.Command,
		Args:      s.Args,
		URL:       s.URL,
		Env:       s.Env,
		Headers:   s.Headers,
		Prefix:    s.ToolPrefix,
		Timeout:   s.TimeoutSec,
		Enabled:   true,
	}

	tools, err := m.client.ConnectAny(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", s.Name, err)
	}

	toolsJSON, _ := json.Marshal(tools)
	m.pool.Exec(ctx, `UPDATE mcp_servers SET installed=true, tools_discovered=$1, updated_at=NOW() WHERE id=$2`, toolsJSON, id)
	return tools, nil
}

// Uninstall disconnects from the MCP server.
func (m *Manager) Uninstall(ctx context.Context, id string) error {
	s, err := m.Get(ctx, id)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if ss, ok := m.servers[s.Name]; ok {
		if ss.cancel != nil {
			ss.cancel()
		}
		delete(m.servers, s.Name)
	}
	m.mu.Unlock()

	_, err = m.pool.Exec(ctx, `UPDATE mcp_servers SET installed=false, tools_discovered=NULL, updated_at=NOW() WHERE id=$1`, id)
	return err
}

// BuildKnowledge generates a compact MCP server manifest for agent prompts.
// Emits one line per enabled server (name + tool count). Full tool details are
// available on demand via list_mcp_tools(server).
func (m *Manager) BuildKnowledge(ctx context.Context, tenantID, agentID string) (string, error) {
	servers, err := m.List(ctx, tenantID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	n := 0
	for _, s := range servers {
		if !s.Enabled || !s.Installed {
			continue
		}
		if agentID != "" && len(s.AssignedAgents) > 0 && !containsStr(s.AssignedAgents, agentID) {
			continue
		}
		discovered := []DiscoveredTool{}
		json.Unmarshal(s.ToolsDiscovered, &discovered)
		if len(discovered) == 0 {
			continue
		}
		if n == 0 {
			sb.WriteString("### MCP Servers\nCall MCP tools directly by name. Use list_mcp_tools(server) for tool details.\n")
		}
		n++
		sb.WriteString(fmt.Sprintf("- %s (%d tools)\n", s.Name, len(discovered)))
	}
	if n == 0 {
		return "", nil
	}
	return sb.String(), nil
}

// BuildKnowledgeFull returns the original full tool-list dump (for diagnostics only).
func (m *Manager) BuildKnowledgeFull(ctx context.Context, tenantID, agentID string) (string, error) {
	servers, err := m.List(ctx, tenantID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("### MCP Servers (connected)\nCall these tools directly by name.\n\n")
	n := 0
	for _, s := range servers {
		if !s.Enabled || !s.Installed {
			continue
		}
		if agentID != "" && len(s.AssignedAgents) > 0 && !containsStr(s.AssignedAgents, agentID) {
			continue
		}
		n++
		discovered := []DiscoveredTool{}
		json.Unmarshal(s.ToolsDiscovered, &discovered)
		names := make([]string, len(discovered))
		for i, t := range discovered {
			names[i] = t.Name
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** — %s\n   Tools: %s\n\n", n, s.Name, s.Description, strings.Join(names, ", ")))
	}
	if n == 0 {
		return "", nil
	}
	return sb.String(), nil
}

// GetToolDescriptions returns a map of tool name to description for inline mode.
func (m *Manager) GetToolDescriptions(ctx context.Context, tenantID, agentID string) (map[string]string, error) {
	servers, err := m.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	descs := make(map[string]string)
	for _, s := range servers {
		if !s.Enabled || !s.Installed {
			continue
		}
		if agentID != "" && len(s.AssignedAgents) > 0 && !containsStr(s.AssignedAgents, agentID) {
			continue
		}
		tools := []DiscoveredTool{}
		json.Unmarshal(s.ToolsDiscovered, &tools)
		for _, t := range tools {
			name := t.Name
			if s.ToolPrefix != "" {
				name = s.ToolPrefix + "_" + t.Name
			}
			descs[name] = t.Description
		}
	}
	return descs, nil
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
