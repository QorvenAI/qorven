// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

// AgentWorkspace must reduce a crafted agentID to its base element so it can
// never escape the workspace root — defense-in-depth independent of any HTTP
// path-traversal middleware.
func TestAgentWorkspace_NoTraversal(t *testing.T) {
	root := filepath.Clean(WorkspaceRoot())
	for _, id := range []string{"../other", "../../etc", "a/../../b", "..", "../"} {
		ws := filepath.Clean(AgentWorkspace(id))
		if ws != root && !strings.HasPrefix(ws, root+string(filepath.Separator)) {
			t.Errorf("AgentWorkspace(%q) = %q escaped root %q", id, ws, root)
		}
	}
}
