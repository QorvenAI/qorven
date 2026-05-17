// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultWorkspaceRoot is the base directory for all agent workspaces.
	// Production: /var/lib/qorven/workspaces
	// Development: /tmp/qorven-workspaces
	DefaultWorkspaceRoot = "/tmp/qorven-workspaces"

	// MaxFileSize is the maximum size of a single file write (100MB).
	MaxFileSize = 100 * 1024 * 1024

	// MaxWorkspaceSize is the maximum total size of an agent's workspace (500MB).
	MaxWorkspaceSize = 500 * 1024 * 1024
)

// WorkspaceRoot returns the configured workspace root directory.
// Uses QORVEN_WORKSPACE_ROOT env var if set, otherwise DefaultWorkspaceRoot.
func WorkspaceRoot() string {
	if root := os.Getenv("QORVEN_WORKSPACE_ROOT"); root != "" {
		return root
	}
	return DefaultWorkspaceRoot
}

// AgentWorkspace returns the workspace path for a specific agent.
// Creates the directory if it doesn't exist.
func AgentWorkspace(agentID string) string {
	ws := filepath.Join(WorkspaceRoot(), agentID)
	os.MkdirAll(ws, 0755)
	return ws
}

// CleanWorkspace removes an agent's workspace directory.
func CleanWorkspace(agentID string) error {
	ws := filepath.Join(WorkspaceRoot(), agentID)
	return os.RemoveAll(ws)
}

// WorkspaceSize calculates the total size of an agent's workspace.
func WorkspaceSize(agentID string) (int64, error) {
	ws := filepath.Join(WorkspaceRoot(), agentID)
	var total int64
	err := filepath.Walk(ws, func(_ string, info os.FileInfo, err error) error {
		if err != nil { return err }
		if !info.IsDir() { total += info.Size() }
		return nil
	})
	return total, err
}

// CheckWorkspaceQuota returns an error if the workspace exceeds the quota.
func CheckWorkspaceQuota(agentID string) error {
	size, err := WorkspaceSize(agentID)
	if err != nil { return nil } // can't check = allow
	if size > MaxWorkspaceSize {
		return fmt.Errorf("workspace quota exceeded: %dMB / %dMB. Delete unused files to free space",
			size/(1024*1024), MaxWorkspaceSize/(1024*1024))
	}
	return nil
}

// CheckFileSize returns an error if the content exceeds the max file size.
func CheckFileSize(content string) error {
	if len(content) > MaxFileSize {
		return fmt.Errorf("file too large: %dMB (max %dMB)",
			len(content)/(1024*1024), MaxFileSize/(1024*1024))
	}
	return nil
}
