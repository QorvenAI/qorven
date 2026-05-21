// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package gateway

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// migrateBinaryToOpt moves the Qorven binary from /usr/local/bin/qorven to
// /opt/qorven/bin/qorven on existing installs and replaces the old path with a
// symlink. This one-time migration ensures the service user owns the binary's
// parent directory, which is required for atomic GUI self-updates without sudo.
//
// Safe to call on every startup — exits immediately if already migrated.
func migrateBinaryToOpt() {
	const (
		oldPath = "/usr/local/bin/qorven"
		binDir  = "/opt/qorven/bin"
		newPath = "/opt/qorven/bin/qorven"
	)

	// Already migrated: old path is a symlink.
	if fi, err := os.Lstat(oldPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return
	}

	// Old path must be a regular file for migration to proceed.
	fi, err := os.Lstat(oldPath)
	if err != nil || !fi.Mode().IsRegular() {
		return
	}

	// New path already owns this binary — just need the symlink.
	if _, err := os.Stat(newPath); err == nil {
		sudo, _ := exec.LookPath("sudo")
		if sudo != "" {
			exec.Command(sudo, "ln", "-sf", newPath, oldPath).Run()
		}
		return
	}

	slog.Info("update.migrate_binary", "from", oldPath, "to", newPath)

	sudo, err := exec.LookPath("sudo")
	if err != nil {
		slog.Warn("update.migrate_binary_skipped", "reason", "sudo not found")
		return
	}

	if out, err := exec.Command(sudo, "mkdir", "-p", binDir).CombinedOutput(); err != nil {
		slog.Warn("update.migrate_binary_mkdir_failed", "err", err, "out", string(out))
		return
	}
	if out, err := exec.Command(sudo, "cp", "-f", oldPath, newPath).CombinedOutput(); err != nil {
		slog.Warn("update.migrate_binary_cp_failed", "err", err, "out", string(out))
		return
	}

	exec.Command(sudo, "chmod", "0755", newPath).Run()
	exec.Command(sudo, "chown", "qorven:qorven", binDir).Run()
	exec.Command(sudo, "chown", "qorven:qorven", newPath).Run()
	exec.Command(sudo, "ln", "-sf", newPath, oldPath).Run()

	patchUnitForOptBin()
	slog.Info("update.migrate_binary_done", "path", newPath, "symlink", oldPath)
}

// patchUnitForOptBin updates an existing systemd unit that still references
// /usr/local/bin to use /opt/qorven/bin instead.
func patchUnitForOptBin() {
	const unitPath = "/etc/systemd/system/qorven.service"
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return
	}
	updated := string(data)
	if !strings.Contains(updated, "/usr/local/bin/qorven") {
		return
	}
	updated = strings.ReplaceAll(updated, "ExecStart=/usr/local/bin/qorven", "ExecStart=/opt/qorven/bin/qorven")
	updated = strings.ReplaceAll(updated, "/usr/local/bin\n", "/opt/qorven/bin\n")
	updated = strings.ReplaceAll(updated, "/usr/local/bin\r\n", "/opt/qorven/bin\r\n")
	// Handle ReadWritePaths that ends with /usr/local/bin at end of line or followed by space
	updated = strings.ReplaceAll(updated, "ReadWritePaths=/var/lib/qorven /etc/qorven /usr/local/bin", "ReadWritePaths=/var/lib/qorven /etc/qorven /opt/qorven/bin")

	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return
	}
	tmp := filepath.Join(os.TempDir(), "qorven.service.migrate")
	if err := os.WriteFile(tmp, []byte(updated), 0644); err != nil {
		return
	}
	defer os.Remove(tmp)
	exec.Command(sudo, "cp", tmp, unitPath).Run()
	exec.Command(sudo, "systemctl", "daemon-reload").Run()
	slog.Info("update.unit_patched_for_opt_bin")
}
