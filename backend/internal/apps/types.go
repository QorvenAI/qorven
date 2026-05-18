// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import "time"

// App is a row from the apps table — an installed Qorven App.
type App struct {
	ID          string         `json:"id"`
	TenantID    string         `json:"tenant_id"`
	Slug        string         `json:"slug"`
	DisplayName string         `json:"display_name"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	Author      string         `json:"author"`
	IconURL     string         `json:"icon_url"`
	InstallPath string         `json:"install_path"`
	Enabled      bool           `json:"enabled"`
	Config       map[string]any `json:"config"`
	InstalledAt  time.Time      `json:"installed_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	Scope        string         `json:"scope"`          // workspace | agent | team
	OwnerAgentID string         `json:"owner_agent_id"`
	OwnerTeamID  string         `json:"owner_team_id"`
}

// Manifest is parsed from app.yaml inside every installed App directory.
type Manifest struct {
	Slug          string           `yaml:"slug"`
	DisplayName   string           `yaml:"display_name"`
	Version       string           `yaml:"version"`
	Description   string           `yaml:"description"`
	Author        string           `yaml:"author"`
	IconURL       string           `yaml:"icon_url"`
	RequiresEnv   []string         `yaml:"requires_env"`
	Permissions   []string         `yaml:"permissions"`
	Tools         []ToolDef        `yaml:"tools"`
	Hooks         []HookDef        `yaml:"hooks"`
	Frontend      FrontendManifest `yaml:"frontend"`
	MigrationsDir string           `yaml:"migrations_dir"`          // defaults to "migrations"
	Scope         string           `yaml:"scope,omitempty"`         // workspace | agent | team; defaults to workspace
	OwnerAgentID  string           `yaml:"owner_agent_id,omitempty"` // set when scope=agent
	OwnerTeamID   string           `yaml:"owner_team_id,omitempty"`  // set when scope=team
	DataSource    *DataSourceConfig `yaml:"data_source,omitempty"`
}

// DataSourceConfig describes the connector data source scheduling config in app.yaml.
type DataSourceConfig struct {
	Enabled   bool           `yaml:"enabled"`
	Schedule  string         `yaml:"schedule"`   // standard cron e.g. "0 9 * * *"
	Tool      string         `yaml:"tool"`
	Args      map[string]any `yaml:"args"`
	ResultKey string         `yaml:"result_key"` // stored as key in snapshot JSONB
}

// FrontendManifest describes what UI surfaces an App injects.
type FrontendManifest struct {
	Bundle      string    `yaml:"bundle"`       // path relative to install dir; default "frontend/bundle.js"
	Pages       []PageDef `yaml:"pages"`
	AgentTabs   []TabDef  `yaml:"agent_tabs"`
	SettingTabs []TabDef  `yaml:"setting_tabs"`
}

// ToolDef mirrors a tools: entry in app.yaml.
type ToolDef struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Command     string         `yaml:"command"` // path relative to install dir
	Parameters  map[string]any `yaml:"parameters"`
	Timeout     int            `yaml:"timeout"` // seconds; 0 → 30s default
}

// HookDef mirrors a hooks: entry in app.yaml.
type HookDef struct {
	Event   string `yaml:"event"`
	Command string `yaml:"command"` // path relative to install dir
}

// PageDef declares a top-level page the App adds to the Apps section.
type PageDef struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Icon  string `yaml:"icon"`
	Path  string `yaml:"path"` // mounted at /apps/{slug}/{path}
}

// TabDef declares a tab injected into the Qor workspace or Settings page.
type TabDef struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Icon  string `yaml:"icon"`
	Order int    `yaml:"order"`
}

// AppFrontendEntry is returned by the API to tell the frontend which bundle to load.
type AppFrontendEntry struct {
	AppID     string           `json:"app_id"`
	Slug      string           `json:"slug"`
	BundleURL string           `json:"bundle_url"` // e.g. /app-assets/{slug}/bundle.js
	Manifest  FrontendManifest `json:"manifest"`
}

// validPermissions is the allowlist of permission strings an app may declare.
var validPermissions = map[string]bool{
	"tool_register":  true,
	"hook_register":  true,
	"db_write":       true,
	"inbound_pipeline": true,
	"files_read":     true,
}
