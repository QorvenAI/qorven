// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// listProjectFiles lists files synchronously for code mode.
func listProjectFiles(dir string) []string {
	var files []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" || name == "target" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, path)
		if len(files) >= 200 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// Messages for code mode
type codeFileLoadedMsg struct{ path, content string }
type codeFilesListedMsg struct{ files []string }

// loadFileCmd reads a file and returns its content.
func (m *Model) loadFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return codeFileLoadedMsg{path: path, content: "Error: " + err.Error()}
		}
		return codeFileLoadedMsg{path: path, content: string(data)}
	}
}

// listProjectCmd lists files in the project directory.
func (m *Model) listProjectCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		var files []string
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			// Skip hidden dirs and common noise
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" || name == "target" {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.Join(dir, rel))
			if len(files) >= 200 {
				return filepath.SkipAll
			}
			return nil
		})
		sort.Strings(files)
		return codeFilesListedMsg{files: files}
	}
}

// handleCodeMsg processes code mode messages in Update.
func (m *Model) handleCodeMsg(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case codeFileLoadedMsg:
		m.codePath = msg.path
		m.codeContent = msg.content
		return true
	case codeFilesListedMsg:
		m.codeFiles = msg.files
		m.codeCursor = 0
		return true
	}
	return false
}

// Ensure json import is used (for future API calls)
var _ = json.Marshal
