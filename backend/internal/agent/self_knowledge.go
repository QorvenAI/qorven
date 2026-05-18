// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qorvenai/qorven/internal/tools"
)

// SelfKnowledge gives the agent deep understanding of its own codebase.
// Without this, self-building is blind — the agent would break things.
//
// The agent can query:
//   - Package structure (what exists, what each package does)
//   - File contents (read any source file)
//   - DB schema (tables, columns)
//   - API endpoints (routes, handlers)
//   - Build status (does it compile? do tests pass?)
//   - Git history (recent changes, who changed what)
//   - Error logs (what's failing in production)

type SelfKnowledgeTool struct {
	codebaseDir string
	dbDSN       string
}

func NewSelfKnowledgeTool(codebaseDir string) *SelfKnowledgeTool {
	return &SelfKnowledgeTool{codebaseDir: codebaseDir}
}

func (t *SelfKnowledgeTool) Name() string { return "self_knowledge" }
func (t *SelfKnowledgeTool) Description() string {
	return "Query Qorven's own codebase, architecture, DB schema, API endpoints, build status, and error logs. Use this BEFORE making any changes to understand the system."
}
func (t *SelfKnowledgeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type": "string",
				"enum": []string{"packages", "file", "search", "schema", "endpoints", "tools", "build", "test", "git_log", "errors"},
				"description": "What to query: packages (list all), file (read source), search (grep), schema (DB tables), endpoints (API routes), tools (registered), build (compile check), test (run tests), git_log (recent changes), errors (recent failures)",
			},
			"target": map[string]any{
				"type":        "string",
				"description": "File path (for 'file'), search term (for 'search'), table name (for 'schema')",
			},
		},
		"required": []string{"query"},
	}
}

func (t *SelfKnowledgeTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	query, _ := args["query"].(string)
	target, _ := args["target"].(string)

	switch query {
	case "packages":
		return t.listPackages()
	case "file":
		return t.readFile(target)
	case "search":
		return t.searchCode(target)
	case "schema":
		return t.dbSchema(target)
	case "endpoints":
		return t.apiEndpoints()
	case "tools":
		return t.registeredTools()
	case "build":
		return t.buildCheck()
	case "test":
		return t.runTests(target)
	case "git_log":
		return t.gitLog()
	case "errors":
		return t.recentErrors()
	default:
		return tools.ErrorResult("unknown query: " + query)
	}
}

func (t *SelfKnowledgeTool) listPackages() *tools.Result {
	var sb strings.Builder
	entries, _ := os.ReadDir(filepath.Join(t.codebaseDir, "internal"))
	for _, e := range entries {
		if !e.IsDir() { continue }
		files, _ := filepath.Glob(filepath.Join(t.codebaseDir, "internal", e.Name(), "*.go"))
		count := 0
		for _, f := range files {
			if !strings.HasSuffix(f, "_test.go") { count++ }
		}
		if count > 0 {
			sb.WriteString(fmt.Sprintf("internal/%s/ (%d files)\n", e.Name(), count))
		}
	}
	return tools.TextResult(sb.String())
}

func (t *SelfKnowledgeTool) readFile(path string) *tools.Result {
	if path == "" { return tools.ErrorResult("target file path required") }
	// Strip root prefix if agent passes absolute path
	path = strings.TrimPrefix(path, t.codebaseDir+"/")
	path = strings.TrimPrefix(path, t.codebaseDir)
	full := filepath.Join(t.codebaseDir, path)
	data, err := os.ReadFile(full)
	if err != nil { return tools.ErrorResult(err.Error()) }
	content := string(data)
	if len(content) > 8000 { content = content[:8000] + "\n... (truncated)" }
	return tools.TextResult(content)
}

func (t *SelfKnowledgeTool) searchCode(term string) *tools.Result {
	if term == "" { return tools.ErrorResult("search term required") }
	cmd := exec.Command("grep", "-rn", "--include=*.go", "-l", term, filepath.Join(t.codebaseDir, "internal"))
	out, _ := cmd.CombinedOutput()
	result := string(out)
	if len(result) > 4000 { result = result[:4000] + "\n... (truncated)" }
	if result == "" { result = "no matches found" }
	return tools.TextResult(result)
}

