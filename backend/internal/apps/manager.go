// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/plugin"
	"github.com/qorvenai/qorven/internal/tools"
)

// Sentinel errors for RunTool — used by the HTTP handler for status mapping.
var (
	ErrAppNotLoaded = errors.New("app not loaded or disabled")
	ErrToolNotFound = errors.New("tool not found")
)

const maxAppToolOutput = 100_000 // 100 KB

// limitedBuffer caps output at maxAppToolOutput bytes to prevent OOM.
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	if lb.truncated {
		return len(p), nil
	}
	remaining := lb.max - lb.buf.Len()
	if remaining <= 0 {
		lb.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		lb.buf.Write(p[:remaining])
		lb.truncated = true
		return len(p), nil
	}
	return lb.buf.Write(p)
}

func (lb *limitedBuffer) Bytes() []byte { return lb.buf.Bytes() }

// appToolEnv builds a minimal environment for app tool subprocesses.
// Starts from a safe base rather than os.Environ() to prevent secret leakage.
func appToolEnv(dir string, extras ...string) []string {
	base := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + dir,
		"LANG=en_US.UTF-8",
		"TERM=dumb",
	}
	return append(base, extras...)
}

// toolRegistrar is the subset of tools.Registry used by AppManager.
// Using an interface allows unit tests to inject a fake without a real pool.
type toolRegistrar interface {
	Register(tools.Tool)
	Unregister(string)
}

// AppManager handles the full lifecycle of installed apps: install, enable,
// disable, reload, uninstall, and serving frontend bundle paths.
type AppManager struct {
	store      *AppStore
	toolReg    toolRegistrar
	pluginMgr  *plugin.Manager
	pool       *pgxpool.Pool
	tenantID   string
	dsn        string // extracted from pool at construction for env injection
	credLookup func(tenantID, slug string) string

	mu     sync.RWMutex
	loaded map[string]*loadedApp // slug → live state
}

type loadedApp struct {
	app       App
	manifest  Manifest
	toolNames []string // registered tool names, for clean unregistration
}

// NewAppManager creates an AppManager. tenantID is the single-tenant ID
// ("00000000-0000-0000-0000-000000000001") used for all DB operations.
// credLookup is called to retrieve a credential key for a connector app at
// tool registration and execution time; it may be nil (no injection).
func NewAppManager(store *AppStore, toolReg *tools.Registry, pluginMgr *plugin.Manager, pool *pgxpool.Pool, tenantID string, credLookup func(tenantID, slug string) string) *AppManager {
	var dsn string
	if pool != nil {
		dsn = pool.Config().ConnString()
	}
	return &AppManager{
		store:      store,
		toolReg:    toolReg, // *tools.Registry satisfies toolRegistrar interface
		pluginMgr:  pluginMgr,
		pool:       pool,
		tenantID:   tenantID,
		dsn:        dsn,
		credLookup: credLookup,
		loaded:     make(map[string]*loadedApp),
	}
}

// LoadAll loads enabled apps from the DB on gateway boot.
// agentID and teamID filter which scoped apps are loaded:
//   - workspace-scoped apps are always loaded
//   - agent-scoped apps are loaded only when agentID matches
//   - team-scoped apps are loaded only when teamID matches
//
// Pass ("", "") at boot to load only workspace-scoped apps.
func (m *AppManager) LoadAll(ctx context.Context, agentID, teamID string) error {
	apps, err := m.store.ListScoped(ctx, m.tenantID, agentID, teamID)
	if err != nil {
		return fmt.Errorf("apps.load_all: list: %w", err)
	}
	for _, a := range apps {
		if !a.Enabled {
			continue
		}
		manifest, err := LoadManifest(a.InstallPath)
		if err != nil {
			slog.Warn("app.load_all.skip", "slug", a.Slug, "err", err)
			continue
		}
		if err := CheckRequiresEnv(manifest); err != nil {
			slog.Info("app.load_all.skip_env", "slug", a.Slug, "err", err)
			continue
		}
		toolNames := m.registerTools(a, manifest)
		m.registerHooks(a, manifest)
		m.mu.Lock()
		m.loaded[a.Slug] = &loadedApp{app: a, manifest: manifest, toolNames: toolNames}
		m.mu.Unlock()
		slog.Info("app.loaded", "slug", a.Slug, "version", manifest.Version, "tools", len(toolNames))
	}
	return nil
}

