// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/bootstrap"
	"github.com/qorvenai/qorven/internal/store"
)

// UserProfileFunc creates/resolves a user's profile and workspace.
// Returns the effective workspace path and whether the profile is new.
type UserProfileFunc func(ctx context.Context, agentID uuid.UUID, userID, workspace, channel string) (effectiveWorkspace string, isNew bool, err error)

// SeedUserFilesFunc seeds per-user context files (BOOTSTRAP.md, USER.md, etc.).
type SeedUserFilesFunc func(ctx context.Context, agentID uuid.UUID, userID, agentType string, isNew bool) error

// BootstrapCleanupFunc removes BOOTSTRAP.md after enough conversation turns.
type BootstrapCleanupFunc func(ctx context.Context, agentID uuid.UUID, userID string) error

// ContextFileLoaderFunc loads context files dynamically per-request.
type ContextFileLoaderFunc func(ctx context.Context, agentID uuid.UUID, userID, agentType string) []bootstrap.ContextFile

// buildUserProfileCallback creates the user profile resolution callback.
// Wraps AgentContextStore.GetAgentContextFiles + profile creation.
func buildUserProfileCallback(agentStore store.AgentStore) UserProfileFunc {
	if agentStore == nil {
		return nil
	}
	return func(ctx context.Context, agentID uuid.UUID, userID, workspace, channel string) (string, bool, error) {
		// AgentProfileStore.GetOrCreateUserProfile handles profile creation
		isNew, effectiveWs, err := agentStore.GetOrCreateUserProfile(ctx, agentID, userID, workspace, channel)
		if err != nil {
			slog.Warn("user_profile.create_failed", "agent", agentID, "user", userID, "error", err)
			return workspace, false, err
		}
		return effectiveWs, isNew, nil
	}
}

// buildSeedUserFilesCallback creates the context file seeding callback.
// Seeds BOOTSTRAP.md, USER.md into user_context_files.
// isNew=true seeds all files; isNew=false only seeds if user has zero files.
func buildSeedUserFilesCallback(agentStore store.AgentStore) SeedUserFilesFunc {
	if agentStore == nil {
		return nil
	}
	return func(ctx context.Context, agentID uuid.UUID, userID, agentType string, isNew bool) error {
		_, err := bootstrap.SeedUserFiles(ctx, agentStore, agentID, userID, agentType, !isNew)
		if err != nil {
			slog.Warn("user_files.seed_failed", "agent", agentID, "user", userID, "error", err)
		}
		return err
	}
}

// buildBootstrapCleanupCallback creates a callback that removes BOOTSTRAP.md.
// Safety net after enough turns, in case the LLM didn't clear it.
func buildBootstrapCleanupCallback(agentStore store.AgentStore) BootstrapCleanupFunc {
	if agentStore == nil {
		return nil
	}
	return func(ctx context.Context, agentID uuid.UUID, userID string) error {
		return agentStore.DeleteUserContextFile(ctx, agentID, userID, bootstrap.BootstrapFile)
	}
}

// buildContextFileLoaderCallback creates the per-request context file loader.
// Loads agent-level + user-level context files for system prompt injection.
func buildContextFileLoaderCallback(agentStore store.AgentStore) ContextFileLoaderFunc {
	if agentStore == nil {
		return nil
	}
	return func(ctx context.Context, agentID uuid.UUID, userID, agentType string) []bootstrap.ContextFile {
		// Load agent-level files
		agentFiles, err := agentStore.GetAgentContextFiles(ctx, agentID)
		if err != nil {
			slog.Warn("context_files.load_failed", "agent", agentID, "error", err)
			return nil
		}

		result := []bootstrap.ContextFile{}
		for _, f := range agentFiles {
			result = append(result, bootstrap.ContextFile{Path: f.FileName, Content: f.Content})
		}

		// Load user-level files (for predefined agents)
		if agentType == store.AgentTypePredefined && userID != "" {
			userFiles, err := agentStore.GetUserContextFiles(ctx, agentID, userID)
			if err == nil {
				for _, f := range userFiles {
					result = append(result, bootstrap.ContextFile{Path: f.FileName, Content: f.Content})
				}
			}
		}

		return result
	}
}
