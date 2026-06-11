// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// verifyForMarkers maps detected project-root marker files to a verify command
// run after a code edit. Pure (testable). "" = no verify available (safe no-op).
func verifyForMarkers(markers map[string]bool) string {
	switch {
	case markers["go.mod"]:
		return "go build ./... && go vet ./..."
	case markers["package.json"]:
		return "npm run build --if-present"
	case markers["pyproject.toml"], markers["requirements.txt"], markers["setup.py"]:
		return "python -m compileall -q ."
	default:
		return ""
	}
}

// detectVerifyCommand stats dir for known project markers and returns the verify
// command, or "" if none recognized.
func detectVerifyCommand(dir string) string {
	candidates := []string{"go.mod", "package.json", "pyproject.toml", "requirements.txt", "setup.py"}
	markers := map[string]bool{}
	for _, m := range candidates {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			markers[m] = true
		}
	}
	return verifyForMarkers(markers)
}

// runVerify runs the detected verify command in dir with a bounded timeout.
// Returns combined output and whether it succeeded. A no-op ("") always succeeds.
func runVerify(ctx context.Context, dir, cmd string) (string, bool) {
	if cmd == "" {
		return "", true
	}
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	c := exec.CommandContext(runCtx, "bash", "-c", cmd)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err == nil
}