// Install parses app.yaml from manifestDir, runs migrations, registers tools/hooks,
// and creates the DB row. Returns the created App.
func (m *AppManager) Install(ctx context.Context, manifestDir string) (*App, error) {
	absDir, err := filepath.Abs(manifestDir)
	if err != nil {
		return nil, err
	}
	manifest, err := LoadManifest(absDir)
	if err != nil {
		return nil, err
	}
	// Run app-owned migrations if db_write permission granted.
	if HasPermission(manifest, "db_write") {
		migrDir := filepath.Join(absDir, manifest.MigrationsDir)
		if err := RunAppMigrations(ctx, m.pool, manifest.Slug, m.tenantID, migrDir); err != nil {
			return nil, fmt.Errorf("app migrations: %w", err)
		}
	}

	scope := manifest.Scope
	if scope == "" {
		scope = "workspace"
	}
	created, err := m.store.Create(ctx, App{
		TenantID:     m.tenantID,
		Slug:         manifest.Slug,
		DisplayName:  manifest.DisplayName,
		Description:  manifest.Description,
		Version:      manifest.Version,
		Author:       manifest.Author,
		IconURL:      manifest.IconURL,
		InstallPath:  absDir,
		Enabled:      true,
		Config:       map[string]any{},
		Scope:        scope,
		OwnerAgentID: manifest.OwnerAgentID,
		OwnerTeamID:  manifest.OwnerTeamID,
	})
	if err != nil {
		return nil, fmt.Errorf("create app row: %w", err)
	}

	toolNames := m.registerTools(created, manifest)
	m.registerHooks(created, manifest)

	m.mu.Lock()
	m.loaded[created.Slug] = &loadedApp{app: created, manifest: manifest, toolNames: toolNames}
	m.mu.Unlock()

	if m.pluginMgr != nil {
		m.pluginMgr.FireHook(ctx, plugin.HookAppInstalled, map[string]any{
			"app_id": created.ID,
			"slug":   created.Slug,
		})
	}
	slog.Info("app.installed", "slug", created.Slug, "path", absDir)
	return &created, nil
}

// Reload re-reads the manifest from the existing install_path and re-registers
// tools/hooks. Used after an app is updated on disk.
func (m *AppManager) Reload(ctx context.Context, slug string) error {
	a, err := m.store.GetBySlug(ctx, m.tenantID, slug)
	if err != nil {
		return fmt.Errorf("app not found: %w", err)
	}
	m.unload(slug)
	manifest, err := LoadManifest(a.InstallPath)
	if err != nil {
		return err
	}
	toolNames := m.registerTools(a, manifest)
	m.registerHooks(a, manifest)
	m.mu.Lock()
	m.loaded[slug] = &loadedApp{app: a, manifest: manifest, toolNames: toolNames}
	m.mu.Unlock()
	slog.Info("app.reloaded", "slug", slug)
	return nil
}

// Enable re-registers tools/hooks for a disabled app.
func (m *AppManager) Enable(ctx context.Context, id string) error {
	a, err := m.store.Get(ctx, m.tenantID, id)
	if err != nil {
		return err
	}
	if err := m.store.SetEnabled(ctx, m.tenantID, id, true); err != nil {
		return err
	}
	manifest, err := LoadManifest(a.InstallPath)
	if err != nil {
		return err
	}
	toolNames := m.registerTools(a, manifest)
	m.registerHooks(a, manifest)
	m.mu.Lock()
	m.loaded[a.Slug] = &loadedApp{app: a, manifest: manifest, toolNames: toolNames}
	m.mu.Unlock()
	return nil
}

// Disable unregisters tools/hooks and marks the app disabled in DB.
func (m *AppManager) Disable(ctx context.Context, id string) error {
	a, err := m.store.Get(ctx, m.tenantID, id)
	if err != nil {
		return err
	}
	m.unload(a.Slug)
	return m.store.SetEnabled(ctx, m.tenantID, id, false)
}

