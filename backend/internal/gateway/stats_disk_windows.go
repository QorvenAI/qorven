//go:build windows

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func readDiskGB() (usedGB, totalGB float64) {
	root, _ := windows.UTF16PtrFromString(`C:\`)
	var free, total, avail uint64
	err := windows.GetDiskFreeSpaceEx(root,
		(*uint64)(unsafe.Pointer(&avail)),
		(*uint64)(unsafe.Pointer(&total)),
		(*uint64)(unsafe.Pointer(&free)),
	)
	if err != nil {
		return
	}
	totalGB = float64(total) / 1e9
	usedGB = float64(total-avail) / 1e9
	return
}
