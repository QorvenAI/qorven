// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package apps

import (
	"os/user"
	"strconv"
	"syscall"
)

// appSubprocCredential returns the OS credential to drop privileges to when
// running app tool subprocesses. It looks up the dedicated "qorven-app" OS user;
// if that user does not exist (typical on dev boxes), it returns nil and the
// subprocess inherits the gateway process's uid/gid unchanged.
//
// This helper is shared between Linux (proc_linux.go) and BSD/darwin
// (proc_bsd.go). Windows uses a no-op path in proc_windows.go.
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