// Uninstall disables the app and removes the DB row. App-owned tables are
// NOT dropped unless dropTables is true.
func (m *AppManager) Uninstall(ctx context.Context, id string, dropTables bool) error {
	a, err := m.store.Get(ctx, m.tenantID, id)
	if err != nil {
		return err
	}
	m.unload(a.Slug)

	if dropTables {
		// Best-effort: drop tables whose migrations we have on record.
		m.dropAppTables(ctx, a)
	}

	if m.pluginMgr != nil {
		m.pluginMgr.FireHook(ctx, plugin.HookAppUninstalled, map[string]any{
			"app_id": a.ID, "slug": a.Slug,
		})
	}
	return m.store.Delete(ctx, m.tenantID, id)
}

// BundlePath returns the absolute path to the app's frontend bundle.js.
// Returns ("", false) if the app is not loaded or has no bundle.
func (m *AppManager) BundlePath(slug string) (string, bool) {
	m.mu.RLock()
	la, ok := m.loaded[slug]
	m.mu.RUnlock()
	if !ok || !la.app.Enabled {
		return "", false
	}
	bundleRel := la.manifest.Frontend.Bundle
	if bundleRel == "" {
		bundleRel = "frontend/bundle.js"
	}
	path := filepath.Join(la.app.InstallPath, bundleRel)
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// FrontendManifests returns the frontend manifest for every loaded enabled app.
func (m *AppManager) FrontendManifests() []AppFrontendEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var entries []AppFrontendEntry
	for _, la := range m.loaded {
		if !la.app.Enabled {
			continue
		}
		entries = append(entries, AppFrontendEntry{
			AppID:     la.app.ID,
			Slug:      la.app.Slug,
			BundleURL: "/app-assets/" + la.app.Slug + "/bundle.js",
			Manifest:  la.manifest.Frontend,
		})
	}
	return entries
}

// Store returns the underlying AppStore for direct CRUD from gateway handlers.
func (m *AppManager) Store() *AppStore { return m.store }

// Reset clears the in-memory loaded map (called after factory reset so
// the next LoadAll starts from a clean DB).
func (m *AppManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loaded = make(map[string]*loadedApp)
}

// --- internal helpers ---

