// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package config

import (
	"os"
	"sync"
)

var (
	inDockerOnce   sync.Once
	inDockerResult bool
)

// InDocker returns true if running inside a Docker container.
func InDocker() bool {
	inDockerOnce.Do(func() {
		// Check /.dockerenv (most reliable)
		if _, err := os.Stat("/.dockerenv"); err == nil {
			inDockerResult = true
			return
		}
		// Check cgroup (fallback)
		if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
			if containsDocker(data) {
				inDockerResult = true
			}
		}
	})
	return inDockerResult
}

// DockerLocalhost returns "host.docker.internal" if in Docker, else "localhost".
func DockerLocalhost() string {
	if InDocker() {
		return "host.docker.internal"
	}
	return "localhost"
}

func containsDocker(data []byte) bool {
	for i := 0; i+5 < len(data); i++ {
		if string(data[i:i+6]) == "docker" {
			return true
		}
	}
	return false
}
