// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/tools"
)

// PrimeCoderWorkflow implements the structured coding workflow:
// 1. Analyze request → 2. Create plan → 3. Get approval → 4. Execute via sub-agents → 5. Verify
//
// Project files it manages:
//   .qorven/PLAN.md      — Current task plan with steps
//   .qorven/SPEC.md      — Requirements and acceptance criteria
//   .qorven/RULES.md     — Project coding rules (like CLAUDE.md / .cursorrules)
//   .qorven/MEMORY.md    — Project-level memory (learnings, decisions, patterns)
//   .qorven/TASKS.md     — Todo list for the project

const qorvenDir = ".qorven"

// ProjectContext loads all project-level context files for Prime Coder.
func LoadProjectContext(projectPath string) string {
	dir := filepath.Join(projectPath, qorvenDir)
	files := []string{"RULES.md", "MEMORY.md", "SPEC.md", "PLAN.md", "TASKS.md"}
	var sb strings.Builder

	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n<project_%s>\n%s\n</project_%s>\n", strings.ToLower(strings.TrimSuffix(f, ".md")), content, strings.ToLower(strings.TrimSuffix(f, ".md"))))
	}
	return sb.String()
}

// SaveProjectFile writes a file to the project's .qorven directory.
func SaveProjectFile(projectPath, filename, content string) error {
	dir := filepath.Join(projectPath, qorvenDir)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
}

// PrimeCoderTool manages the structured coding workflow.
type PrimeCoderTool struct {
	projectReg   *tools.ProjectRegistry
	hierarchyMem *memory.HierarchyStore
}

func NewPrimeCoderTool(reg *tools.ProjectRegistry) *PrimeCoderTool {
	return &PrimeCoderTool{projectReg: reg}
}

func (t *PrimeCoderTool) SetHierarchyMem(h *memory.HierarchyStore) { t.hierarchyMem = h }

func (t *PrimeCoderTool) Name() string { return "prime_coder" }
func (t *PrimeCoderTool) Description() string {
	return `Structured coding workflow manager. Actions:
  plan        — Create/read implementation plan (.qorven/PLAN.md)
  spec        — Create/read requirements (.qorven/SPEC.md)
  rules       — Read/update project coding rules (.qorven/RULES.md)
  memory      — Read/update project memory file (.qorven/MEMORY.md)
  save_memory — Save a learning to project DB memory (persists across sessions, searchable)
  tasks       — Read/update project tasks (.qorven/TASKS.md)
  context     — Load all project context files
  init        — Initialize .qorven directory with default files`
}
func (t *PrimeCoderTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":      map[string]any{"type": "string", "description": "plan|spec|rules|memory|tasks|context|init"},
		"project_id":  map[string]any{"type": "string", "description": "Project ID"},
		"content":     map[string]any{"type": "string", "description": "Content to write (for rules/memory/tasks/plan/spec)"},
	}, "required": []string{"action"}}
}

