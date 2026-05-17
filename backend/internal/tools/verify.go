// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// PostEditVerify runs language-specific verification after file edits.
// Returns errors found, or empty string if clean.
func PostEditVerify(ctx context.Context, filePath, workspace string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	var checks [][]string

	switch ext {
	case ".go":
		checks = [][]string{{"go", "build", "./..."}, {"go", "vet", "./..."}}
	case ".ts", ".tsx":
		checks = [][]string{{"npx", "tsc", "--noEmit"}}
	case ".js", ".jsx":
		checks = [][]string{{"npx", "eslint", filePath, "--quiet"}}
	case ".py":
		checks = [][]string{{"python3", "-m", "py_compile", filePath}}
	case ".rs":
		checks = [][]string{{"cargo", "check", "--quiet"}}
	default:
		return ""
	}

	var errors []string
	for _, args := range checks {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %s", args[0], strings.TrimSpace(string(out))))
		}
	}
	if len(errors) > 0 {
		return "⚠️ Verification errors:\n" + strings.Join(errors, "\n")
	}
	return ""
}
