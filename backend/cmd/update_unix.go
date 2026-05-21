// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package cmd

import (
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

// stopService is a no-op on Unix — the running binary can be replaced
// without stopping the service (Linux allows writes to open executables
// via the rename-into-place pattern).
func stopService() error { return nil }

// startService is a no-op on Unix — systemd's Restart=always restarts
// the process after it exits following the binary swap.
func startService() error { return nil }

// ensureOptBin migrates the Qorven binary from /usr/local/bin/ to
// /opt/qorven/bin/ on existing installs. Called from `qorven update`, which
// runs as the ubuntu/ec2-user with passwordless sudo — outside the systemd
// sandbox where NoNewPrivileges=yes would block escalation.
func ensureOptBin() {
	const (
		oldPath = "/usr/local/bin/qorven"
		binDir  = "/opt/qorven/bin"
		newPath = "/opt/qorven/bin/qorven"
	)

	// Already migrated: old path is a symlink.
	if fi, err := os.Lstat(oldPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return
	}

	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return
	}

	// Create /opt/qorven/bin owned by the qorven service user.
	if out, err := exec.Command(sudo, "mkdir", "-p", binDir).CombinedOutput(); err != nil {
		slog.Warn("update.migrate_mkdir_failed", "err", err, "out", string(out))
		return
	}

	// If new path doesn't exist yet, copy the binary there.
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		if out, err := exec.Command(sudo, "cp", "-f", oldPath, newPath).CombinedOutput(); err != nil {
			slog.Warn("update.migrate_cp_failed", "err", err, "out", string(out))
			return
		}
		exec.Command(sudo, "chmod", "0755", newPath).Run()
	}

	exec.Command(sudo, "chown", "qorven:qorven", binDir).Run()
	exec.Command(sudo, "chown", "qorven:qorven", newPath).Run()
	exec.Command(sudo, "ln", "-sf", newPath, oldPath).Run()

	// Patch the systemd unit to use /opt/qorven/bin.
	patchUnitForOptBin(sudo)
}

func patchUnitForOptBin(sudo string) {
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
	updated = strings.ReplaceAll(updated,
		"ReadWritePaths=/var/lib/qorven /etc/qorven /usr/local/bin",
		"ReadWritePaths=/var/lib/qorven /etc/qorven /opt/qorven/bin")
	updated = strings.ReplaceAll(updated,
		"ReadWritePaths=/var/lib/qorven /etc/qorven",
		"ReadWritePaths=/var/lib/qorven /etc/qorven /opt/qorven/bin")

	tmp, err := os.CreateTemp("", "qorven.service.*")
	if err != nil {
		return
	}
	tmp.WriteString(updated)
	tmp.Close()
	defer os.Remove(tmp.Name())

	exec.Command(sudo, "cp", tmp.Name(), unitPath).Run()
	exec.Command(sudo, "systemctl", "daemon-reload").Run()
}
