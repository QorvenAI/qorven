// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
)

// serverSpec maps a language to its LSP server binary + args.
type serverSpec struct {
	bin  string
	args []string
}

var serverSpecs = map[string]serverSpec{
	"go":         {bin: "gopls", args: []string{"serve"}},
	"typescript": {bin: "typescript-language-server", args: []string{"--stdio"}},
	"javascript": {bin: "typescript-language-server", args: []string{"--stdio"}},
	"python":     {bin: "pyright-langserver", args: []string{"--stdio"}},
}

// resolveServer returns the absolute binary path + args for a language's LSP
// server and whether it is available. Checks PATH and the Go bin dir (where
// `go install gopls` lands). Missing servers → (_, _, false): the bridge skips
// that language gracefully (Monaco keeps AI ghost-text, no squiggles).
func resolveServer(lang string) (string, []string, bool) {
	spec, ok := serverSpecs[lang]
	if !ok {
		return "", nil, false
	}
	// PATH first.
	if p, err := exec.LookPath(spec.bin); err == nil {
		return p, spec.args, true
	}
	// Go bin dir fallback (gopls from `go install`).
	if gobin := goBinDir(); gobin != "" {
		cand := filepath.Join(gobin, spec.bin)
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			return cand, spec.args, true
		}
	}
	return "", nil, false
}

func goBinDir() string {
	if b := os.Getenv("GOBIN"); b != "" {
		return b
	}
	if gp := os.Getenv("GOPATH"); gp != "" {
		return filepath.Join(gp, "bin")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "go", "bin")
	}
	return ""
}

// LanguageForExt maps a file extension to an LSP language id (or "" if none).
func LanguageForExt(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	default:
		return ""
	}
}
