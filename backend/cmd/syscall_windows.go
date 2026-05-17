//go:build windows

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import "syscall"

func flushStdinNonblock() {} // no-op on Windows

func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func diskFree(path string) string {
	return "unknown"
}
