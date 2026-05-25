// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	dataDirOnce  sync.Once
	dataDirValue string
)

// DataDir returns the root Qorven data directory for the current user or service.
//
// Resolution order (first match wins):
//  1. QORVEN_DATA_DIR environment variable — explicit override, used by systemd
//     service units (e.g. QORVEN_DATA_DIR=/var/lib/qorven)
//  2. Legacy ~/.qorven — if it already exists, honour it so existing installs
//     continue working without migration
//  3. Platform-default XDG/OS data directory:
//     - Linux:   $XDG_DATA_HOME/qorven  (fallback: ~/.local/share/qorven)
//     - macOS:   ~/Library/Application Support/Qorven
//     - Windows: %APPDATA%\Qorven
//
// The result is cached after the first call. The directory is NOT created here;
// callers that need it to exist should call os.MkdirAll themselves.
func DataDir() string {
	dataDirOnce.Do(func() {
		dataDirValue = resolveDataDir()
	})
	return dataDirValue
}

// Sub returns a path inside DataDir(), e.g. Sub("apps") → /var/lib/qorven/apps.
func Sub(parts ...string) string {
	args := append([]string{DataDir()}, parts...)
	return filepath.Join(args...)
}

func resolveDataDir() string {
	// 1. Explicit env override (systemd, Docker, CI)
	if d := os.Getenv("QORVEN_DATA_DIR"); d != "" {
		return d
	}

	home, _ := os.UserHomeDir()

	// 2. Legacy path — honour existing installs
	if home != "" {
		legacy := filepath.Join(home, ".qorven")
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}

	// 3. Platform default
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Qorven")
		}
		if home != "" {
			return filepath.Join(home, "AppData", "Roaming", "Qorven")
		}
	case "darwin":
		if home != "" {
			return filepath.Join(home, "Library", "Application Support", "Qorven")
		}
	default: // Linux and everything else
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "qorven")
		}
		if home != "" {
			return filepath.Join(home, ".local", "share", "qorven")
		}
	}

	// Last resort: current directory (should never reach here in practice)
	return ".qorven"
}