// unload removes tools/hooks for slug from the live registries.
func (m *AppManager) unload(slug string) {
	m.mu.Lock()
	la, ok := m.loaded[slug]
	if ok {
		delete(m.loaded, slug)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	if m.toolReg != nil {
		for _, name := range la.toolNames {
			m.toolReg.Unregister(name)
		}
	}
	if m.pluginMgr != nil {
		m.pluginMgr.UnregisterByTag(slug)
	}
}

// registerTools wires all tools declared in the manifest into toolReg.
// Returns the list of registered tool names for later unregistration.
func (m *AppManager) registerTools(a App, manifest Manifest) []string {
	if m.toolReg == nil || !HasPermission(manifest, "tool_register") {
		return nil
	}
	var names []string
	for _, td := range manifest.Tools {
		if td.Name == "" || td.Command == "" {
			continue
		}
		dir := a.InstallPath
		cmd := td.Command
		params := td.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		timeoutSec := td.Timeout
		if timeoutSec <= 0 {
			timeoutSec = 30
		}
		dsn := m.dsn
		t := &appTool{
			name:        td.Name,
			description: td.Description,
			parameters:  params,
			run: func(ctx context.Context, args map[string]any) *tools.Result {
				tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
				defer cancel()

				sanitized := sanitizeCmd(cmd)
				c := exec.CommandContext(tctx, "sh", "-c", sanitized)
				c.Dir = dir
				argsJSON, _ := json.Marshal(args)
				c.Stdin = strings.NewReader(string(argsJSON))
				c.Env = appToolEnv(dir,
					"QORVEN_APP_SLUG="+manifest.Slug,
					"QORVEN_TENANT_ID="+a.TenantID,
					"QORVEN_APP_ID="+a.ID,
					"QORVEN_AGENT_ID="+tools.AgentIDFromCtx(ctx),
					"QORVEN_DB_DSN="+dsn,
				)
				if m.credLookup != nil {
					if key := m.credLookup(a.TenantID, manifest.Slug); key != "" {
						c.Env = append(c.Env, "CONNECTOR_"+strings.ToUpper(strings.ReplaceAll(manifest.Slug, "-", "_"))+"_KEY="+key)
					}
				}
				// Put the child in its own process group so that a
				// context timeout kills all descendants (sh + sleep, etc.)
				// not just the sh process itself.
				setProcGroup(c)
				stdout := &limitedBuffer{max: maxAppToolOutput}
				stderr := &limitedBuffer{max: maxAppToolOutput}
				c.Stdout = stdout
				c.Stderr = stderr
				if err := c.Start(); err != nil {
					return tools.ErrorResult(err.Error())
				}
				// Watch for context cancellation and kill the whole group.
				done := make(chan struct{})
				go func() {
					select {
					case <-tctx.Done():
						if c.Process != nil {
							killGroup(c)
						}
					case <-done:
					}
				}()
				waitErr := c.Wait()
				close(done)
				if waitErr != nil {
					errOut := string(stderr.Bytes())
					if errOut == "" {
						errOut = waitErr.Error()
					}
					return tools.ErrorResult(errOut)
				}
				return parseToolOutput(stdout.Bytes())
			},
		}
		m.toolReg.Register(t)
		names = append(names, td.Name)
	}
	return names
}

const qorvenJSONHeader = "#!qorven:json\n"

// parseToolOutput checks for the #!qorven:json structured output protocol.
// If the output starts with the header, the remainder is parsed as JSON.
// Falls back to tools.TextResult on missing header or parse failure.
func parseToolOutput(out []byte) *tools.Result {
	raw := string(out)
	if !strings.HasPrefix(raw, qorvenJSONHeader) {
		return tools.TextResult(raw)
	}
	jsonPart := raw[len(qorvenJSONHeader):]
	var payload struct {
		Text   string        `json:"text"`
		User   string        `json:"user"`
		Widget *tools.Widget `json:"widget"`
	}
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		// Malformed JSON after header — fall back to raw text (not error)
		return tools.TextResult(raw)
	}
	r := &tools.Result{
		ForLLM:  payload.Text,
		ForUser: payload.User,
		Widget:  payload.Widget,
	}
	if r.ForUser == "" {
		r.ForUser = r.ForLLM
	}
	return r
}

// registerHooks wires all hooks declared in the manifest into pluginMgr.
func (m *AppManager) registerHooks(a App, manifest Manifest) {
	if m.pluginMgr == nil || !HasPermission(manifest, "hook_register") {
		return
	}
	for _, hd := range manifest.Hooks {
		if hd.Command == "" {
			continue
		}
		dir := a.InstallPath
		cmd := hd.Command
		slug := manifest.Slug
		event := plugin.HookEvent(hd.Event)
		m.pluginMgr.Context().RegisterTaggedHook(slug, event, func(ctx context.Context, data map[string]any) error {
			sanitized := sanitizeCmd(cmd)
			c := exec.CommandContext(ctx, "sh", "-c", sanitized)
			c.Dir = dir
			payload, _ := json.Marshal(data)
			c.Stdin = strings.NewReader(string(payload))
			c.Env = appToolEnv(dir,
				"QORVEN_APP_SLUG="+slug,
				"QORVEN_TENANT_ID="+a.TenantID,
			)
			out, err := c.CombinedOutput()
			if err != nil {
				slog.Warn("app.hook.error", "slug", slug, "event", event, "err", err, "out", string(out))
			}
			return nil // hooks never fail the pipeline
		})
	}
}

