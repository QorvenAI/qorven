// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
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
// appsDir is the resolved data directory for apps (e.g. /var/lib/qorven/apps or ~/.qorven/apps).
func PrimeCoderSystemPrompt(projectPath, appsDir string) string {
	ctx := LoadProjectContext(projectPath)
	if appsDir == "" {
		appsDir = "~/.qorven/apps"
	}

	prompt := fmt.Sprintf(`## Prime Coder Mode

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

### Qorven App Platform

You are running INSIDE Qorven. Qorven has a first-class app platform — like WordPress plugins.
When the user asks you to "build an app", they mean a Qorven app that installs into /apps, NOT a standalone project.

**App directory:** ` + "`~/.qorven/apps/{slug}/`" + `

**Required structure:**
` + "```" + `
~/.qorven/apps/my-app/
├── app.yaml              ← manifest (required)
├── migrations/
│   └── 001_schema.sql    ← DB schema (CREATE TABLE IF NOT EXISTS ...)
├── tools/
│   └── my_tool.sh        ← shell scripts for agent tools
└── ui/
    ├── package.json
    ├── vite.config.ts
    └── src/
        └── index.tsx     ← IIFE bundle entry
` + "```" + `

**app.yaml format:**
` + "```yaml" + `
slug: my-app   # MUST use hyphens, not underscores (e.g., todo-app not todo_app)
display_name: My App
version: 0.1.0
description: What this app does
author: qorven
permissions:
  - db_write
  - tool_register   # REQUIRED for tool scripts to execute — without this, tools load with 0 entries
migrations_dir: migrations
frontend:
  bundle: ui/frontend/bundle.js   # output path after build
  pages:
    - id: home
      label: Home
      icon: Home
      path: home
tools:
  - name: add_item
    description: Add an item
    command: tools/add_item.sh
    parameters:
      type: object
      properties:
        name: { type: string, description: "Item name" }
      required: [name]
` + "```" + `

**UI bundle — ui/src/index.tsx:**
The bundle is an IIFE that calls ` + "`window.__QorvenApp.register()`" + `. Read ` + "`backend/cmd/scaffold/templates/ui/src/index.tsx.tmpl`" + ` for the exact pattern.
Key points:
- Call ` + "`register({ id: 'my-app', displayName: 'My App', pages: [{id, path, label, component}] })`" + `
- Use ` + "`window.__QorvenUI`" + ` for React (do NOT import React — it's provided by the host)
- Fetch data via ` + "`request('/apps/my-app/tools/tool_name', {method:'POST', body: JSON.stringify({args:{}})})`" + `
- ` + "`request()`" + ` is available on ` + "`window.__QorvenApp.request`" + ` — auto-attaches auth token
- **` + "`request()`" + ` returns a ` + "`tools.Result`" + ` object, NOT a fetch Response. Shape:**
  ` + "```ts" + `
  { content: string, user_content: string, is_error: boolean }
  // content = raw stdout from your tool script
  // Parse it: const data = JSON.parse(result.content)
  // Do NOT call result.ok, result.json() — those don't exist
  ` + "```" + `
- **Available ` + "`window.__QorvenUI`" + ` components** (only use these — do NOT invent names):
  ` + "`Button`" + `, ` + "`Card`" + `, ` + "`Input`" + `, ` + "`Checkbox`" + `, ` + "`Badge`" + `, ` + "`Avatar`" + `, ` + "`Separator`" + `, ` + "`Skeleton`" + `,
  ` + "`Select`" + `, ` + "`Tabs`" + `, ` + "`Dialog`" + `, ` + "`Drawer`" + `, ` + "`Sheet`" + `, ` + "`Popover`" + `, ` + "`Tooltip`" + `, ` + "`Switch`" + `,
  ` + "`Progress`" + `, ` + "`Textarea`" + `, ` + "`Label`" + `, ` + "`Toggle`" + `, ` + "`Table`" + `, ` + "`TableBody`" + `, ` + "`TableCell`" + `,
  ` + "`TableHead`" + `, ` + "`TableHeader`" + `, ` + "`TableRow`" + `, ` + "`Text`" + `, ` + "`Accordion`" + `, ` + "`Collapsible`" + `
  Plus ` + "`icons`" + ` (all Lucide icons) and ` + "`cn`" + ` (classnames helper).
  There is NO ` + "`List`" + `, ` + "`ListItem`" + `, ` + "`IconButton`" + ` — use plain ` + "`<div>`" + ` rows with ` + "`icons.Trash2`" + ` etc instead.

**Tool scripts — tools/*.sh:**
Shell scripts that run server-side. Args arrive via STDIN as JSON (NOT as $1).
` + "```bash" + `
#!/bin/bash
INPUT=$(cat)   # read JSON from stdin
NAME=$(echo "$INPUT" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d.get('name',''))" 2>/dev/null)
# then use psql $QORVEN_DB_DSN to query the DB
` + "```" + `

**Build the bundle:**
` + "```bash" + `
cd ~/.qorven/apps/my-app/ui
npm install
npm run build   # outputs to ui/frontend/bundle.js
` + "```" + `

**Install the app:**
` + "```bash" + `
curl -s -X POST http://localhost:4200/v1/apps/ \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"path": "~/.qorven/apps/my-app"}'
` + "```" + `
Get a token first: ` + "`curl -s -X POST http://localhost:4200/auth/login -d '{\"username\":\"jay\",\"password\":\"devpass123\"}' | python3 -c \"import sys,json; print(json.load(sys.stdin)['token'])\"`" + `

**Reload after editing:**
` + "```bash" + `
curl -s -X POST http://localhost:4200/v1/apps/{id}/reload -H "Authorization: Bearer $TOKEN"
` + "```" + `

**When building a Qorven app, follow these steps exactly:**
1. exec: mkdir -p ~/.qorven/apps/{slug}/migrations ~/.qorven/apps/{slug}/tools ~/.qorven/apps/{slug}/ui/src
2. write_file: ~/.qorven/apps/{slug}/app.yaml (follow the format above exactly)
3. write_file: ~/.qorven/apps/{slug}/migrations/001_create_tables.up.sql (CREATE TABLE IF NOT EXISTS ... — MUST end in .up.sql or migrations are skipped)
4. write_file: each tool script in ~/.qorven/apps/{slug}/tools/ (read args from stdin: INPUT=$(cat))
   exec: chmod +x ~/.qorven/apps/{slug}/tools/*.sh  ← REQUIRED or tools fail with Permission denied
5. write_file: UI source files (index.tsx, vite.config.ts, package.json) following the pattern in the UI bundle section above
6. exec: cd ~/.qorven/apps/{slug}/ui && npm install && npm run build
7. install_app: path=~/.qorven/apps/{slug}

**CRITICAL: Do NOT call scaffold_app** — it creates a Go Wasm plugin (wrong format). Use write_file and exec to create files directly per the structure above.

**When editing an existing app:**
ALWAYS use your tools directly. Never respond with code blocks for the user to run manually.
1. exec: cat ~/.qorven/apps/{slug}/app.yaml  (read current state first)
2. exec: cat the relevant tool scripts / bundle.js
3. write_file: overwrite each file that needs changing
4. If schema changed: exec the ALTER TABLE or new migration via psql
5. If bundle changed: cd ~/.qorven/apps/{slug}/ui && npm run build
6. exec: reload the app via the API (see Reload above)
This is autonomous work — execute every step yourself with tools.
`, ctx)
	// Replace placeholder paths with the actual resolved apps directory.
	prompt = strings.ReplaceAll(prompt, "~/.qorven/apps", appsDir)
	return prompt
}

var _ = json.Marshal // keep import
