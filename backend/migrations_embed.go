// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package main

import (
	"embed"
	"io/fs"
	"log/slog"
)

//go:embed migrations
var embeddedMigrationsDir embed.FS

// EmbeddedMigrations is the sub-FS rooted at migrations/.
// Passed to store.DB.MigrateUpFS so the binary is self-contained on
// any fresh install where no external migrations/ directory exists.
var EmbeddedMigrations fs.FS

func init() {
	sub, err := fs.Sub(embeddedMigrationsDir, "migrations")
	if err != nil {
		slog.Error("failed to init embedded migrations FS", "error", err)
		return
	}
	EmbeddedMigrations = sub
}
