//go:build unit

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsAllowedInstallPath_UnderRoot_Allowed(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "my-app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if !isAllowedInstallPath(sub, []string{root}) {
		t.Errorf("expected path %q under root %q to be allowed", sub, root)
	}
}

func TestIsAllowedInstallPath_RootItself_Allowed(t *testing.T) {
	root := t.TempDir()
	if !isAllowedInstallPath(root, []string{root}) {
		t.Errorf("expected root itself %q to be allowed", root)
	}
}

func TestIsAllowedInstallPath_OutsideRoot_Rejected(t *testing.T) {
	root := t.TempDir()
	outside := "/etc"
	if isAllowedInstallPath(outside, []string{root}) {
		t.Errorf("expected path %q outside root %q to be rejected", outside, root)
	}
}

func TestIsAllowedInstallPath_TmpEvil_Rejected(t *testing.T) {
	root := t.TempDir()
	if isAllowedInstallPath("/tmp/evil", []string{root}) {
		t.Errorf("expected /tmp/evil to be rejected")
	}
}

func TestIsAllowedInstallPath_DotDotTraversal_Rejected(t *testing.T) {
	root := t.TempDir()
	// Construct a path that contains ".." to escape the root.
	escapePath := filepath.Join(root, "sub", "..", "..", "etc")
	if isAllowedInstallPath(escapePath, []string{root}) {
		t.Errorf("expected dotdot-traversal path %q to be rejected", escapePath)
	}
}

func TestIsAllowedInstallPath_SimilarPrefixName_Rejected(t *testing.T) {
	// "/var/lib/qorven/apps" should NOT match "/var/lib/qorven/apps-evil"
	root := t.TempDir()
	// Create a sibling dir with the same prefix + extra chars.
	parent := filepath.Dir(root)
	base := filepath.Base(root)
	sibling := filepath.Join(parent, base+"-evil")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if isAllowedInstallPath(sibling, []string{root}) {
		t.Errorf("sibling dir with extended name %q should not match root %q", sibling, root)
	}
}

func TestIsAllowedInstallPath_EmptyRoots_Rejected(t *testing.T) {
	root := t.TempDir()
	if isAllowedInstallPath(root, nil) {
		t.Errorf("expected rejection when allowedRoots is nil")
	}
	if isAllowedInstallPath(root, []string{}) {
		t.Errorf("expected rejection when allowedRoots is empty")
	}
}

func TestIsAllowedInstallPath_SymlinkEscape_Rejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Create a symlink inside root that points outside.
	linkPath := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	// The resolved path is "outside", which is not under root.
	if isAllowedInstallPath(linkPath, []string{root}) {
		t.Errorf("symlink escaping to %q should be rejected when root is %q", outside, root)
	}
}

func TestIsAllowedInstallPath_NonExistentSubdir_AllowedWhenParentUnderRoot(t *testing.T) {
	root := t.TempDir()
	// A path that doesn't exist yet but would be under root.
	notYet := filepath.Join(root, "apps", "new-app")
	if !isAllowedInstallPath(notYet, []string{root}) {
		t.Errorf("non-existent subdir %q under root %q should be allowed (will be created by Install)", notYet, root)
	}
}
