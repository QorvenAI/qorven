// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import "time"

// types.go — Browser automation types for Qorven.

// Config holds browser session configuration.
type Config struct {
	Headless      bool          `json:"headless" toml:"headless"`
	RemoteURL     string        `json:"remote_url" toml:"remote_url"`
	MaxPages      int           `json:"max_pages" toml:"max_pages"`
	ActionTimeout time.Duration `json:"action_timeout" toml:"action_timeout"`
	IdleTimeout   time.Duration `json:"idle_timeout" toml:"idle_timeout"`
	UserAgent     string        `json:"user_agent" toml:"user_agent"`
}

func DefaultConfig() Config {
	return Config{
		Headless:      true,
		MaxPages:      5,
		ActionTimeout: 30 * time.Second,
		IdleTimeout:   5 * time.Minute,
	}
}

// TabInfo describes an open browser tab.
type TabInfo struct {
	TargetID string `json:"target_id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Active   bool   `json:"active"`
}

// SnapshotResult holds a page's accessibility tree as text for LLM consumption.
type SnapshotResult struct {
	URL   string         `json:"url"`
	Title string         `json:"title"`
	Tree  string         `json:"tree"`
	Stats SnapshotStats  `json:"stats"`
	Refs  map[string]Ref `json:"refs,omitempty"`
}

type SnapshotStats struct {
	Nodes    int `json:"nodes"`
	MaxDepth int `json:"max_depth"`
}

// Ref maps a short reference ID to a DOM element for actions.
type Ref struct {
	Role     string `json:"role"`
	Name     string `json:"name"`
	NodeID   string  `json:"node_id"`
	BackendID int64 `json:"backend_id"`
}

// ActResult holds the result of a browser action.
type ActResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// ClickOpts configures click behavior.
type ClickOpts struct {
	Button     string `json:"button,omitempty"` // left, right, middle
	ClickCount int    `json:"click_count,omitempty"`
}

// TypeOpts configures typing behavior.
type TypeOpts struct {
	Clear bool `json:"clear,omitempty"` // clear field before typing
	Delay int  `json:"delay_ms,omitempty"`
}

// WaitOpts configures wait behavior.
type WaitOpts struct {
	Selector string        `json:"selector,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
	Visible  bool          `json:"visible,omitempty"`
}

// ConsoleMessage is a captured browser console log entry.
type ConsoleMessage struct {
	Level   string `json:"level"` // log, warn, error, info
	Text    string `json:"text"`
	URL     string `json:"url,omitempty"`
	LineNo  int    `json:"line,omitempty"`
}

// StatusInfo describes the current browser state.
type StatusInfo struct {
	Running   bool      `json:"running"`
	Headless  bool      `json:"headless"`
	TabCount  int       `json:"tab_count"`
	ActiveTab string    `json:"active_tab"`
	Uptime    string    `json:"uptime,omitempty"`
}
