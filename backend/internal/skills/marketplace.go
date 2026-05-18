// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	
)

// MarketplaceStore manages the skill marketplace and installations.
type MarketplaceStore struct {
	pool      *pgxpool.Pool
	connectMCP func(name, transport, command string, args []string) (int, error)
	basePath  string // /opt/qorven/skills
}

func NewMarketplaceStore(pool *pgxpool.Pool, connectMCP func(name, transport, command string, args []string) (int, error)) *MarketplaceStore {
	base := "/opt/qorven/skills"
	os.MkdirAll(base, 0755)
	return &MarketplaceStore{pool: pool, connectMCP: connectMCP, basePath: base}
}

// Manifest is a skill from the marketplace catalog.
type Manifest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	RepoURL     string   `json:"repo_url"`
	License     string   `json:"license"`
	Type        string   `json:"type"`
	Transport   string   `json:"transport"`
	InstallCmd  string   `json:"install_cmd"`
	StartCmd    string   `json:"start_cmd"`
	Tags        []string `json:"tags"`
	Verified    bool     `json:"verified"`
	Stars       int      `json:"stars"`
}

// Installation is a user's installed skill.
type Installation struct {
	ID              string    `json:"id"`
	ManifestID      string    `json:"manifest_id"`
	Status          string    `json:"status"`
	ToolsRegistered int       `json:"tools_registered"`
	ErrorMsg        string    `json:"error_msg,omitempty"`
	InstalledAt     time.Time `json:"installed_at"`
}

// ListMarketplace returns all available skills.
func (s *MarketplaceStore) ListMarketplace(ctx context.Context) ([]Manifest, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, COALESCE(description,''), COALESCE(repo_url,''), COALESCE(license,''),
		        type, transport, COALESCE(install_cmd,''), COALESCE(start_cmd,''),
		        COALESCE(tags, ARRAY[]::TEXT[]), verified, COALESCE(stars,0)
		 FROM skill_manifests ORDER BY verified DESC, stars DESC, name`)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []Manifest{}
	for rows.Next() {
		var m Manifest
		rows.Scan(&m.ID, &m.Name, &m.Description, &m.RepoURL, &m.License,
			&m.Type, &m.Transport, &m.InstallCmd, &m.StartCmd, &m.Tags, &m.Verified, &m.Stars)
		out = append(out, m)
	}
	return out, nil
}

// ListInstalled returns installed skills for a tenant.
func (s *MarketplaceStore) ListInstalled(ctx context.Context, tenantID string) ([]Installation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, manifest_id, status, tools_registered, COALESCE(error_msg,''), installed_at
		 FROM skill_installations WHERE tenant_id = $1 ORDER BY installed_at DESC`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []Installation{}
	for rows.Next() {
		var i Installation
		rows.Scan(&i.ID, &i.ManifestID, &i.Status, &i.ToolsRegistered, &i.ErrorMsg, &i.InstalledAt)
		out = append(out, i)
	}
	return out, nil
}

// Install installs a skill from the marketplace.
func (s *MarketplaceStore) Install(ctx context.Context, tenantID, manifestID string) (*Installation, error) {
	// Get manifest
	var m Manifest
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, type, transport, install_cmd, start_cmd FROM skill_manifests WHERE id = $1`, manifestID,
	).Scan(&m.ID, &m.Name, &m.Type, &m.Transport, &m.InstallCmd, &m.StartCmd)
	if err != nil { return nil, fmt.Errorf("skill not found: %s", manifestID) }

	// Create installation record
	var instID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO skill_installations (tenant_id, manifest_id, status) VALUES ($1, $2, 'installing')
		 ON CONFLICT (tenant_id, manifest_id) DO UPDATE SET status = 'installing', error_msg = NULL
		 RETURNING id`, tenantID, manifestID).Scan(&instID)
	if err != nil { return nil, err }

	// Run install command in background
	go s.runInstall(tenantID, instID, m)

	return &Installation{ID: instID, ManifestID: manifestID, Status: "installing"}, nil
}

