//go:build !windows

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import "syscall"

func readDiskGB() (usedGB, totalGB float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		totalGB = float64(total) / 1e9
		usedGB = float64(total-free) / 1e9
	}
	return
}
