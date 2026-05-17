// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qorvenai/qorven/cmd/scaffold"
)

// ScaffoldAppTool creates a new Qorven app from the scaffold template.
// Output lands in appsDir/<name>/ with plugin/ and ui/ subtrees.
type ScaffoldAppTool struct {
	appsDir string // e.g. ~/.qorven/apps
}

func NewScaffoldAppTool(appsDir string) *ScaffoldAppTool {
	return &ScaffoldAppTool{appsDir: appsDir}
}

func (t *ScaffoldAppTool) Name() string { return "scaffold_app" }
func (t *ScaffoldAppTool) Description() string {
	return "Scaffold a new Qorven app from the standard template. Creates plugin/ (Go Wasm) and ui/ (Vite IIFE bundle) source trees in ~/.qorven/apps/<name>/. Returns the absolute path to the scaffold root."
}
func (t *ScaffoldAppTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "App identifier — lowercase letters, digits, underscores. e.g. crm_app",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "One-sentence description of what the app does.",
			},
			"plugin_only": map[string]any{
				"type":        "boolean",
				"description": "Only scaffold the Go plugin (no UI bundle). Default false.",
			},
			"ui_only": map[string]any{
				"type":        "boolean",
				"description": "Only scaffold the Vite UI bundle (no plugin). Default false.",
			},
		},
		"required": []string{"name"},
	}
}

func (t *ScaffoldAppTool) Execute(_ context.Context, args map[string]any) *Result {
	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name is required")
	}
	description, _ := args["description"].(string)
	pluginOnly, _ := args["plugin_only"].(bool)
	uiOnly, _ := args["ui_only"].(bool)

	targetDir := filepath.Join(t.appsDir, name)

	opts := scaffold.Options{
		Name:        name,
		Description: description,
		Plugin:      !uiOnly,
		UI:          !pluginOnly,
		Runtime:     scaffold.RuntimeGo,
		TargetDir:   targetDir,
	}

	written, err := scaffold.Render(opts)
	if err != nil {
		return ErrorResult(fmt.Sprintf("scaffold failed: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Scaffolded %d files at %s\n", len(written), targetDir)
	if opts.Plugin {
		fmt.Fprintf(&sb, "\nBuild the plugin:\n  cd %s/plugin && make build\n", targetDir)
	}
	if opts.UI {
		fmt.Fprintf(&sb, "\nBuild the UI bundle:\n  cd %s/ui && npm install && npm run build\n", targetDir)
	}
	fmt.Fprintf(&sb, "\nInstall when ready:\n  install_app(path=%q)\n", targetDir)

	return &Result{
		ForLLM:  sb.String(),
		ForUser: fmt.Sprintf("App scaffolded at `%s` (%d files)", targetDir, len(written)),
	}
}

// AppInstallFunc is a callback that installs an app from a manifest directory
// and returns (id, slug, displayName, error). Using a function avoids the
// import cycle that would arise from importing apps (which imports tools).
type AppInstallFunc func(ctx context.Context, manifestDir string) (id, slug, displayName string, err error)

// AppReloadFunc reloads an already-installed app by slug.
type AppReloadFunc func(ctx context.Context, slug string) error

// InstallAppTool installs a scaffolded app via callbacks provided by the gateway.
// The app directory must already contain a valid app.yaml and, if it declares a
// frontend bundle, the bundle must be built before calling this tool.
type InstallAppTool struct {
	install AppInstallFunc
	reload  AppReloadFunc
}

func NewInstallAppTool(install AppInstallFunc, reload AppReloadFunc) *InstallAppTool {
	return &InstallAppTool{install: install, reload: reload}
}

func (t *InstallAppTool) Name() string { return "install_app" }
func (t *InstallAppTool) Description() string {
	return "Install a Qorven app from disk. The directory must contain app.yaml. If the app declares a frontend bundle, run 'npm run build' in ui/ first. Use reload=true to hot-reload an already-installed app."
}
func (t *InstallAppTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the app directory (must contain app.yaml).",
			},
			"reload": map[string]any{
				"type":        "boolean",
				"description": "If the app is already installed, reload it (hot-reload tools + hooks + bundle). Default false.",
			},
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{"workspace", "agent", "team"},
				"description": "Who can access this connector. workspace=all agents (default), agent=only this agent, team=only agents in the specified team.",
			},
			"owner_agent_id": map[string]any{
				"type":        "string",
				"description": "Agent ID — required when scope=agent.",
			},
			"owner_team_id": map[string]any{
				"type":        "string",
				"description": "Team ID — required when scope=team.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *InstallAppTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required")
	}
	doReload, _ := args["reload"].(bool)
	scope, _ := args["scope"].(string)
	ownerAgentID, _ := args["owner_agent_id"].(string)
	ownerTeamID, _ := args["owner_team_id"].(string)

	// Validate scope.
	if scope != "" {
		switch scope {
		case "workspace", "agent", "team":
		default:
			return ErrorResult(fmt.Sprintf("invalid scope %q: must be workspace, agent, or team", scope))
		}
	}
	if scope == "agent" && ownerAgentID == "" {
		return ErrorResult("owner_agent_id is required when scope=agent")
	}
	if scope == "team" && ownerTeamID == "" {
		return ErrorResult("owner_team_id is required when scope=team")
	}

	// Verify app.yaml exists before attempting install.
	appYAML := filepath.Join(path, "app.yaml")
	if _, err := os.Stat(appYAML); err != nil {
		return ErrorResult(fmt.Sprintf("no app.yaml at %s — scaffold the app first", path))
	}

	// If scope params are provided, patch app.yaml to embed scope fields.
	// AppManager.Install reads the manifest including scope, so this is the
	// clean path: scope lives in app.yaml and is persisted via the normal flow.
	if scope != "" {
		if err := patchManifestScope(appYAML, scope, ownerAgentID, ownerTeamID); err != nil {
			return ErrorResult(fmt.Sprintf("failed to patch app.yaml with scope: %v", err))
		}
	}

	if doReload && t.reload != nil {
		// Read slug from manifest name (first non-comment line with slug:).
		if slug := readSlugFromManifest(path); slug != "" {
			if err := t.reload(ctx, slug); err == nil {
				return &Result{
					ForLLM:  fmt.Sprintf("App %q reloaded successfully", slug),
					ForUser: fmt.Sprintf("App `%s` reloaded", slug),
				}
			}
		}
		// Fall through to install if reload fails (not yet installed).
	}

	id, slug, displayName, err := t.install(ctx, path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("install failed: %v", err))
	}

	msg := fmt.Sprintf("App installed. id=%s slug=%s display_name=%q", id, slug, displayName)
	if scope != "" {
		msg += fmt.Sprintf(" scope=%s", scope)
	}
	return &Result{
		ForLLM:  msg,
		ForUser: fmt.Sprintf("App `%s` installed", slug),
	}
}

