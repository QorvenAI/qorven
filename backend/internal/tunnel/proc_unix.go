// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package tunnel

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the command in its own process group (Unix only) so that
// killGroup can terminate the cloudflared process and any children it spawns.
func setProcGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup sends SIGKILL to the process group.
func killGroup(c *exec.Cmd) {
	if c.Process != nil {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