func (t *PrimeCoderTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	projectID, _ := args["project_id"].(string)
	content, _ := args["content"].(string)

	// Resolve project path
	projectPath := ""
	if t.projectReg != nil && projectID != "" {
		if p := t.projectReg.Get(projectID); p != nil {
			projectPath = p.Path
		}
	}
	if projectPath == "" {
		projectPath = tools.WorkspaceFromCtx(ctx)
	}

	switch action {
	case "init":
		return t.initProject(projectPath)
	case "context":
		ctx := LoadProjectContext(projectPath)
		if ctx == "" {
			return tools.TextResult("No project context files found. Use prime_coder(action=init) to create them.")
		}
		return tools.TextResult(ctx)
	case "plan":
		if content != "" {
			SaveProjectFile(projectPath, "PLAN.md", content)
			return tools.TextResult("Plan saved to .qorven/PLAN.md")
		}
		return t.readFile(projectPath, "PLAN.md")
	case "spec":
		if content != "" {
			SaveProjectFile(projectPath, "SPEC.md", content)
			return tools.TextResult("Spec saved to .qorven/SPEC.md")
		}
		return t.readFile(projectPath, "SPEC.md")
	case "rules":
		if content != "" {
			SaveProjectFile(projectPath, "RULES.md", content)
			return tools.TextResult("Rules saved to .qorven/RULES.md")
		}
		return t.readFile(projectPath, "RULES.md")
	case "memory":
		if content != "" {
			SaveProjectFile(projectPath, "MEMORY.md", content)
			return tools.TextResult("Memory saved to .qorven/MEMORY.md")
		}
		return t.readFile(projectPath, "MEMORY.md")
	case "tasks":
		if content != "" {
			SaveProjectFile(projectPath, "TASKS.md", content)
			return tools.TextResult("Tasks saved to .qorven/TASKS.md")
		}
		return t.readFile(projectPath, "TASKS.md")
	case "save_memory":
		if content == "" {
			return tools.ErrorResult("content required for save_memory")
		}
		sessionID := tools.SessionIDFromCtx(ctx)
		if t.hierarchyMem != nil && sessionID != "" {
			t.hierarchyMem.SaveTask(ctx, sessionID, "prime", content, "prime_coder")
			return tools.TextResult("Memory saved to project DB (searchable across sessions)")
		}
		// Fallback: append to MEMORY.md
		existing, _ := os.ReadFile(filepath.Join(projectPath, qorvenDir, "MEMORY.md"))
		SaveProjectFile(projectPath, "MEMORY.md", string(existing)+"\n- "+content)
		return tools.TextResult("Memory appended to .qorven/MEMORY.md")
	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}

func (t *PrimeCoderTool) readFile(projectPath, filename string) *tools.Result {
	data, err := os.ReadFile(filepath.Join(projectPath, qorvenDir, filename))
	if err != nil {
		return tools.TextResult(fmt.Sprintf("No %s found. Create one with prime_coder(action=%s, content=...)", filename, strings.TrimSuffix(strings.ToLower(filename), ".md")))
	}
	return tools.TextResult(string(data))
}

func (t *PrimeCoderTool) initProject(projectPath string) *tools.Result {
	dir := filepath.Join(projectPath, qorvenDir)
	os.MkdirAll(dir, 0755)

	// Default RULES.md
	if _, err := os.Stat(filepath.Join(dir, "RULES.md")); os.IsNotExist(err) {
		SaveProjectFile(projectPath, "RULES.md", defaultRules)
	}
	// Default MEMORY.md
	if _, err := os.Stat(filepath.Join(dir, "MEMORY.md")); os.IsNotExist(err) {
		SaveProjectFile(projectPath, "MEMORY.md", "# Project Memory\n\nLearnings, decisions, and patterns discovered during development.\n")
	}
	// Default TASKS.md
	if _, err := os.Stat(filepath.Join(dir, "TASKS.md")); os.IsNotExist(err) {
		SaveProjectFile(projectPath, "TASKS.md", "# Tasks\n\n- [ ] Initial setup\n")
	}

	return tools.TextResult(fmt.Sprintf("Initialized .qorven/ in %s with RULES.md, MEMORY.md, TASKS.md", projectPath))
}

const defaultRules = `# Project Rules

## Code Style
- Follow existing code conventions in the project
- Keep changes minimal and focused
- Fix root causes, not symptoms

## Process
- Read relevant files before making changes
- Run diagnostics after every edit
- Write tests for new functionality
- Commit messages: type(scope): description

## Safety
- Never delete files without confirmation
- Always check git status before committing
- Run the test suite before marking a task done

## Architecture
- Keep functions small and focused
- Prefer composition over inheritance
- Document public APIs
`

// PrimeCoderSystemPrompt returns the system prompt addition for Prime Coder mode.
func PrimeCoderSystemPrompt(projectPath string) string {
	ctx := LoadProjectContext(projectPath)

	return fmt.Sprintf(`## Prime Coder Mode

You are Prime Coder — the user's primary AI coding assistant. You have FULL access to:
- **Company memory** — shared knowledge all agents see (company facts, policies)
- **Your memory** — everything the user has ever told you across all sessions
- **Project memory** — learnings specific to this project (.qorven/MEMORY.md + task-scoped DB memories)

### Memory Hierarchy (you see ALL of these)
1. Company knowledge — visible to all Qors
2. Prime observations — your notes about the system
3. Your agent memories — from all past conversations with the user
4. Project memories — scoped to this project/task only

Other Qors the user created (for email, social, etc.) do NOT see your project memories.
They only see company + their own agent memories.

### Workflow
When the user gives you a coding task:

1. **Understand** — Read relevant files, check project memory and rules
2. **Plan** — Create a step-by-step plan. Save with prime_coder(action=plan, content=...)
3. **Confirm** — Show the plan. Ask "Shall I proceed?" Wait for approval.
4. **Execute** — Implement each step. Delegate to sub-agents for parallel work.
5. **Verify** — Run diagnostics, tests. Check git diff.
6. **Learn** — Save important decisions/patterns to project memory.

### Project Context
%s

### Rules
- ALWAYS read the file before editing it
- ALWAYS run diagnostics after changes
- ALWAYS show the plan before executing multi-file changes
- Update .qorven/TASKS.md as you complete items
- Save important decisions to .qorven/MEMORY.md
- When you learn something about the codebase, save it to memory

### Sub-Agent Delegation
For complex tasks, delegate to specialist agents:
- delegate(agent="developer", message="Implement X in path/to/file.go")
- delegate(agent="developer", message="Write tests for X")
Sub-agents work independently. They do NOT share your project context.
You are the orchestrator — review their output before presenting to user.
`, ctx)
}

var _ = json.Marshal // keep import