// dropAppTables removes app-owned tables by running down migrations.
func (m *AppManager) dropAppTables(ctx context.Context, a App) {
	manifest, err := LoadManifest(a.InstallPath)
	if err != nil {
		return
	}
	migrDir := filepath.Join(a.InstallPath, manifest.MigrationsDir)
	entries, err := os.ReadDir(migrDir)
	if err != nil {
		return
	}
	// Run *.down.sql in reverse numeric order.
	type downMig struct {
		version int
		path    string
	}
	var downs []downMig
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".down.sql") {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		if len(parts) == 0 {
			continue
		}
		var v int
		fmt.Sscanf(parts[0], "%d", &v)
		downs = append(downs, downMig{v, filepath.Join(migrDir, e.Name())})
	}
	// Sort descending.
	for i, j := 0, len(downs)-1; i < j; i, j = i+1, j-1 {
		downs[i], downs[j] = downs[j], downs[i]
	}
	for _, d := range downs {
		sql, err := os.ReadFile(d.path)
		if err != nil {
			continue
		}
		if _, err := m.pool.Exec(ctx, string(sql)); err != nil {
			slog.Warn("app.drop_tables.down_failed", "slug", a.Slug, "version", d.version, "err", err)
		}
	}
	// Clean migration tracking rows.
	m.pool.Exec(ctx,
		`DELETE FROM app_schema_migrations WHERE app_slug=$1 AND tenant_id=$2`,
		a.Slug, a.TenantID)
}

// RunTool executes a named tool for a loaded app and returns its Result.
// Returns an error if the app is not loaded/enabled or the tool is not found.
func (m *AppManager) RunTool(ctx context.Context, slug, toolName string, args map[string]any) (*tools.Result, error) {
	m.mu.RLock()
	la, ok := m.loaded[slug]
	m.mu.RUnlock()
	if !ok || !la.app.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrAppNotLoaded, slug)
	}

	var td *ToolDef
	for i := range la.manifest.Tools {
		if la.manifest.Tools[i].Name == toolName {
			td = &la.manifest.Tools[i]
			break
		}
	}
	if td == nil {
		return nil, fmt.Errorf("%w: %s in app %s", ErrToolNotFound, toolName, slug)
	}

	timeoutSec := td.Timeout
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	sanitized := sanitizeCmd(td.Command)
	c := exec.CommandContext(tctx, "sh", "-c", sanitized)
	c.Dir = la.app.InstallPath

	stdout := &limitedBuffer{max: maxAppToolOutput}
	stderr := &limitedBuffer{max: maxAppToolOutput}
	c.Stdout = stdout
	c.Stderr = stderr

	argsJSON, _ := json.Marshal(args)
	c.Stdin = strings.NewReader(string(argsJSON))
	c.Env = appToolEnv(la.app.InstallPath,
		"QORVEN_APP_SLUG="+la.app.Slug,
		"QORVEN_TENANT_ID="+la.app.TenantID,
		"QORVEN_APP_ID="+la.app.ID,
		"QORVEN_AGENT_ID="+tools.AgentIDFromCtx(ctx),
		"QORVEN_DB_DSN="+m.dsn,
	)
	if m.credLookup != nil {
		if key := m.credLookup(la.app.TenantID, la.manifest.Slug); key != "" {
			c.Env = append(c.Env, "CONNECTOR_"+strings.ToUpper(strings.ReplaceAll(la.manifest.Slug, "-", "_"))+"_KEY="+key)
		}
	}

	// Kill process group on timeout (same pattern as registerTools)
	setProcGroup(c)
	if err := c.Start(); err != nil {
		return tools.ErrorResult(err.Error()), nil
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-tctx.Done():
			if c.Process != nil {
				killGroup(c)
			}
		case <-done:
		}
	}()
	err := c.Wait()
	close(done)

	if err != nil {
		msg := string(stderr.Bytes())
		if msg == "" {
			msg = err.Error()
		}
		return tools.ErrorResult(msg), nil
	}
	return parseToolOutput(stdout.Bytes()), nil
}

// sanitizeCmd strips characters that aren't safe in shell command strings.
// Same allowlist as dirPlugin in plugin.go.
func sanitizeCmd(cmd string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == '/' || r == ' ' {
			return r
		}
		return -1
	}, cmd)
}

// appTool wraps a shell command as a tools.Tool.
type appTool struct {
	name, description string
	parameters        map[string]any
	run               func(ctx context.Context, args map[string]any) *tools.Result
}

func (t *appTool) Name() string                                            { return t.name }
func (t *appTool) Description() string                                     { return t.description }
func (t *appTool) Parameters() map[string]any                              { return t.parameters }
func (t *appTool) Execute(ctx context.Context, args map[string]any) *tools.Result { return t.run(ctx, args) }
