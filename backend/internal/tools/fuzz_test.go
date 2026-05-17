// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"testing"
)

func FuzzSafePath(f *testing.F) {
	f.Add("/tmp/workspace", "test.txt")
	f.Add("/tmp/workspace", "../../../etc/passwd")
	f.Add("/tmp/workspace", "%2e%2e%2fetc/passwd")
	f.Add("/tmp/workspace", "subdir/../../../etc/hosts")
	f.Add("/tmp/workspace", "/etc/passwd")
	f.Add("/tmp/workspace", "")

	f.Fuzz(func(t *testing.T, workspace, path string) {
		if workspace == "" { return }
		// Should never panic
		SafePath(workspace, path)
	})
}

func FuzzReadFileTool(f *testing.F) {
	f.Add("test.txt")
	f.Add("../../../etc/passwd")
	f.Add("")
	f.Add("subdir/file.go")

	dir := f.TempDir()
	tool := NewReadFileTool(dir)

	f.Fuzz(func(t *testing.T, path string) {
		tool.Execute(context.Background(), map[string]any{"path": path})
	})
}
