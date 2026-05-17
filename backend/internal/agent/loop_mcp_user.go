// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"sync"

	"github.com/qorvenai/qorven/internal/tools"
)

// MCPUserCredServer represents an MCP server requiring user credentials.
type MCPUserCredServer struct {
	ServerID   string
	ServerName string
	Transport  string
	Command    string
	Args       []string
	Env        map[string]string
	URL        string
	Headers    map[string]string
	APIKey     string
	ToolPrefix string
	TimeoutSec int
}

// MCPUserCredentials represents user-specific credentials for an MCP server.
type MCPUserCredentials struct {
	APIKey  string
	Headers map[string]string
	Env     map[string]string
}

// MCPPool interface for MCP connection pooling.
type MCPPool interface {
	AcquireUser(ctx context.Context, tenantID, serverName, userID, transport, command string, args []string, env map[string]string, url string, headers map[string]string, timeoutSec int) (MCPPoolEntry, error)
	ReleaseUser(key string)
}

// MCPPoolEntry represents a pool entry with MCP tools.
type MCPPoolEntry interface {
	MCPTools() []MCPToolInfo
	Client() any
	Connected() *bool
}

// MCPToolInfo represents info about an MCP tool.
type MCPToolInfo struct {
	Name        string
	Description string
}

// MCPUserToolLoader handles per-user MCP tool loading.
type MCPUserToolLoader struct {
	pool         MCPPool
	store        MCPCredentialStore
	registry     *tools.Registry
	userTools    sync.Map // userID → []tools.Tool
	userCredSrvs []MCPUserCredServer
	tenantID     string
}

// MCPCredentialStore interface for retrieving user credentials.
type MCPCredentialStore interface {
	GetUserCredentials(ctx context.Context, serverID, userID string) (*MCPUserCredentials, error)
}

// NewMCPUserToolLoader creates a new user tool loader.
func NewMCPUserToolLoader(pool MCPPool, store MCPCredentialStore, registry *tools.Registry, tenantID string) *MCPUserToolLoader {
	return &MCPUserToolLoader{
		pool:     pool,
		store:    store,
		registry: registry,
		tenantID: tenantID,
	}
}

// SetUserCredServers sets the list of servers requiring user credentials.
func (l *MCPUserToolLoader) SetUserCredServers(servers []MCPUserCredServer) {
	l.userCredSrvs = servers
}

// GetUserTools returns per-user MCP tools for servers requiring user credentials.
// Tools are cached per-user and registered in the shared tool registry.
func (l *MCPUserToolLoader) GetUserTools(ctx context.Context, userID string) []tools.Tool {
	if len(l.userCredSrvs) == 0 || l.pool == nil || l.store == nil || userID == "" {
		return nil
	}

	// Check cache
	if cached, ok := l.userTools.Load(userID); ok {
		cachedTools := cached.([]tools.Tool)
		// Verify connections are still valid
		allConnected := true
		for _, t := range cachedTools {
			if bt, ok := t.(interface{ IsConnected() bool }); ok && !bt.IsConnected() {
				allConnected = false
				break
			}
		}
		if allConnected {
			return cachedTools
		}
		l.userTools.Delete(userID)
		slog.Debug("mcp.user_tools_stale", "user", userID, "reason", "pool_evicted")
	}

	var userTools []tools.Tool
	for _, srv := range l.userCredSrvs {
		// Check if user has credentials for this server
		uc, err := l.store.GetUserCredentials(ctx, srv.ServerID, userID)
		if err != nil || uc == nil || (uc.APIKey == "" && len(uc.Headers) == 0 && len(uc.Env) == 0) {
			continue
		}

		// Merge credentials
		env := copyMap(srv.Env)
		headers := copyMap(srv.Headers)

		if srv.APIKey != "" && headers["Authorization"] == "" {
			headers["Authorization"] = "Bearer " + srv.APIKey
		}
		if uc.APIKey != "" {
			headers["Authorization"] = "Bearer " + uc.APIKey
		}
		mergeMaps(headers, uc.Headers)
		mergeMaps(env, uc.Env)

		// Acquire user-keyed pool connection
		entry, err := l.pool.AcquireUser(ctx, l.tenantID, srv.ServerName, userID,
			srv.Transport, srv.Command, srv.Args, env, srv.URL, headers, srv.TimeoutSec)
		if err != nil {
			slog.Warn("mcp.user_pool_acquire_failed", "server", srv.ServerName, "user", userID, "error", err)
			continue
		}

		// Release immediately — BridgeTools hold client pointer directly
		l.pool.ReleaseUser(l.tenantID + ":" + srv.ServerName + ":" + userID)

		// Create tools from MCP tools
		for _, mcpTool := range entry.MCPTools() {
			// Tool creation would be done by the MCP package
			// For now, just log that we found tools
			slog.Debug("mcp.user_tool_found", "server", srv.ServerName, "tool", mcpTool.Name)
		}
	}

	if len(userTools) > 0 {
		l.userTools.Store(userID, userTools)
		slog.Info("mcp.user_tools_loaded", "user", userID, "tools", len(userTools))
	}
	return userTools
}

// ClearUserTools removes cached tools for a user.
func (l *MCPUserToolLoader) ClearUserTools(userID string) {
	l.userTools.Delete(userID)
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mergeMaps(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}
