// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build windows

package gateway

import "os/exec"

// stopWindowsService stops the qorven Windows service so the binary can be
// overwritten. On Windows, open executables are locked by the OS.
func stopWindowsService() {
	exec.Command("net", "stop", "qorven").Run()
}