// AppUninstallFunc removes an installed app by slug.
// dropTables=true also runs the app's own down-migrations.
type AppUninstallFunc func(ctx context.Context, id string, dropTables bool) error

// AppGetIDBySlugFunc resolves an app ID from its slug (needed for uninstall which takes an ID).
type AppGetIDBySlugFunc func(ctx context.Context, slug string) (string, error)

// UninstallAppTool removes an installed Qorven app by slug.
type UninstallAppTool struct {
	getID     AppGetIDBySlugFunc
	uninstall AppUninstallFunc
}

func NewUninstallAppTool(getID AppGetIDBySlugFunc, uninstall AppUninstallFunc) *UninstallAppTool {
	return &UninstallAppTool{getID: getID, uninstall: uninstall}
}

func (t *UninstallAppTool) Name() string { return "uninstall_app" }
func (t *UninstallAppTool) Description() string {
	return "Uninstall a Qorven app by slug. Removes the DB row and unregisters all tools/hooks. Pass drop_tables=true to also drop the app's own database tables."
}
func (t *UninstallAppTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug": map[string]any{
				"type":        "string",
				"description": "The app slug (from app.yaml).",
			},
			"drop_tables": map[string]any{
				"type":        "boolean",
				"description": "Also drop any database tables the app created. Default false.",
			},
		},
		"required": []string{"slug"},
	}
}

func (t *UninstallAppTool) Execute(ctx context.Context, args map[string]any) *Result {
	slug, _ := args["slug"].(string)
	if slug == "" {
		return ErrorResult("slug is required")
	}
	dropTables, _ := args["drop_tables"].(bool)

	id, err := t.getID(ctx, slug)
	if err != nil {
		return ErrorResult(fmt.Sprintf("app %q not found: %v", slug, err))
	}
	if err := t.uninstall(ctx, id, dropTables); err != nil {
		return ErrorResult(fmt.Sprintf("uninstall failed: %v", err))
	}
	msg := fmt.Sprintf("App %q uninstalled", slug)
	if dropTables {
		msg += " (tables dropped)"
	}
	return &Result{ForLLM: msg, ForUser: msg}
}

// patchManifestScope rewrites scope-related fields in app.yaml.
// It removes any existing scope/owner_agent_id/owner_team_id lines then
// appends the new values so that AppManager.Install picks them up.
func patchManifestScope(appYAML, scope, ownerAgentID, ownerTeamID string) error {
	data, err := os.ReadFile(appYAML)
	if err != nil {
		return fmt.Errorf("read app.yaml: %w", err)
	}
	// Strip any existing scope lines to avoid duplicates.
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "scope:") ||
			strings.HasPrefix(trimmed, "owner_agent_id:") ||
			strings.HasPrefix(trimmed, "owner_team_id:") {
			continue
		}
		kept = append(kept, line)
	}
	// Remove trailing blank lines introduced by stripping.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	// Append new scope fields.
	kept = append(kept, fmt.Sprintf("scope: %s", scope))
	if ownerAgentID != "" {
		kept = append(kept, fmt.Sprintf("owner_agent_id: %s", ownerAgentID))
	}
	if ownerTeamID != "" {
		kept = append(kept, fmt.Sprintf("owner_team_id: %s", ownerTeamID))
	}
	kept = append(kept, "") // trailing newline
	return os.WriteFile(appYAML, []byte(strings.Join(kept, "\n")), 0644)
}

// readSlugFromManifest does a minimal parse of app.yaml to extract the slug
// without importing the full apps package.
func readSlugFromManifest(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "app.yaml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "slug:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "slug:"))
		}
	}
	return ""
}
