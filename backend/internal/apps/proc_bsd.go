// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !linux && !windows

package apps

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the command in its own process group on darwin/BSD.
// Pdeathsig is a Linux-only syscall field and is intentionally omitted here;
// on macOS/BSD the child will be reparented to launchd on parent exit (the
// gateway always calls killGroup on cleanup so orphan risk is low in practice).
// If a dedicated "qorven-app" OS user exists the child is started under that
// uid/gid to drop privileges; otherwise the gateway uid is inherited.
func setProcGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:    true,
		Credential: appSubprocCredential(), // nil = no drop, which is safe
	}
}

// killGroup sends SIGKILL to the process group.
func killGroup(c *exec.Cmd) {
	if c.Process != nil {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
