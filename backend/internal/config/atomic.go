// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile writes data to path without leaving a partial file
// on crash. The classic failure we're avoiding: process crashes mid-
// write, leaves a truncated config.toml, next start reads an invalid
// file and bails with a parse error. The user's real config is gone.
//
// Implementation: write to a temp file in the same directory (same
// filesystem → rename is atomic), fsync to push bytes to disk, then
// rename over the target. If any step fails, the original file is
// untouched.
//
// Permissions: pass the mode you want the final file to have. The
// temp file is created with the same mode so `0o600` secrets don't
// briefly land as `0o644`.
//
// Usage: replace `os.WriteFile(path, data, 0o644)` with
//   `config.AtomicWriteFile(path, data, 0o644)`.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("atomic write: mkdir %s: %w", dir, err)
	}
	// CreateTemp in the target dir — cross-fs renames aren't atomic.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("atomic write: create temp: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup on any error path. The rename below makes
	// the file disappear (from tmpPath) so os.Remove becomes a no-op
	// on the happy path — harmless.
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("atomic write: write temp: %w", err)
	}
	// fsync forces data to disk before rename — otherwise a crash
	// after rename can still lose the write to the kernel's dirty
	// page cache.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("atomic write: fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("atomic write: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("atomic write: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("atomic write: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