func (t *SelfKnowledgeTool) dbSchema(table string) *tools.Result {
	// Require QORVEN_POSTGRES_DSN — no fallback to a hardcoded dev DSN.
	// Shipping a literal `postgres://qorven:qorven2026@localhost…` in
	// source code is a published credential even if the password is
	// the project's own; open-source installs shouldn't contain any
	// default secret, and this tool is only useful when a real DSN
	// is already present in the environment.
	dsn := os.Getenv("QORVEN_POSTGRES_DSN")
	if dsn == "" {
		return tools.TextResult("db_schema: QORVEN_POSTGRES_DSN not set — cannot introspect schema.")
	}
	// Extract connection params from DSN for psql invocation
	psqlCmd := fmt.Sprintf(`psql "%s"`, dsn)
	if table != "" {
		cmd := exec.Command("bash", "-c", fmt.Sprintf(
			`%s -c "SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name = '%s' ORDER BY ordinal_position;"`,
			psqlCmd, table))
		out, _ := cmd.CombinedOutput()
		return tools.TextResult(string(out))
	}
	cmd := exec.Command("bash", "-c",
		fmt.Sprintf(`%s -c "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;"`, psqlCmd))
	out, _ := cmd.CombinedOutput()
	return tools.TextResult(string(out))
}

func (t *SelfKnowledgeTool) apiEndpoints() *tools.Result {
	data, _ := os.ReadFile(filepath.Join(t.codebaseDir, "internal/gateway/gateway.go"))
	var endpoints []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, `r.Get("`) || strings.Contains(line, `r.Post("`) ||
			strings.Contains(line, `r.Put("`) || strings.Contains(line, `r.Delete("`) {
			endpoints = append(endpoints, strings.TrimSpace(line))
		}
	}
	if len(endpoints) > 50 { endpoints = endpoints[:50] }
	return tools.TextResult(strings.Join(endpoints, "\n"))
}

func (t *SelfKnowledgeTool) registeredTools() *tools.Result {
	cmd := exec.Command("grep", "-n", `reg.Register(`, filepath.Join(t.codebaseDir, "internal/gateway/gateway.go"))
	out, _ := cmd.CombinedOutput()
	return tools.TextResult(string(out))
}

func (t *SelfKnowledgeTool) buildCheck() *tools.Result {
	cmd := exec.Command("go", "build", "-o", "/dev/null", ".")
	cmd.Dir = t.codebaseDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tools.TextResult("BUILD FAILED:\n" + string(out))
	}
	return tools.TextResult("BUILD OK")
}

func (t *SelfKnowledgeTool) runTests(pkg string) *tools.Result {
	target := "./..."
	if pkg != "" { target = "./" + pkg + "/..." }
	cmd := exec.Command("go", "test", "-short", "-count=1", target)
	cmd.Dir = t.codebaseDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, _ := cmd.CombinedOutput()
	result := string(out)
	if len(result) > 4000 { result = result[:4000] + "\n... (truncated)" }
	return tools.TextResult(result)
}

func (t *SelfKnowledgeTool) gitLog() *tools.Result {
	cmd := exec.Command("git", "log", "--oneline", "-20")
	cmd.Dir = t.codebaseDir
	out, _ := cmd.CombinedOutput()
	return tools.TextResult(string(out))
}

func (t *SelfKnowledgeTool) recentErrors() *tools.Result {
	// Read gateway log for recent errors
	data, err := os.ReadFile("/tmp/gateway.log")
	if err != nil { return tools.TextResult("no gateway log found") }
	lines := strings.Split(string(data), "\n")
	var errors []string
	for _, line := range lines {
		if strings.Contains(line, "ERROR") || strings.Contains(line, "WARN") {
			errors = append(errors, line)
		}
	}
	if len(errors) > 20 { errors = errors[len(errors)-20:] }
	if len(errors) == 0 { return tools.TextResult("no recent errors") }
	return tools.TextResult(strings.Join(errors, "\n"))
}
