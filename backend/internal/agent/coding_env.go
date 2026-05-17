// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// CodingEnvironment gives an agent an isolated workspace to build full applications.
// Inspired by Google Jitro's goal-driven development.
//
// The agent can:
//   - Create a project from scratch in an isolated directory
//   - Run commands (build, test, lint) inside the workspace
//   - Present results as a diff/summary
//   - Persist the workspace for future iterations

type CodingEnvironment struct {
	BaseDir   string // e.g., /tmp/qorven-projects
	AgentID   string
	ProjectID string
	WorkDir   string // computed: BaseDir/AgentID/ProjectID
}

// NewCodingEnv creates or opens a coding environment for an agent+project.
func NewCodingEnv(baseDir, agentID, projectID string) *CodingEnvironment {
	if baseDir == "" { baseDir = "/tmp/qorven-projects" }
	if projectID == "" { projectID = fmt.Sprintf("proj_%d", time.Now().Unix()) }

	workDir := filepath.Join(baseDir, agentID[:8], projectID)
	os.MkdirAll(workDir, 0755)

	return &CodingEnvironment{
		BaseDir:   baseDir,
		AgentID:   agentID,
		ProjectID: projectID,
		WorkDir:   workDir,
	}
}

// WriteFile creates or overwrites a file in the project.
func (ce *CodingEnvironment) WriteFile(path, content string) error {
	full := filepath.Join(ce.WorkDir, path)
	os.MkdirAll(filepath.Dir(full), 0755)
	return os.WriteFile(full, []byte(content), 0644)
}

// ReadFile reads a file from the project.
func (ce *CodingEnvironment) ReadFile(path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(ce.WorkDir, path))
	return string(data), err
}

// Run executes a command in the project directory.
func (ce *CodingEnvironment) Run(ctx context.Context, command string) (string, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = ce.WorkDir
	cmd.Env = append(os.Environ(), "PROJECT_DIR="+ce.WorkDir)

	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}
	return string(out), exitCode, err
}

// ListFiles returns all files in the project.
func (ce *CodingEnvironment) ListFiles() []string {
	var files []string
	filepath.Walk(ce.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return nil }
		rel, _ := filepath.Rel(ce.WorkDir, path)
		if strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, "node_modules/") { return nil }
		files = append(files, rel)
		return nil
	})
	return files
}

// Diff returns a summary of what was created/changed.
func (ce *CodingEnvironment) Diff() string {
	files := ce.ListFiles()
	if len(files) == 0 { return "Empty project" }

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project: %s (%d files)\n\n", ce.ProjectID, len(files)))
	for _, f := range files {
		info, _ := os.Stat(filepath.Join(ce.WorkDir, f))
		if info != nil {
			sb.WriteString(fmt.Sprintf("  %s (%d bytes)\n", f, info.Size()))
		}
	}
	return sb.String()
}

// InitGit initializes a git repo in the project.
func (ce *CodingEnvironment) InitGit() error {
	_, _, err := ce.Run(context.Background(), "git init && git add -A && git commit -m 'initial' --allow-empty")
	return err
}

// --- Tool: project_create ---

// ProjectTool lets agents create and manage full coding projects.
type ProjectTool struct {
	baseDir string
}

func NewProjectTool(baseDir string) *ProjectTool {
	return &ProjectTool{baseDir: baseDir}
}

func (t *ProjectTool) Name() string { return "project" }
func (t *ProjectTool) Description() string {
	return "Create and manage coding projects in isolated environments. Actions: create, write, read, run, list, diff."
}
func (t *ProjectTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":     map[string]any{"type": "string", "description": "create|write|read|run|list|diff", "enum": []string{"create", "write", "read", "run", "list", "diff"}},
			"project_id": map[string]any{"type": "string", "description": "Project identifier"},
			"path":       map[string]any{"type": "string", "description": "File path (for write/read)"},
			"content":    map[string]any{"type": "string", "description": "File content (for write)"},
			"command":    map[string]any{"type": "string", "description": "Shell command (for run)"},
		},
		"required": []string{"action"},
	}
}

func (t *ProjectTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	projectID, _ := args["project_id"].(string)
	agentID := tools.AgentIDFromCtx(ctx)

	ce := NewCodingEnv(t.baseDir, agentID, projectID)

	switch action {
	case "create":
		ce.InitGit()
		slog.Info("project.created", "agent", agentID, "project", ce.ProjectID, "dir", ce.WorkDir)
		return tools.TextResult(fmt.Sprintf("Project created: %s\nDirectory: %s", ce.ProjectID, ce.WorkDir))

	case "write":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path == "" || content == "" { return tools.ErrorResult("path and content required") }
		if err := ce.WriteFile(path, content); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.TextResult(fmt.Sprintf("wrote %s (%d bytes)", path, len(content)))

	case "read":
		path, _ := args["path"].(string)
		content, err := ce.ReadFile(path)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.TextResult(content)

	case "run":
		command, _ := args["command"].(string)
		if command == "" { return tools.ErrorResult("command required") }
		out, exitCode, _ := ce.Run(ctx, command)
		if exitCode != 0 {
			return tools.ErrorResult(fmt.Sprintf("exit %d\n%s", exitCode, out))
		}
		return tools.TextResult(out)

	case "list":
		return tools.TextResult(strings.Join(ce.ListFiles(), "\n"))

	case "diff":
		return tools.TextResult(ce.Diff())

	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}


