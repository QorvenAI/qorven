// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Confirm asks a yes/no question.
func Confirm(msg string, autoYes bool) bool {
	if autoYes || !IsInteractive() {
		return true
	}
	fmt.Printf("%s [y/N]: ", msg)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

// ReadContent reads from @file or returns literal string.
func ReadContent(val string) (string, error) {
	if strings.HasPrefix(val, "@") {
		data, err := os.ReadFile(val[1:])
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", val[1:], err)
		}
		return string(data), nil
	}
	return val, nil
}

// Input prompts for text input.
func Input(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}
