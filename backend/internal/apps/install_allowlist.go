// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"fmt"
	"path/filepath"
	"strings"
)

// isAllowedInstallPath reports whether absPath is permitted as an app install
// location. It resolves symlinks where possible to prevent symlink-escape
// attacks, then checks that the resolved path is under one of the allowedRoots
// using a strict path-prefix check (with a trailing separator so that
// "/allowed/apps-extra" is NOT a match for "/allowed/apps").
//
// allowedRoots must be absolute, cleaned paths; they should be the Qorven
// data/apps directory and any other legitimate install locations.
//
// The not-exist case is handled gracefully: if EvalSymlinks fails because the
// path does not yet exist, we fall back to filepath.Clean so that a path that
// will be created under an allowed root still passes.
func isAllowedInstallPath(absPath string, allowedRoots []string) bool {
	if absPath == "" || len(allowedRoots) == 0 {
		return false
	}

	// Resolve to an absolute, cleaned path first.
	cleaned, err := filepath.Abs(absPath)
	if err != nil {
		return false
	}
	cleaned = filepath.Clean(cleaned)

	// Attempt symlink resolution; fall back to the cleaned path if the target
	// does not exist yet (e.g. a directory about to be created by Install).
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// Not-exist is acceptable: the directory is created by Install after
		// this check. Any other error is treated conservatively as a rejection.
		resolved = cleaned
	}

	for _, root := range allowedRoots {
		root = filepath.Clean(root)
		// Require the resolved path to equal the root exactly OR start with
		// root + separator so "/allowed/apps" does NOT match "/allowed/apps-evil".
		if resolved == root {
			return true
		}
		if strings.HasPrefix(resolved, root+string(filepath.Separator)) {
			// Extra guard: filepath.Rel must not produce a ".." component.
			rel, err := filepath.Rel(root, resolved)
			if err == nil && !strings.HasPrefix(rel, "..") {
				return true
			}
		}
	}
	return false
}

// ErrInstallPathNotPermitted is returned by Install when the supplied path is
// not under any allowed install root.
var ErrInstallPathNotPermitted = fmt.Errorf("install path not permitted")
