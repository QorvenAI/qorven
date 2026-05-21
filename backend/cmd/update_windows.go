// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os/exec"
)

// stopService stops the Qorven Windows service before a binary swap.
// On Windows, a running executable cannot be overwritten — the service must
// be stopped first. net stop is available on all Windows versions.
func stopService() error {
	if out, err := exec.Command("net", "stop", "qorven").CombinedOutput(); err != nil {
		// Service may not be running (manual install) — not fatal.
		_ = out
	}
	return nil
}

// startService starts the Qorven Windows service after a binary swap.
func startService() error {
	if out, err := exec.Command("net", "start", "qorven").CombinedOutput(); err != nil {
		return fmt.Errorf("net start qorven: %w — output: %s", err, out)
	}
	return nil
}
