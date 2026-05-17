// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mcp

import "testing"

func TestClient_New(t *testing.T) {
	c := NewClient()
	if c == nil { t.Fatal("nil") }
}

func TestServerConfig_Fields(t *testing.T) {
	cfg := ServerConfig{Name: "github", Transport: "stdio", Command: "npx", Args: []string{"-y", "mcp-github"}}
	if cfg.Name != "github" { t.Error("wrong name") }
	if cfg.Transport != "stdio" { t.Error("wrong transport") }
}

func TestDiscoveredTool_Fields(t *testing.T) {
	dt := DiscoveredTool{Name: "search_repos", Description: "Search GitHub repos"}
	if dt.Name != "search_repos" { t.Error("wrong name") }
}

func TestServerStatus_Fields(t *testing.T) {
	s := ServerStatus{Name: "github", Connected: true, ToolCount: 5}
	if !s.Connected { t.Error("should be connected") }
	if s.ToolCount != 5 { t.Error("wrong tool count") }
}

func TestManager_New(t *testing.T) {
	m := NewManager(nil, nil)
	if m == nil { t.Fatal("nil") }
}

func TestManager_ToolNames_Empty(t *testing.T) {
	m := NewManager(nil, nil)
	names := m.ToolNames()
	if len(names) != 0 { t.Error("should be empty") }
}

func TestManager_ServerStatus_Empty(t *testing.T) {
	m := NewManager(nil, nil)
	status := m.ServerStatus()
	if len(status) != 0 { t.Error("should be empty") }
}

func TestManager_Stop_NotStarted(t *testing.T) {
	m := NewManager(nil, nil)
	m.Stop() // should not panic
}
