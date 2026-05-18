// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// IsInteractive returns true if stdin is a terminal.
func IsInteractive() bool {
	var termios [256]byte
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), 0x5401, uintptr(unsafe.Pointer(&termios[0])), 0, 0, 0)
	return err == 0
}

// Password prompts for masked input.
func Password(label string) (string, error) {
	fmt.Printf("%s: ", label)
	var old syscall.Termios
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), 0x5401, uintptr(unsafe.Pointer(&old)), 0, 0, 0); err != 0 {
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line), nil
	}
	raw := old
	raw.Lflag &^= syscall.ECHO
	syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), 0x5402, uintptr(unsafe.Pointer(&raw)), 0, 0, 0)
	defer func() {
		syscall.Syscall6(syscall.SYS_IOCTL, uintptr(syscall.Stdin), 0x5402, uintptr(unsafe.Pointer(&old)), 0, 0, 0)
		fmt.Println()
	}()
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line), nil
}
