// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build windows

package gateway

import "os"

// selfExit exits cleanly. NSSM (the Windows service wrapper installed by
// install.ps1) has Start=SERVICE_AUTO_START and will restart the process.
func selfExit() {
	os.Exit(0)
}
