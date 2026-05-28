// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ProjectHookEvent is a lifecycle event that can trigger project hooks.
type ProjectHookEvent string

const (
	PHookFileCreated    ProjectHookEvent = "file_created"
	PHookFileChanged    ProjectHookEvent = "file_changed"
	PHookFileDeleted    ProjectHookEvent = "file_deleted"
	PHookBuildStarted   ProjectHookEvent = "build_started"
	PHookBuildCompleted ProjectHookEvent = "build_completed"
	PHookBuildFailed    ProjectHookEvent = "build_failed"
	PHookTestPassed     ProjectHookEvent = "test_passed"
	PHookTestFailed     ProjectHookEvent = "test_failed"
	PHookAgentCompleted ProjectHookEvent = "agent_task_completed"
)

// ProjectHookAction defines what a hook does when triggered.
type ProjectHookAction string

const (
	PHookRunCommand ProjectHookAction = "run_command"
	PHookRunAgent   ProjectHookAction = "run_agent"
	PHookNotify     ProjectHookAction = "notify"
)

// ProjectHook is a single hook definition from .qorven/hooks.yaml.
type ProjectHook struct {
	Event   ProjectHookEvent  `yaml:"event" json:"event"`
	Pattern string            `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Action  ProjectHookAction `yaml:"action" json:"action"`
	Command string            `yaml:"command,omitempty" json:"command,omitempty"`
	Agent   string            `yaml:"agent,omitempty" json:"agent,omitempty"`
	Prompt  string            `yaml:"prompt,omitempty" json:"prompt,omitempty"`
	Message string            `yaml:"message,omitempty" json:"message,omitempty"`
}

// ProjectHooksConfig is the top-level structure of .qorven/hooks.yaml.
type ProjectHooksConfig struct {
	Hooks []ProjectHook `yaml:"hooks" json:"hooks"`
}

// HookAgentRunner is the minimal interface the project hook registry needs.
// Satisfied by *agent.Loop.
type HookAgentRunner interface {
	Run(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) (*RunResult, error)
}

// ProjectHookRegistry manages project hooks and fires them on events.
type ProjectHookRegistry struct {
	mu          sync.RWMutex
	hooks       map[string][]ProjectHook // projectPath → hooks
	firing      sync.Map                 // key → bool (loop prevention)
	agentRunner HookAgentRunner
}

func NewProjectHookRegistry(runner HookAgentRunner) *ProjectHookRegistry {
	return &ProjectHookRegistry{
		hooks:       make(map[string][]ProjectHook),
		agentRunner: runner,
	}
}

// LoadHooks reads .qorven/hooks.yaml for a project.
func (hr *ProjectHookRegistry) LoadHooks(projectPath string) {
	hooksFile := filepath.Join(projectPath, ".qorven", "hooks.yaml")
	data, err := os.ReadFile(hooksFile)
	if err != nil {
		return
	}

	var cfg ProjectHooksConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		slog.Warn("project_hooks.load_failed", "path", hooksFile, "err", err)
		return
	}

	hr.mu.Lock()
	hr.hooks[projectPath] = cfg.Hooks
	hr.mu.Unlock()

	slog.Info("project_hooks.loaded", "project", projectPath, "count", len(cfg.Hooks))
}

// Fire triggers all matching hooks for an event.
func (hr *ProjectHookRegistry) Fire(ctx context.Context, projectPath string, event ProjectHookEvent, payload map[string]string) {
	key := projectPath + ":" + string(event)
	if _, loaded := hr.firing.LoadOrStore(key, true); loaded {
		return // loop prevention
	}
	defer hr.firing.Delete(key)

	hr.mu.RLock()
	hooks := hr.hooks[projectPath]
	hr.mu.RUnlock()

	if len(hooks) == 0 {
		hr.LoadHooks(projectPath)
		hr.mu.RLock()
		hooks = hr.hooks[projectPath]
		hr.mu.RUnlock()
	}

	for _, h := range hooks {
		if h.Event != event {
			continue
		}

		if h.Pattern != "" {
			filePath := payload["file"]
			if filePath == "" {
				continue
			}
			matched, _ := filepath.Match(h.Pattern, filepath.Base(filePath))
			if !matched {
				matched, _ = filepath.Match(h.Pattern, filePath)
			}
			if !matched {
				continue
			}
		}

		go hr.executeHook(ctx, projectPath, h, payload)
	}
}

func (hr *ProjectHookRegistry) executeHook(ctx context.Context, projectPath string, h ProjectHook, payload map[string]string) {
	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	switch h.Action {
	case PHookRunCommand:
		cmd := expandHookPayload(h.Command, payload)
		proc := exec.CommandContext(execCtx, "sh", "-c", cmd)
		proc.Dir = projectPath
		output, err := proc.CombinedOutput()
		if err != nil {
			slog.Warn("project_hooks.command_failed",
				"event", h.Event, "cmd", cmd, "err", err,
				"output", truncateStr(string(output), 500))
		} else {
			slog.Debug("project_hooks.command_ok", "event", h.Event, "cmd", cmd)
		}

	case PHookRunAgent:
		if hr.agentRunner == nil {
			slog.Warn("project_hooks.no_agent_runner", "event", h.Event)
			return
		}
		prompt := expandHookPayload(h.Prompt, payload)
		agentID := h.Agent
		if agentID == "" {
			agentID = "coder"
		}
		_, err := hr.agentRunner.Run(execCtx, RunRequest{
			AgentID:     agentID,
			UserMessage: prompt,
			Channel:     "hook",
			Stream:      false,
			NoPersist:   true,
		}, func(_ StreamEvent) {})
		if err != nil {
			slog.Warn("project_hooks.agent_failed", "event", h.Event, "agent", agentID, "err", err)
		}

	case PHookNotify:
		msg := expandHookPayload(h.Message, payload)
		slog.Info("project_hooks.notify", "event", h.Event, "message", msg)
	}
}

// FireFileEvent is a convenience function for file-related events.
func (hr *ProjectHookRegistry) FireFileEvent(ctx context.Context, projectPath string, event ProjectHookEvent, filePath string) {
	hr.Fire(ctx, projectPath, event, map[string]string{
		"file": filePath,
		"dir":  filepath.Dir(filePath),
		"ext":  filepath.Ext(filePath),
		"name": filepath.Base(filePath),
	})
}

// FireBuildEvent is a convenience for build lifecycle events.
func (hr *ProjectHookRegistry) FireBuildEvent(ctx context.Context, projectPath string, event ProjectHookEvent, detail string) {
	hr.Fire(ctx, projectPath, event, map[string]string{
		"detail": detail,
	})
}

// GetHooks returns the loaded hooks for a project.
func (hr *ProjectHookRegistry) GetHooks(projectPath string) []ProjectHook {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return hr.hooks[projectPath]
}

// SetHooks updates hooks for a project and persists to disk.
func (hr *ProjectHookRegistry) SetHooks(projectPath string, hooks []ProjectHook) error {
	cfg := ProjectHooksConfig{Hooks: hooks}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	hooksDir := filepath.Join(projectPath, ".qorven")
	os.MkdirAll(hooksDir, 0755)

	if err := os.WriteFile(filepath.Join(hooksDir, "hooks.yaml"), data, 0644); err != nil {
		return err
	}

	hr.mu.Lock()
	hr.hooks[projectPath] = hooks
	hr.mu.Unlock()

	return nil
}

func expandHookPayload(template string, payload map[string]string) string {
	result := template
	for k, v := range payload {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// DefaultProjectHooksTemplate returns a starter hooks.yaml.
func DefaultProjectHooksTemplate() string {
	return `# Qorven Project Hooks
# Hooks trigger actions on project lifecycle events.
# Available events: file_created, file_changed, file_deleted,
#   build_started, build_completed, build_failed,
#   test_passed, test_failed, agent_task_completed
# Available actions: run_command, run_agent, notify

hooks: []
  # Example: Auto-format Go files on change
  # - event: file_changed
  #   pattern: "*.go"
  #   action: run_command
  #   command: "gofmt -w {{file}}"

  # Example: Auto-fix TypeScript errors
  # - event: file_changed
  #   pattern: "*.tsx"
  #   action: run_agent
  #   agent: coder
  #   prompt: "Check {{file}} for TypeScript errors and fix them"

  # Example: Run tests after build
  # - event: build_completed
  #   action: run_command
  #   command: "npm test"
`
}
