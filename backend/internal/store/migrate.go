// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MigrateUpFS runs all pending up-migrations. It tries migrationsDir on disk
// first; if the directory does not exist it falls back to embedded, which is
// the fs.FS passed by the binary's go:embed directive. Passing nil for
// embedded is fine — the fallback is simply skipped.
func (db *DB) MigrateUpFS(embedded fs.FS, migrationsDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY, dirty BOOLEAN NOT NULL DEFAULT false, applied_at TIMESTAMPTZ DEFAULT NOW())`)

	current, dirty, err := db.currentVersion(ctx)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("schema is dirty at version %d — run: qorven migrate force %d", current, current-1)
	}

	var files []migration
	if _, statErr := os.Stat(migrationsDir); statErr == nil {
		files, err = findMigrations(migrationsDir, "up")
	} else if embedded != nil {
		files, err = findMigrationsFS(embedded, "up")
	} else {
		return fmt.Errorf("migrations directory not found: %s", migrationsDir)
	}
	if err != nil {
		return err
	}

	applied := 0
	for _, f := range files {
		if f.version <= current {
			continue
		}
		slog.Info("applying migration", "version", f.version, "file", f.name)
		if err := db.applyMigrationFS(ctx, f, true); err != nil {
			return fmt.Errorf("migration %d failed: %w", f.version, err)
		}
		applied++
	}
	if applied == 0 {
		slog.Info("database is up to date", "version", current)
	} else {
		slog.Info("migrations applied", "count", applied)
	}
	return nil
}

// MigrateUp is kept for backward compatibility (used by the CLI migrate command).
func (db *DB) MigrateUp(migrationsDir string) error {
	return db.MigrateUpFS(nil, migrationsDir)
}

func (db *DB) MigrateDown(migrationsDir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	current, _, err := db.currentVersion(ctx)
	if err != nil {
		return err
	}
	if current == 0 {
		slog.Info("already at version 0")
		return nil
	}

	files, err := findMigrations(migrationsDir, "down")
	if err != nil {
		return err
	}

	for i := len(files) - 1; i >= 0; i-- {
		if files[i].version == current {
			slog.Info("rolling back", "version", files[i].version)
			return db.applyMigration(ctx, files[i], false)
		}
	}
	return fmt.Errorf("no down migration for version %d", current)
}

func (db *DB) MigrateForce(version int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)
		ON CONFLICT (version) DO UPDATE SET dirty = false`, version)
	if err != nil {
		// Table might not exist yet
		_, err = db.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INT PRIMARY KEY, dirty BOOLEAN NOT NULL DEFAULT false, applied_at TIMESTAMPTZ DEFAULT NOW())`)
		if err != nil {
			return err
		}
		_, err = db.Pool.Exec(ctx, `DELETE FROM schema_migrations`)
		if err != nil {
			return err
		}
		_, err = db.Pool.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)`, version)
	}
	slog.Info("forced version", "version", version)
	return err
}

func (db *DB) currentVersion(ctx context.Context) (int, bool, error) {
	var version int
	var dirty bool
	err := db.Pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&version, &dirty)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "no rows") {
			return 0, false, nil
		}
		return 0, false, err
	}
	return version, dirty, nil
}

func (db *DB) applyMigration(ctx context.Context, f migration, isUp bool) error {
	sql, err := os.ReadFile(f.path)
	if err != nil {
		return err
	}
	return db.execMigration(ctx, f, isUp, string(sql))
}

func (db *DB) applyMigrationFS(ctx context.Context, f migration, isUp bool) error {
	var sqlBytes []byte
	var err error
	if f.fsys != nil {
		var r fs.File
		r, err = f.fsys.Open(f.path)
		if err != nil {
			return err
		}
		defer r.Close()
		sqlBytes, err = io.ReadAll(r)
	} else {
		sqlBytes, err = os.ReadFile(f.path)
	}
	if err != nil {
		return err
	}
	return db.execMigration(ctx, f, isUp, string(sqlBytes))
}

func (db *DB) execMigration(ctx context.Context, f migration, isUp bool, sql string) error {
	// Execute without transaction — CREATE EXTENSION fails inside a tx.
	if _, err := db.Pool.Exec(ctx, sql); err != nil {
		return err
	}
	if isUp {
		db.Pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, f.version)
		db.Pool.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)`, f.version)
	} else {
		db.Pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, f.version)
	}
	return nil
}

type migration struct {
	version int
	name    string
	path    string
	fsys    fs.FS // non-nil when loaded from embedded FS
}

func findMigrations(dir, direction string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	migs := []migration{}
	suffix := "." + direction + ".sql"
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		migs = append(migs, migration{version: v, name: e.Name(), path: filepath.Join(dir, e.Name())})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}

func findMigrationsFS(fsys fs.FS, direction string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migs := []migration{}
	suffix := "." + direction + ".sql"
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		migs = append(migs, migration{version: v, name: e.Name(), path: e.Name(), fsys: fsys})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })
	return migs, nil
}
