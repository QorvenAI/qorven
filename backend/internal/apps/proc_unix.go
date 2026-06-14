// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package apps

import (
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// appSubprocCredential returns the OS credential to drop privileges to when
// running app tool subprocesses. It looks up the dedicated "qorven-app" OS user;
// if that user does not exist (typical on dev boxes), it returns nil and the
// subprocess inherits the gateway process's uid/gid unchanged.
//
// Note: Go's syscall.SysProcAttr on Linux does not expose a NoNewPrivileges
// field (it was not added to the stdlib syscall package). We use Pdeathsig
// (SIGKILL on parent thread exit) as a belt-and-suspenders containment measure
// instead. Rlimits per-child are not portably settable via SysProcAttr in Go;
// they are a noted follow-up (use systemd slice limits or a wrapper binary).
func appSubprocCredential() *syscall.Credential {
	u, err := user.Lookup("qorven-app")
	if err != nil {
		// Dedicated OS user does not exist — run as the gateway uid (acceptable).
		return nil
	}
	uid, err1 := strconv.ParseUint(u.Uid, 10, 32)
	gid, err2 := strconv.ParseUint(u.Gid, 10, 32)
	if err1 != nil || err2 != nil {
		return nil
	}
	return &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}
}

// setProcGroup puts the command in its own process group (Unix only).
// It also sets Pdeathsig so that if the parent thread exits (e.g. gateway
// crashes) the child receives SIGKILL rather than being reparented to init.
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
