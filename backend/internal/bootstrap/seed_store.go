// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/store"
)

// retryOnBusy retries fn up to 3 times on SQLITE_BUSY errors with 500ms delay.
func retryOnBusy(fn func() error) error {
	var lastErr error
	for attempt := range 3 {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !strings.Contains(lastErr.Error(), "SQLITE_BUSY") && !strings.Contains(lastErr.Error(), "database is locked") {
			return lastErr
		}
		if attempt < 2 {
			slog.Warn("bootstrap: retrying after SQLITE_BUSY", "attempt", attempt+1)
			time.Sleep(500 * time.Millisecond)
		}
	}
	return lastErr
}

// SeedToStore seeds embedded templates into agent_context_files (agent-level).
// Used for predefined agents only — open agents get per-user files via SeedUserFiles.
// Only writes files that don't already have content.
func SeedToStore(ctx context.Context, agentStore store.AgentStore, agentID uuid.UUID, agentType string) ([]string, error) {
	if agentType == store.AgentTypeOpen {
		return nil, nil
	}

	existing, err := agentStore.GetAgentContextFiles(ctx, agentID)
	if err != nil {
		slog.Warn("bootstrap: failed to check existing agent files", "agent", agentID, "error", err)
		return nil, err
	}

	hasContent := make(map[string]bool)
	for _, f := range existing {
		if f.Content != "" {
			hasContent[f.FileName] = true
		}
	}

	seeded := []string{}
	for _, name := range templateFiles {
		if name == UserFile || name == ToolsFile || hasContent[name] {
			continue
		}
		content, err := templateFS.ReadFile(filepath.Join("templates", name))
		if err != nil {
			slog.Warn("bootstrap: failed to read embedded template", "file", name, "error", err)
			continue
		}
		if err := retryOnBusy(func() error { return agentStore.SetAgentContextFile(ctx, agentID, name, string(content)) }); err != nil {
			return seeded, err
		}
		seeded = append(seeded, name)
	}

	// Seed USER_PREDEFINED.md for predefined agents
	if !hasContent[UserPredefinedFile] {
		content, err := templateFS.ReadFile(filepath.Join("templates", UserPredefinedFile))
		if err == nil {
			if err := retryOnBusy(func() error { return agentStore.SetAgentContextFile(ctx, agentID, UserPredefinedFile, string(content)) }); err != nil {
				return seeded, err
			}
			seeded = append(seeded, UserPredefinedFile)
		}
	}

	if len(seeded) > 0 {
		slog.Info("seeded agent context files to store", "agent", agentID, "files", seeded)
	}
	return seeded, nil
}

// userSeedFilesOpen is the full set of files seeded per-user for open agents.
var userSeedFilesOpen = []string{AgentsFile, SoulFile, IdentityFile, UserFile, BootstrapFile}

// userSeedFilesPredefined is the set of files seeded per-user for predefined agents.
var userSeedFilesPredefined = []string{UserFile, BootstrapFile}

// SeedUserFiles seeds embedded templates into user_context_files for a new user.
// For "open" agents: all 5 files (including BOOTSTRAP.md).
// For "predefined" agents: USER.md + BOOTSTRAP.md.
// Only writes files that don't already exist — safe to call multiple times.
func SeedUserFiles(ctx context.Context, agentStore store.AgentStore, agentID uuid.UUID, userID, agentType string, skipIfAnyExist bool) ([]string, error) {
	files := userSeedFilesOpen
	if agentType == store.AgentTypePredefined {
		files = userSeedFilesPredefined
	}

	existing, err := agentStore.GetUserContextFiles(ctx, agentID, userID)
	if err != nil {
		slog.Warn("bootstrap: failed to check existing user files", "agent", agentID, "user", userID, "error", err)
		return nil, err
	}

	if skipIfAnyExist && len(existing) > 0 {
		slog.Debug("bootstrap: skip user seed (existing files)", "agent", agentID, "user", userID, "existing", len(existing))
		return nil, nil
	}

	hasFile := make(map[string]bool, len(existing))
	for _, f := range existing {
		if f.Content != "" {
			hasFile[f.FileName] = true
		}
	}

	// For predefined agents: load agent-level files to use as seed fallback
	var agentLevelFiles map[string]string
	if agentType == store.AgentTypePredefined {
		agentFiles, err := agentStore.GetAgentContextFiles(ctx, agentID)
		if err == nil && len(agentFiles) > 0 {
			agentLevelFiles = make(map[string]string, len(agentFiles))
			for _, f := range agentFiles {
				if f.Content != "" {
					agentLevelFiles[f.FileName] = f.Content
				}
			}
		}
	}

	seeded := []string{}
	for _, name := range files {
		if hasFile[name] {
			continue
		}

		// For predefined agents seeding USER.md: prefer agent-level content
		if agentType == store.AgentTypePredefined && name == UserFile {
			if agentContent, ok := agentLevelFiles[name]; ok {
				if err := retryOnBusy(func() error { return agentStore.SetUserContextFile(ctx, agentID, userID, name, agentContent) }); err != nil {
					return seeded, err
				}
				seeded = append(seeded, name)
				continue
			}
		}

		templateName := name
		if agentType == store.AgentTypePredefined && name == BootstrapFile {
			templateName = "BOOTSTRAP_PREDEFINED.md"
		}

		content, err := templateFS.ReadFile(filepath.Join("templates", templateName))
		if err != nil {
			slog.Warn("bootstrap: failed to read embedded template for user seed", "file", name, "error", err)
			continue
		}

		if err := retryOnBusy(func() error { return agentStore.SetUserContextFile(ctx, agentID, userID, name, string(content)) }); err != nil {
			return seeded, err
		}
		seeded = append(seeded, name)
	}

	if len(seeded) > 0 {
		slog.Info("seeded user context files", "agent", agentID, "user", userID, "type", agentType, "files", seeded)
	}
	return seeded, nil
}

// EmbeddedUserFiles returns in-memory context files from embedded templates.
// Used as a fallback when DB seeding fails so the first turn still gets bootstrap onboarding.
func EmbeddedUserFiles(agentType string) []ContextFile {
	files := userSeedFilesOpen
	if agentType == store.AgentTypePredefined {
		files = userSeedFilesPredefined
	}
	result := []ContextFile{}
	for _, name := range files {
		templateName := name
		if agentType == store.AgentTypePredefined && name == BootstrapFile {
			templateName = "BOOTSTRAP_PREDEFINED.md"
		}
		content, err := templateFS.ReadFile(filepath.Join("templates", templateName))
		if err != nil {
			continue
		}
		result = append(result, ContextFile{Path: name, Content: string(content)})
	}
	return result
}
