// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupConfig controls automatic cleanup of old data.
type CleanupConfig struct {
	WorkspaceMaxAge time.Duration // delete workspaces for deleted agents
	SessionMaxAge   time.Duration // delete sessions older than this
	SandboxMaxAge   time.Duration // delete sandbox runs older than this
}

func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		WorkspaceMaxAge: 7 * 24 * time.Hour,   // 7 days
		SessionMaxAge:   30 * 24 * time.Hour,   // 30 days
		SandboxMaxAge:   7 * 24 * time.Hour,    // 7 days
	}
}

// RunCleanup performs periodic cleanup of old workspaces, sessions, and sandbox runs.
func RunCleanup(ctx context.Context, pool *pgxpool.Pool, cfg CleanupConfig) {
	// Clean old sandbox runs
	if pool != nil {
		tag, err := pool.Exec(ctx,
			`DELETE FROM sandbox_runs WHERE created_at < NOW() - $1::interval`,
			cfg.SandboxMaxAge.String())
		if err == nil {
			slog.Info("cleanup.sandbox_runs", "deleted", tag.RowsAffected())
		}

		// Clean old sessions (archived only)
		tag, err = pool.Exec(ctx,
			`DELETE FROM sessions WHERE status = 'archived' AND updated_at < NOW() - $1::interval`,
			cfg.SessionMaxAge.String())
		if err == nil {
			slog.Info("cleanup.sessions", "deleted", tag.RowsAffected())
		}

		// Expire old outbound approvals
		pool.Exec(ctx, `UPDATE outbound_queue SET status = 'expired' WHERE status = 'pending' AND expires_at < NOW()`)
	}

	// Clean orphaned workspace directories
	root := WorkspaceRoot()
	entries, err := os.ReadDir(root)
	if err != nil { return }
	for _, entry := range entries {
		if !entry.IsDir() { continue }
		info, err := entry.Info()
		if err != nil { continue }
		if time.Since(info.ModTime()) > cfg.WorkspaceMaxAge {
			ws := filepath.Join(root, entry.Name())
			os.RemoveAll(ws)
			slog.Info("cleanup.workspace", "path", ws, "age", time.Since(info.ModTime()).Round(time.Hour))
		}
	}
}
