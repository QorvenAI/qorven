// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Plugin interface {
	Name() string
	Version() string
	Register(ctx *Context) error
}

type Context struct {
	mu       sync.Mutex
	tools    []ToolRegistration
	hooks    []HookRegistration
	commands []CommandRegistration
}

type ToolRegistration struct {
	Name, Description string
	Parameters        map[string]any
	Handler           func(ctx context.Context, args map[string]any) (string, error)
}

type HookRegistration struct {
	Event   HookEvent
	Tag     string // app slug for app-registered hooks; empty for core plugins
	Handler func(ctx context.Context, data map[string]any) error
}

type HookEvent string

const (
	HookPreToolCall    HookEvent = "pre_tool_call"
	HookPostToolCall   HookEvent = "post_tool_call"
	HookPreLLMCall     HookEvent = "pre_llm_call"
	HookPostLLMCall    HookEvent = "post_llm_call"
	HookSessionStart   HookEvent = "session_start"
	HookSessionEnd     HookEvent = "session_end"
	HookMessageReceive HookEvent = "message_receive"
	HookMessageSend    HookEvent = "message_send"
	HookSkillCreated   HookEvent = "skill_created"
	HookMemorySaved    HookEvent = "memory_saved"
	// App platform hooks
	HookAppInstalled    HookEvent = "app_installed"
	HookAppUninstalled  HookEvent = "app_uninstalled"
	HookDraftTransition HookEvent = "draft_transition"
)

type CommandRegistration struct {
	Name, Description string
	Handler           func(args string) string
}

func (c *Context) RegisterTool(name, desc string, params map[string]any, handler func(ctx context.Context, args map[string]any) (string, error)) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.tools = append(c.tools, ToolRegistration{Name: name, Description: desc, Parameters: params, Handler: handler})
}

func (c *Context) RegisterHook(event HookEvent, handler func(ctx context.Context, data map[string]any) error) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.hooks = append(c.hooks, HookRegistration{Event: event, Handler: handler})
}

// RegisterTaggedHook registers a hook with an identifying tag (e.g. app slug).
// Tagged hooks can be bulk-removed by calling Manager.UnregisterByTag.
func (c *Context) RegisterTaggedHook(tag string, event HookEvent, handler func(ctx context.Context, data map[string]any) error) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.hooks = append(c.hooks, HookRegistration{Event: event, Tag: tag, Handler: handler})
}

func (c *Context) RegisterCommand(name, desc string, handler func(args string) string) {
	c.mu.Lock(); defer c.mu.Unlock()
	c.commands = append(c.commands, CommandRegistration{Name: name, Description: desc, Handler: handler})
}

func (c *Context) Tools() []ToolRegistration       { return c.tools }
func (c *Context) Hooks() []HookRegistration       { return c.hooks }
func (c *Context) Commands() []CommandRegistration { return c.commands }

type Manager struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	ctx     *Context
}

func NewManager() *Manager {
	return &Manager{plugins: make(map[string]Plugin), ctx: &Context{}}
}

func (m *Manager) Register(p Plugin) error {
	m.mu.Lock(); defer m.mu.Unlock()
	if _, exists := m.plugins[p.Name()]; exists {
		return fmt.Errorf("plugin %q already registered", p.Name())
	}
	if err := p.Register(m.ctx); err != nil {
		return fmt.Errorf("plugin %q register: %w", p.Name(), err)
	}
	m.plugins[p.Name()] = p
	slog.Info("plugin registered", "name", p.Name(), "version", p.Version())
	return nil
}

func (m *Manager) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) { return nil }
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() { continue }
		if err := m.loadPlugin(filepath.Join(dir, entry.Name())); err != nil {
			slog.Warn("plugin load failed", "dir", entry.Name(), "error", err)
		}
	}
	return nil
}

// PluginManifest is parsed from plugin.yaml using real YAML parser.
type PluginManifest struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Description string   `yaml:"description"`
	Author      string   `yaml:"author"`
	RequiresEnv []string `yaml:"requires_env"`
}

type ToolDef struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Command     string         `yaml:"command"`
	Parameters  map[string]any `yaml:"parameters"`
}

func (m *Manager) loadPlugin(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return fmt.Errorf("no plugin.yaml in %s", filepath.Base(dir))
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("invalid plugin.yaml: %w", err)
	}
	if manifest.Name == "" { manifest.Name = filepath.Base(dir) }
	if manifest.Version == "" { manifest.Version = "0.0.0" }

	for _, env := range manifest.RequiresEnv {
		if os.Getenv(env) == "" {
			slog.Info("plugin skipped (missing env)", "name", manifest.Name, "env", env)
			return nil
		}
	}

	dp := &dirPlugin{name: manifest.Name, version: manifest.Version, dir: dir}

	if toolData, err := os.ReadFile(filepath.Join(dir, "tools.yaml")); err == nil {
		tools := []ToolDef{}
		yaml.Unmarshal(toolData, &tools)
		dp.toolDefs = tools
	}

	return m.Register(dp)
}

type dirPlugin struct {
	name, version, dir string
	toolDefs           []ToolDef
}

func (p *dirPlugin) Name() string    { return p.name }
func (p *dirPlugin) Version() string { return p.version }
func (p *dirPlugin) Register(ctx *Context) error {
	for _, t := range p.toolDefs {
		dir, cmd := p.dir, t.Command
		ctx.RegisterTool(t.Name, t.Description, t.Parameters, func(execCtx context.Context, args map[string]any) (string, error) {
			if cmd == "" { return "", fmt.Errorf("no command") }
			// Sanitize: only allow alphanumeric, dash, underscore, dot, slash in command
			sanitized := strings.Map(func(r rune) rune {
				if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
					r == '-' || r == '_' || r == '.' || r == '/' || r == ' ' {
					return r
				}
				return -1
			}, cmd)
			c := exec.CommandContext(execCtx, "sh", "-c", sanitized)
			c.Dir = dir
			argsJSON, _ := json.Marshal(args)
			c.Stdin = strings.NewReader(string(argsJSON))
			out, err := c.CombinedOutput()
			if err != nil { return string(out), err }
			return string(out), nil
		})
	}
	slog.Info("plugin loaded", "name", p.name, "version", p.version, "tools", len(p.toolDefs))
	return nil
}

// UnregisterByTag removes all hooks whose Tag matches tag.
// Used by AppManager.Disable to cleanly unload an app's hooks.
func (m *Manager) UnregisterByTag(tag string) {
	if tag == "" {
		return
	}
	m.ctx.mu.Lock(); defer m.ctx.mu.Unlock()
	filtered := m.ctx.hooks[:0]
	for _, h := range m.ctx.hooks {
		if h.Tag != tag {
			filtered = append(filtered, h)
		}
	}
	m.ctx.hooks = filtered
}

func (m *Manager) Context() *Context { return m.ctx }
func (m *Manager) List() []string {
	m.mu.RLock(); defer m.mu.RUnlock()
	names := make([]string, 0, len(m.plugins))
	for name := range m.plugins { names = append(names, name) }
	return names
}

func (m *Manager) FireHook(ctx context.Context, event HookEvent, data map[string]any) {
	for _, h := range m.ctx.Hooks() {
		if h.Event == event {
			if err := h.Handler(ctx, data); err != nil {
				slog.Warn("plugin hook error", "event", event, "error", err)
			}
		}
	}
}