func (s *MarketplaceStore) runInstall(tenantID, instID string, m Manifest) {
	ctx := context.Background()
	installPath := filepath.Join(s.basePath, m.ID)
	os.MkdirAll(installPath, 0755)

	// Run install command
	if m.InstallCmd != "" {
		slog.Info("skill.installing", "skill", m.ID, "cmd", m.InstallCmd)
		parts := strings.Fields(m.InstallCmd)
		cmd := exec.Command(parts[0], parts[1:]...)
		cmd.Dir = installPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := fmt.Sprintf("install failed: %v\n%s", err, string(output))
			slog.Warn("skill.install_failed", "skill", m.ID, "error", errMsg)
			s.pool.Exec(ctx, `UPDATE skill_installations SET status = 'failed', error_msg = $2 WHERE id = $1`, instID, errMsg)
			return
		}
		slog.Info("skill.installed", "skill", m.ID)
	}

	// Start MCP server and connect
	if m.StartCmd != "" && s.connectMCP != nil {
		parts := strings.Fields(m.StartCmd)
		cmd := parts[0]
		args := []string{}
		if len(parts) > 1 { args = parts[1:] }

		toolCount, err := s.connectMCP("skill-"+m.ID, m.Transport, cmd, args)
		if err != nil {
			errMsg := fmt.Sprintf("MCP connect failed: %v", err)
			slog.Warn("skill.mcp_failed", "skill", m.ID, "error", errMsg)
			s.pool.Exec(ctx, `UPDATE skill_installations SET status = 'failed', error_msg = $2 WHERE id = $1`, instID, errMsg)
			return
		}

		s.pool.Exec(ctx,
			`UPDATE skill_installations SET status = 'running', tools_registered = $2, started_at = NOW() WHERE id = $1`,
			instID, toolCount)
		slog.Info("skill.started", "skill", m.ID, "tools", toolCount)
	} else {
		s.pool.Exec(ctx, `UPDATE skill_installations SET status = 'stopped' WHERE id = $1`, instID)
	}
}

// Uninstall removes a skill installation.
func (s *MarketplaceStore) Uninstall(ctx context.Context, tenantID, manifestID string) error {
	// Disconnect MCP server
	// Remove installation record
	_, err := s.pool.Exec(ctx,
		`DELETE FROM skill_installations WHERE tenant_id = $1 AND manifest_id = $2`, tenantID, manifestID)
	// Clean up files
	os.RemoveAll(filepath.Join(s.basePath, manifestID))
	return err
}

// AddCustomSkill lets users add any repo as a skill.
// It auto-detects MCP support by checking for common patterns.
func (s *MarketplaceStore) AddCustomSkill(ctx context.Context, repoURL, name string) (*Manifest, error) {
	// Generate ID from repo URL
	id := strings.ReplaceAll(strings.TrimSuffix(filepath.Base(repoURL), ".git"), "/", "-")
	id = strings.ToLower(id)

	// Detect type from repo URL
	skillType := "mcp"
	transport := "stdio"
	installCmd := ""
	startCmd := ""

	// Auto-detect based on common patterns
	if strings.Contains(repoURL, "modelcontextprotocol/servers") {
		installCmd = "npx @modelcontextprotocol/server-" + id
		startCmd = installCmd
	} else if strings.Contains(repoURL, "python") || strings.Contains(repoURL, "pip") {
		installCmd = "pip install " + id
		startCmd = "python3 -m " + strings.ReplaceAll(id, "-", "_") + ".mcp"
	} else {
		installCmd = "npm install " + id
		startCmd = "npx " + id
	}

	if name == "" { name = id }

	_, err := s.pool.Exec(ctx,
		`INSERT INTO skill_manifests (id, name, description, repo_url, type, transport, install_cmd, start_cmd, verified)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false)
		 ON CONFLICT (id) DO UPDATE SET repo_url = $4, install_cmd = $7, start_cmd = $8`,
		id, name, "Custom skill from "+repoURL, repoURL, skillType, transport, installCmd, startCmd)
	if err != nil { return nil, err }

	m := &Manifest{ID: id, Name: name, RepoURL: repoURL, Type: skillType, Transport: transport,
		InstallCmd: installCmd, StartCmd: startCmd}
	return m, nil
}

// SearchMarketplace searches skills by name or tags.
func (s *MarketplaceStore) SearchMarketplace(ctx context.Context, query string) ([]Manifest, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, COALESCE(description,''), COALESCE(repo_url,''), COALESCE(license,''),
		        type, transport, COALESCE(install_cmd,''), COALESCE(start_cmd,''),
		        COALESCE(tags, ARRAY[]::TEXT[]), verified, COALESCE(stars,0)
		 FROM skill_manifests
		 WHERE name ILIKE '%' || $1 || '%' OR description ILIKE '%' || $1 || '%' OR $1 = ANY(tags)
		 ORDER BY verified DESC, stars DESC`, query)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []Manifest{}
	for rows.Next() {
		var m Manifest
		rows.Scan(&m.ID, &m.Name, &m.Description, &m.RepoURL, &m.License,
			&m.Type, &m.Transport, &m.InstallCmd, &m.StartCmd, &m.Tags, &m.Verified, &m.Stars)
		out = append(out, m)
	}
	return out, nil
}

// ExportManifest returns a JSON manifest for a skill (for sharing).
func (s *MarketplaceStore) ExportManifest(ctx context.Context, id string) (json.RawMessage, error) {
	var m Manifest
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(description,''), repo_url, COALESCE(license,''),
		        type, transport, install_cmd, start_cmd, COALESCE(tags, ARRAY[]::TEXT[]), verified
		 FROM skill_manifests WHERE id = $1`, id,
	).Scan(&m.ID, &m.Name, &m.Description, &m.RepoURL, &m.License,
		&m.Type, &m.Transport, &m.InstallCmd, &m.StartCmd, &m.Tags, &m.Verified)
	if err != nil { return nil, err }
	return json.Marshal(m)
}
