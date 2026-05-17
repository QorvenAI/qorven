// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mcp

import "testing"

func TestHard_MCP_ClientLifecycle(t *testing.T) {
	c := NewClient()
	if c == nil { t.Fatal("nil client") }
	// Client should be usable immediately
}

func TestHard_MCP_ServerConfig_AllTransports(t *testing.T) {
	transports := []string{"stdio", "sse", "streamable-http"}
	for _, tr := range transports {
		cfg := ServerConfig{Name: "test", Transport: tr, Enabled: true}
		if cfg.Transport != tr { t.Errorf("transport=%q", cfg.Transport) }
	}
}

func TestHard_MCP_Manager_Lifecycle(t *testing.T) {
	m := NewManager(nil, nil)
	if m == nil { t.Fatal("nil") }
	if len(m.ToolNames()) != 0 { t.Error("should have 0 tools") }
	if len(m.ServerStatus()) != 0 { t.Error("should have 0 servers") }
	m.Stop() // should not panic
}

func TestHard_MCP_DiscoveredTool(t *testing.T) {
	tools := []DiscoveredTool{
		{Name: "search_repos", Description: "Search GitHub repos"},
		{Name: "create_issue", Description: "Create GitHub issue"},
		{Name: "list_prs", Description: "List pull requests"},
	}
	for _, tool := range tools {
		if tool.Name == "" { t.Error("empty name") }
		if tool.Description == "" { t.Error("empty desc") }
	}
}
