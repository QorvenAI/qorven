// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunAppMigrations applies un-applied *.up.sql files from the app's migrations
// directory. Versions are tracked per (app_slug, tenant_id) in app_schema_migrations.
func RunAppMigrations(ctx context.Context, pool *pgxpool.Pool, slug, tenantID, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no migrations directory — that's fine
		}
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Collect *.up.sql files and sort by numeric prefix.
	type migration struct {
		version int
		path    string
	}
	var migrations []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		parts := strings.SplitN(e.Name(), "_", 2)
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		migrations = append(migrations, migration{version: v, path: filepath.Join(migrationsDir, e.Name())})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	for _, m := range migrations {
		// Check if already applied.
		var count int
		err := pool.QueryRow(ctx,
			`SELECT count(*) FROM app_schema_migrations WHERE app_slug=$1 AND tenant_id=$2 AND version=$3`,
			slug, tenantID, m.version,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration version %d: %w", m.version, err)
		}
		if count > 0 {
			continue // already applied
		}

		sql, err := os.ReadFile(m.path)
		if err != nil {
			return fmt.Errorf("read migration %d: %w", m.version, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}

		if _, err := pool.Exec(ctx,
			`INSERT INTO app_schema_migrations (app_slug, tenant_id, version) VALUES ($1,$2,$3)`,
			slug, tenantID, m.version,
		); err != nil {
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}
		slog.Info("app.migration.applied", "app", slug, "version", m.version)
	}
	return nil
}
