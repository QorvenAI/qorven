// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package gateway

import (
	"os"
	"syscall"
)

// selfExit sends SIGTERM to the current process. On Linux/macOS with
// systemd/launchd Restart=always, the service manager restarts the new binary.
func selfExit() {
	syscall.Kill(os.Getpid(), syscall.SIGTERM) //nolint:errcheck
}
