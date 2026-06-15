// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build linux

package apps

import (
	"os/exec"
	"syscall"
)

// setProcGroup puts the command in its own process group (Linux only).
// It also sets Pdeathsig so that if the parent thread exits (e.g. gateway
// crashes) the child receives SIGKILL rather than being reparented to init.
// Pdeathsig is a Linux-only field; see proc_bsd.go for the darwin/BSD path.
// If a dedicated "qorven-app" OS user exists, the child is started under
// that uid/gid to drop privileges; otherwise the gateway uid is inherited.
func setProcGroup(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:    true,
		Pdeathsig:  syscall.SIGKILL,
		Credential: appSubprocCredential(), // nil = no drop, which is safe
	}
}

// killGroup sends SIGKILL to the process group.
func killGroup(c *exec.Cmd) {
	if c.Process != nil {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}
