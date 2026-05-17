// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Installer deploys a workspace template — creates agents, saves dashboard config.
type Installer struct {
	pool *pgxpool.Pool
}

func NewInstaller(pool *pgxpool.Pool) *Installer {
	return &Installer{pool: pool}
}

// Install deploys a template for a tenant.
func (inst *Installer) Install(ctx context.Context, tenantID, templateID string) (*InstalledWorkspace, error) {
	tmpl := findTemplate(templateID)
	if tmpl == nil {
		return nil, fmt.Errorf("template not found: %s", templateID)
	}

	slog.Info("template.install", "tenant", tenantID, "template", templateID, "agents", len(tmpl.Agents))

	// Create agents
	agentIDs := make(map[string]string) // key → id
	for _, spec := range tmpl.Agents {
		id, err := inst.createAgent(ctx, tenantID, spec)
		if err != nil {
			slog.Warn("template.agent_failed", "key", spec.Key, "error", err)
			continue
		}
		agentIDs[spec.Key] = id
	}

	// Set hierarchy (reports_to)
	for _, spec := range tmpl.Agents {
		if spec.ReportsTo != "" {
			parentID, ok := agentIDs[spec.ReportsTo]
			childID, ok2 := agentIDs[spec.Key]
			if ok && ok2 {
				inst.pool.Exec(ctx, `UPDATE agents SET manager_id = $1 WHERE id = $2`, parentID, childID)
			}
		}
	}

	// Save dashboard config
	dashJSON, _ := json.Marshal(tmpl.Dashboard)
	inst.pool.Exec(ctx,
		`INSERT INTO workspace_dashboards (tenant_id, template_id, name, config) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, template_id) DO UPDATE SET config = $4, updated_at = now()`,
		tenantID, templateID, tmpl.Name, dashJSON)

	return &InstalledWorkspace{
		TemplateID: templateID,
		Name:       tmpl.Name,
		AgentCount: len(agentIDs),
		AgentIDs:   agentIDs,
	}, nil
}

func (inst *Installer) createAgent(ctx context.Context, tenantID string, spec AgentSpec) (string, error) {
	var id string
	err := inst.pool.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, agent_key, display_name, role, system_prompt, model, status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'active')
		 ON CONFLICT (tenant_id, agent_key) DO UPDATE SET display_name = $3, system_prompt = $5
		 RETURNING id`,
		tenantID, spec.Key, spec.Name, spec.Role, spec.SystemPrompt, spec.Model,
	).Scan(&id)
	return id, err
}

// GetDashboard returns the saved dashboard config for a template.
func (inst *Installer) GetDashboard(ctx context.Context, tenantID, templateID string) (*DashboardSpec, error) {
	configJSON := []byte{}
	err := inst.pool.QueryRow(ctx,
		`SELECT config FROM workspace_dashboards WHERE tenant_id = $1 AND template_id = $2`,
		tenantID, templateID).Scan(&configJSON)
	if err != nil { return nil, err }
	var dash DashboardSpec
	json.Unmarshal(configJSON, &dash)
	return &dash, nil
}

// ListInstalled returns all installed templates for a tenant.
func (inst *Installer) ListInstalled(ctx context.Context, tenantID string) ([]InstalledWorkspace, error) {
	rows, err := inst.pool.Query(ctx,
		`SELECT template_id, name FROM workspace_dashboards WHERE tenant_id = $1`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	result := []InstalledWorkspace{}
	for rows.Next() {
		var w InstalledWorkspace
		rows.Scan(&w.TemplateID, &w.Name)
		result = append(result, w)
	}
	return result, nil
}

type InstalledWorkspace struct {
	TemplateID string            `json:"template_id"`
	Name       string            `json:"name"`
	AgentCount int               `json:"agent_count"`
	AgentIDs   map[string]string `json:"agent_ids,omitempty"`
}

func findTemplate(id string) *WorkspaceTemplate {
	for _, t := range Catalog() {
		if t.ID == id { return &t }
	}
	return nil
}

// InstallCustom installs a dynamically generated template (from AI workspace builder).
// Same as Install but takes a template pointer directly instead of looking up by ID.
func (inst *Installer) InstallCustom(ctx context.Context, tenantID string, tmpl *WorkspaceTemplate) (*InstalledWorkspace, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("template is nil")
	}
	slog.Info("template.install_custom", "tenant", tenantID, "name", tmpl.Name, "agents", len(tmpl.Agents))

	agentIDs := make(map[string]string)
	for _, spec := range tmpl.Agents {
		id, err := inst.createAgent(ctx, tenantID, spec)
		if err != nil {
			slog.Warn("template.custom_agent_failed", "key", spec.Key, "error", err)
			continue
		}
		agentIDs[spec.Key] = id
	}

	// Set hierarchy
	for _, spec := range tmpl.Agents {
		if spec.ReportsTo != "" {
			parentID, ok := agentIDs[spec.ReportsTo]
			childID, ok2 := agentIDs[spec.Key]
			if ok && ok2 {
				inst.pool.Exec(ctx, `UPDATE agents SET manager_id = $1 WHERE id = $2`, parentID, childID)
			}
		}
	}

	// Save dashboard
	dashJSON, _ := json.Marshal(tmpl.Dashboard)
	inst.pool.Exec(ctx,
		`INSERT INTO workspace_dashboards (tenant_id, template_id, name, config) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, template_id) DO UPDATE SET config = $4, updated_at = now()`,
		tenantID, tmpl.ID, tmpl.Name, dashJSON)

	return &InstalledWorkspace{
		TemplateID: tmpl.ID,
		Name:       tmpl.Name,
		AgentCount: len(agentIDs),
		AgentIDs:   agentIDs,
	}, nil
}
