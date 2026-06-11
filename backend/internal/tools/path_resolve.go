// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"fmt"
	"os"
	"strings"
)

// resolvePath expands a (possibly tilde/relative) path against a base workspace
// and confirms it stays within the workspace or the user's home directory.
// Platform-neutral: lives here (no build constraint) because exec.go is
// non-Windows-only, while filesystem.go and code_edit.go — which also call this
// — must compile on every platform including Windows.
func resolvePath(path, base string, mustExist bool) (string, error) {
	if path == "" {
		return base, nil
	}
	// Expand tilde to real home directory
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = home + path[1:]
		}
	} else if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	}
	// Expand relative paths against base
	if !strings.HasPrefix(path, "/") {
		path = base + "/" + path
	}
	// Allow workspace or home-dir paths
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, base) && (home == "" || !strings.HasPrefix(path, home)) {
		return "", fmt.Errorf("path %s is outside workspace", path)
	}
	if mustExist {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("path does not exist: %s", path)
		}
	}
	return path, nil
}
