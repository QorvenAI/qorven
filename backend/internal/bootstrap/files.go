// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package bootstrap loads workspace persona/context files and injects them
// into the agent's system prompt.
//
// Bootstrap files are loaded from the workspace directory at startup:
//
//	AGENTS.md   — operating instructions (every session)
//	SOUL.md     — persona, tone, boundaries
//	USER.md     — user profile
//	IDENTITY.md — agent name, emoji, creature, vibe
//	TOOLS.md    — local tool notes
//	BOOTSTRAP.md— first-run ritual (deleted after completion)
//	MEMORY.md   — long-term curated memory
package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
)

// Bootstrap filenames.
const (
	AgentsFile         = "AGENTS.md"
	SoulFile           = "SOUL.md"
	ToolsFile          = "TOOLS.md"
	IdentityFile       = "IDENTITY.md"
	UserFile           = "USER.md"
	UserPredefinedFile = "USER_PREDEFINED.md"
	BootstrapFile      = "BOOTSTRAP.md"
	DelegationFile     = "DELEGATION.md"
	TeamFile           = "TEAM.md"
	AvailabilityFile   = "AVAILABILITY.md"
	HeartbeatFile      = "HEARTBEAT.md"
	MemoryFile         = "MEMORY.md"
	MemoryAltFile      = "memory.md"
	MemoryJSONFile     = "MEMORY.json"
)

// standardFiles is the ordered list of bootstrap files to load.
var standardFiles = []string{AgentsFile, SoulFile, ToolsFile, IdentityFile, UserFile, BootstrapFile}

// minimalAllowlist is the set of files loaded for subagent/cron sessions.
var minimalAllowlist = map[string]bool{AgentsFile: true, ToolsFile: true}

// File represents a workspace bootstrap file loaded from disk.
type File struct {
	Name    string // filename (e.g. "AGENTS.md")
	Path    string // absolute path
	Content string // file content (empty if missing)
	Missing bool   // true if file doesn't exist on disk
}

// ContextFile is the truncated version ready for system prompt injection.
type ContextFile struct {
	Path    string // display path (e.g. "SOUL.md")
	Content string // truncated content
}

// LoadWorkspaceFiles reads all recognized bootstrap files from a workspace directory.
// Files are returned in a fixed order. Missing files are included with Missing=true.
func LoadWorkspaceFiles(workspaceDir string) []File {
	files := []File{}
	for _, name := range standardFiles {
		files = append(files, loadFile(workspaceDir, name))
	}
	// Load MEMORY.md (try MEMORY.md first, then memory.md)
	memFile := loadFile(workspaceDir, MemoryFile)
	if memFile.Missing {
		memFile = loadFile(workspaceDir, MemoryAltFile)
	}
	files = append(files, memFile)
	return files
}

// FilterForSession filters bootstrap files based on session type.
// Normal sessions get all files. Subagent/cron/heartbeat sessions get only AGENTS.md and TOOLS.md.
func FilterForSession(files []File, sessionKey string) []File {
	if !IsSubagentSession(sessionKey) && !IsCronSession(sessionKey) && !IsHeartbeatSession(sessionKey) {
		return files
	}
	filtered := []File{}
	for _, f := range files {
		if minimalAllowlist[f.Name] {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// IsSubagentSession checks if a session key indicates a subagent session.
func IsSubagentSession(sessionKey string) bool {
	return strings.HasPrefix(strings.ToLower(sessionRest(sessionKey)), "subagent:")
}

// IsCronSession checks if a session key indicates a cron session.
func IsCronSession(sessionKey string) bool {
	return strings.HasPrefix(strings.ToLower(sessionRest(sessionKey)), "cron:")
}

// IsHeartbeatSession checks if a session key indicates a heartbeat session.
func IsHeartbeatSession(sessionKey string) bool {
	return strings.HasPrefix(sessionRest(sessionKey), "heartbeat")
}

// IsTeamSession checks if a session key indicates a team-dispatched task session.
func IsTeamSession(sessionKey string) bool {
	return strings.HasPrefix(strings.ToLower(sessionRest(sessionKey)), "team:")
}

// sessionRest extracts the rest part after "agent:{agentId}:" from a session key.
func sessionRest(sessionKey string) string {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) < 3 || parts[0] != "agent" {
		return ""
	}
	return parts[2]
}

func loadFile(dir, name string) File {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return File{Name: name, Path: path, Missing: true}
	}
	return File{Name: name, Path: path, Content: string(data)}
}
