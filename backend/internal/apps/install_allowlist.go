// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isAllowedInstallPath reports whether absPath is permitted as an app install
// location. It resolves symlinks to prevent symlink-escape attacks, then checks
// that the resolved path is under one of the allowedRoots using a strict
// path-prefix check (with a trailing separator so that "/allowed/apps-extra"
// is NOT a match for "/allowed/apps").
//
// allowedRoots must be absolute, cleaned paths; they should be the Qorven
// data/apps directory and any other legitimate install locations.
//
// The not-yet-existing leaf case is handled safely: rather than falling back to
// the raw textual path (which would let a symlinked parent component escape),
// we walk UP from the target to find the deepest ancestor that actually EXISTS,
// resolve symlinks on THAT ancestor, then re-join the non-existent suffix
// (cleaned) on top. This ensures a parent directory that is a symlink pointing
// outside the root is always detected, even when the leaf doesn't exist yet.
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

	// Resolve symlinks via the deepest existing ancestor so that a parent
	// directory that is a symlink escaping the root is caught even when the
	// leaf doesn't exist yet.
	resolved := resolveDeepestExisting(cleaned)

	// Resolve each root for a fair comparison (roots may also be symlinked).
	for _, root := range allowedRoots {
		resolvedRoot := filepath.Clean(root)
		if r, err := filepath.EvalSymlinks(resolvedRoot); err == nil {
			resolvedRoot = r
		}
		// Require the resolved path to equal the root exactly OR start with
		// root + separator so "/allowed/apps" does NOT match "/allowed/apps-evil".
		if resolved == resolvedRoot {
			return true
		}
		if strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator)) {
			// Extra guard: filepath.Rel must not produce a ".." component.
			rel, err := filepath.Rel(resolvedRoot, resolved)
			if err == nil && !strings.HasPrefix(rel, "..") {
				return true
			}
		}
	}
	return false
}

// resolveDeepestExisting resolves symlinks on the deepest ancestor of path
// that actually exists on disk, then re-joins the non-existent suffix (cleaned)
// on top. This prevents a symlinked parent from escaping an allowed root even
// when the leaf component doesn't exist yet.
//
// If no ancestor exists at all (e.g. a completely synthetic path), the cleaned
// path is returned unchanged — the prefix check will then reject it unless it
// happens to be under a root by textual structure, which is safe because the
// root's own symlinks are also resolved at comparison time.
func resolveDeepestExisting(path string) string {
	// Walk from the full path up toward the filesystem root, stopping at the
	// first component that exists. Collect the non-existent suffix as we go.
	current := path
	var suffix []string
	for {
		if _, err := os.Lstat(current); err == nil {
			// This component exists; resolve its symlinks.
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				// Exists but EvalSymlinks failed (e.g. broken inner symlink):
				// treat conservatively — return the cleaned original path so
				// the subsequent prefix check can still reject it.
				return path
			}
			// Re-join the non-existent suffix onto the resolved ancestor.
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding an existing component.
			break
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
	// No existing ancestor found — return cleaned path as-is.
	return path
}

// ErrInstallPathNotPermitted is returned by Install when the supplied path is
// not under any allowed install root.
var ErrInstallPathNotPermitted = fmt.Errorf("install path not permitted")
