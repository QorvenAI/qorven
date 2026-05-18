// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"log/slog"
	"os/exec"
	"sync"
)

// SandboxMode determines how commands are executed.
type ExecSandboxMode string

const (
	SandboxNone   ExecSandboxMode = "none"   // direct host execution (development only)
	SandboxDocker ExecSandboxMode = "docker" // Docker container with restrictions
)

var (
	dockerAvailable     bool
	dockerCheckOnce     sync.Once
	defaultSandboxMode  ExecSandboxMode = SandboxNone
)

// DetectSandboxMode checks if Docker is available and sets the default.
// Called once on startup.
func DetectSandboxMode() ExecSandboxMode {
	dockerCheckOnce.Do(func() {
		cmd := exec.Command("docker", "info")
		if err := cmd.Run(); err == nil {
			dockerAvailable = true
			defaultSandboxMode = SandboxDocker
			slog.Info("sandbox.docker_available", "default", "docker")
		} else {
			dockerAvailable = false
			defaultSandboxMode = SandboxNone
			slog.Warn("sandbox.docker_not_available", "default", "none",
				"warning", "Running without Docker sandbox — commands execute on host. Install Docker for production use.")
		}
	})
	return defaultSandboxMode
}

// IsDockerAvailable returns whether Docker is available on this system.
func IsDockerAvailable() bool {
	DetectSandboxMode()
	return dockerAvailable
}
