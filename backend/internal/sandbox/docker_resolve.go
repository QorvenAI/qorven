// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package sandbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// resolveHostWorkspacePath maps a container-local path to its host-side
// equivalent for Docker-out-of-Docker (DooD) sibling container mounts.
// When Qorven runs inside a container and spawns sandbox containers, the
// workspace path only exists inside the Qorven container — the sandbox needs
// the corresponding host path or volume name to mount it correctly.
func resolveHostWorkspacePath(ctx context.Context, localPath string) string {
	// Not in a container — local path is the host path.
	if _, err := os.Stat("/.dockerenv"); err != nil {
		return localPath
	}

	containerID := detectContainerID()
	if containerID == "" {
		slog.Error("sandbox.resolve: cannot determine container ID — DooD volume mounts will fail", "path", localPath)
		return localPath
	}

	inspectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(inspectCtx, "docker", "inspect", "--format", "{{json .Mounts}}", containerID).Output()
	if err != nil {
		slog.Warn("sandbox.resolve: docker inspect failed", "container", containerID, "error", err)
		return localPath
	}

	var mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Name        string `json:"Name"`
	}
	if err := json.Unmarshal(out, &mounts); err != nil {
		slog.Warn("sandbox.resolve: failed to parse mounts", "error", err)
		return localPath
	}

	targetDir := filepath.Clean(localPath)

	// Resolve symlinks
	if resolved, err := filepath.EvalSymlinks(localPath); err == nil {
		resolvedClean := filepath.Clean(resolved)
		if resolvedClean != targetDir {
			slog.Info("sandbox.resolve: symlink resolved", "path", localPath, "resolved", resolvedClean)
		}
		targetDir = resolvedClean
	}

	var bestDest, bestSource, bestRel, bestType, bestName string

	for _, m := range mounts {
		dest := filepath.Clean(m.Destination)
		if targetDir == dest || strings.HasPrefix(targetDir, dest+string(filepath.Separator)) {
			if len(dest) > len(bestDest) {
				bestDest = dest
				bestSource = m.Source
				bestType = m.Type
				bestName = m.Name
				bestRel, _ = filepath.Rel(dest, targetDir)
			}
		}
	}

	if bestDest == "" {
		slog.Warn("sandbox.resolve: no matching mount found", "path", localPath, "container", containerID)
		return localPath
	}

	// Named volume
	if bestType == "volume" && bestName != "" {
		if bestRel == "." {
			slog.Debug("sandbox.resolve: resolved to named volume", "path", localPath, "volume", bestName)
			return bestName
		}
		if bestSource != "" {
			return filepath.Join(bestSource, bestRel)
		}
	}

	// Bind mount
	if bestSource != "" {
		resolved := filepath.Join(bestSource, bestRel)
		slog.Debug("sandbox.resolve: resolved to host path", "path", localPath, "host", resolved)
		return resolved
	}

	slog.Warn("sandbox.resolve: mount found but no source path", "path", localPath, "mount", bestDest)
	return localPath
}

// detectContainerID returns the current Docker container ID.
func detectContainerID() string {
	// Strategy 1: Parse /proc/self/mountinfo
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if _, after, ok := strings.Cut(line, "/docker/containers/"); ok {
				if slashIdx := strings.IndexByte(after, '/'); slashIdx > 0 {
					id := after[:slashIdx]
					if len(id) >= 12 {
						return id
					}
				}
			}
		}
	}

	// Strategy 2: HOSTNAME env var
	if h := os.Getenv("HOSTNAME"); h != "" {
		return h
	}

	// Strategy 3: os.Hostname()
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}

	return ""
}
