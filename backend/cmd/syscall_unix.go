//go:build !windows

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"syscall"
)

func flushStdinNonblock() {
	syscall.SetNonblock(int(os.Stdin.Fd()), true)
	buf := make([]byte, 256)
	os.Stdin.Read(buf)
	syscall.SetNonblock(int(os.Stdin.Fd()), false)
}

func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func diskFree(path string) string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "unknown"
	}
	free := stat.Bavail * uint64(stat.Bsize)
	if free > 1<<30 {
		return fmt.Sprintf("%.1fGB", float64(free)/float64(1<<30))
	}
	return fmt.Sprintf("%.0fMB", float64(free)/float64(1<<20))
}
