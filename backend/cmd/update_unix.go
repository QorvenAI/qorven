// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package cmd

// stopService is a no-op on Unix — the running binary can be replaced
// without stopping the service (Linux allows writes to open executables
// via the rename-into-place pattern).
func stopService() error { return nil }

// startService is a no-op on Unix — systemd's Restart=always restarts
// the process after it exits following the binary swap.
func startService() error { return nil }
