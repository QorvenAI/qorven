// Copyright 2026 Qorven AI. All rights reserved.
package gateway

import "testing"

func TestWorkspaceFileEditable(t *testing.T) {
	for _, n := range []string{"SOUL.md", "IDENTITY.md", "USER.md", "AGENTS.md", "TOOLS.md", "MEMORY.md"} {
		if !workspaceFileEditable(n) {
			t.Errorf("%s should be editable", n)
		}
	}
	for _, n := range []string{"../etc/passwd", "SOUL.md/../x", "random.txt", "", "sub/dir.md"} {
		if workspaceFileEditable(n) {
			t.Errorf("%s must NOT be editable (traversal/unknown)", n)
		}
	}
}
