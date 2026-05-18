// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build windows

package apps

import "os/exec"

// setProcGroup is a no-op on Windows (process groups work differently).
func setProcGroup(_ *exec.Cmd) {}

// killGroup kills the process on Windows (no process group concept).
func killGroup(c *exec.Cmd) {
	if c.Process != nil {
		c.Process.Kill()
	}
}
