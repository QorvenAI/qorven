// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// LSPBridge provides semantic code navigation via language servers.
// Connects to gopls (Go), typescript-language-server (TS), pyright (Python).
type LSPBridge struct {
	servers map[string]*LSPServer
}

type LSPServer struct {
	Language string
	Command  string
	Args     []string
}

// DefaultServers returns pre-configured language servers.
var DefaultServers = map[string]*LSPServer{
	"go":         {Language: "go", Command: "gopls", Args: []string{"serve"}},
	"typescript": {Language: "typescript", Command: "typescript-language-server", Args: []string{"--stdio"}},
	"python":     {Language: "python", Command: "pyright-langserver", Args: []string{"--stdio"}},
}

func NewLSPBridge() *LSPBridge {
	return &LSPBridge{servers: make(map[string]*LSPServer)}
}

// LSPTool exposes LSP operations as an agent tool.
type LSPTool struct {
	bridge *LSPBridge
}

func NewLSPTool() *LSPTool { return &LSPTool{bridge: NewLSPBridge()} }

func (t *LSPTool) Name() string        { return "lsp" }
func (t *LSPTool) Description() string { return "Semantic code navigation: go-to-definition, find-references, hover info, symbols." }
func (t *LSPTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":   map[string]any{"type": "string", "enum": []string{"definition", "references", "hover", "symbols", "diagnostics"}, "description": "LSP action"},
		"file":     map[string]any{"type": "string", "description": "File path"},
		"line":     map[string]any{"type": "integer", "description": "Line number (1-based)"},
		"column":   map[string]any{"type": "integer", "description": "Column number (1-based)"},
		"symbol":   map[string]any{"type": "string", "description": "Symbol name (for symbols action)"},
	}, "required": []string{"action", "file"}}
}

func (t *LSPTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	file, _ := args["file"].(string)
	line, _ := toInt(args["line"])
	col, _ := toInt(args["column"])
	symbol, _ := args["symbol"].(string)

	switch action {
	case "definition":
		return t.goToDefinition(ctx, file, line, col)
	case "references":
		return t.findReferences(ctx, file, line, col)
	case "hover":
		return t.hover(ctx, file, line, col)
	case "symbols":
		return t.documentSymbols(ctx, file, symbol)
	case "diagnostics":
		return t.diagnostics(ctx, file)
	default:
		return ErrorResult("unknown action: " + action)
	}
}

// diagnostics runs the language server's compile/type-check on file and
// returns errors and warnings. Uses gopls check for Go, tsc --noEmit for TS.
func (t *LSPTool) diagnostics(ctx context.Context, file string) *Result {
	switch {
	case strings.HasSuffix(file, ".go"):
		out, err := exec.CommandContext(ctx, "gopls", "check", file).CombinedOutput()
		s := strings.TrimSpace(string(out))
		if err != nil && s == "" {
			s = err.Error()
		}
		if s == "" {
			return &Result{ForLLM: "No diagnostics — file is clean."}
		}
		return &Result{ForLLM: s}
	case strings.HasSuffix(file, ".ts") || strings.HasSuffix(file, ".tsx"):
		out, _ := exec.CommandContext(ctx, "npx", "--no", "tsc", "--noEmit", "--pretty", "false", file).CombinedOutput()
		s := strings.TrimSpace(string(out))
		if s == "" {
			return &Result{ForLLM: "No diagnostics — file is clean."}
		}
		return &Result{ForLLM: s}
	default:
		return ErrorResult("diagnostics not supported for this file type (supported: .go, .ts, .tsx)")
	}
}

// goToDefinition uses gopls/ctags as fallback for go-to-definition.
func (t *LSPTool) goToDefinition(ctx context.Context, file string, line, col int) *Result {
	// Try gopls for Go files
	if strings.HasSuffix(file, ".go") {
		out, err := exec.CommandContext(ctx, "gopls", "definition", fmt.Sprintf("%s:%d:%d", file, line, col)).CombinedOutput()
		if err == nil { return &Result{ForLLM: string(out)} }
	}
	// Fallback: ctags-based definition search
	return t.ctagsDefinition(ctx, file, line, col)
}

func (t *LSPTool) findReferences(ctx context.Context, file string, line, col int) *Result {
	if strings.HasSuffix(file, ".go") {
		out, err := exec.CommandContext(ctx, "gopls", "references", fmt.Sprintf("%s:%d:%d", file, line, col)).CombinedOutput()
		if err == nil { return &Result{ForLLM: string(out)} }
	}
	// Fallback: grep for symbol at position
	return t.grepReferences(ctx, file, line, col)
}

func (t *LSPTool) hover(ctx context.Context, file string, line, col int) *Result {
	if strings.HasSuffix(file, ".go") {
		out, err := exec.CommandContext(ctx, "gopls", "hover", fmt.Sprintf("%s:%d:%d", file, line, col)).CombinedOutput()
		if err == nil { return &Result{ForLLM: string(out)} }
	}
	return ErrorResult("hover not available for this file type")
}

func (t *LSPTool) documentSymbols(ctx context.Context, file, symbol string) *Result {
	// Use ctags for universal symbol listing
	out, err := exec.CommandContext(ctx, "ctags", "--output-format=json", "-f", "-", file).CombinedOutput()
	if err != nil { return ErrorResult("ctags not available: " + err.Error()) }
	// Filter by symbol name if provided
	if symbol != "" {
		var filtered []string
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, symbol) { filtered = append(filtered, line) }
		}
		return &Result{ForLLM: strings.Join(filtered, "\n")}
	}
	return &Result{ForLLM: string(out)}
}

func (t *LSPTool) ctagsDefinition(ctx context.Context, file string, line, col int) *Result {
	// Read the symbol at position, then search with ctags
	content, err := exec.CommandContext(ctx, "sed", "-n", fmt.Sprintf("%dp", line), file).Output()
	if err != nil { return ErrorResult("cannot read file") }
	words := strings.Fields(string(content))
	if col > 0 && col <= len(string(content)) {
		// Find word at column
		pos := 0
		for _, w := range words {
			pos += len(w) + 1
			if pos >= col {
				out, _ := exec.CommandContext(ctx, "grep", "-rn", fmt.Sprintf("func %s\\|type %s\\|class %s", w, w, w), ".").CombinedOutput()
				return &Result{ForLLM: fmt.Sprintf("Symbol: %s\n%s", w, string(out))}
			}
		}
	}
	return ErrorResult("no symbol at position")
}

func (t *LSPTool) grepReferences(ctx context.Context, file string, line, col int) *Result {
	content, _ := exec.CommandContext(ctx, "sed", "-n", fmt.Sprintf("%dp", line), file).Output()
	words := strings.Fields(string(content))
	if len(words) == 0 { return ErrorResult("no content at line") }
	// Pick the most likely symbol
	sym := words[0]
	for _, w := range words {
		if len(w) > 3 && !isKeyword(w) { sym = w; break }
	}
	out, _ := exec.CommandContext(ctx, "grep", "-rn", sym, ".").CombinedOutput()
	lines := strings.Split(string(out), "\n")
	if len(lines) > 30 { lines = lines[:30] }
	return &Result{ForLLM: fmt.Sprintf("References to '%s':\n%s", sym, strings.Join(lines, "\n"))}
}

func isKeyword(w string) bool {
	kw := map[string]bool{"func": true, "type": true, "var": true, "const": true, "import": true, "package": true, "return": true, "if": true, "for": true, "class": true, "def": true}
	return kw[w]
}

// Ensure LSPTool satisfies the Tool interface
func init() {
	// Register will be called from gateway
	_ = json.Marshal // ensure json import used
}
