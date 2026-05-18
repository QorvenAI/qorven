// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package cmd — migrate subcommand. Exposes the same store.DB.MigrateUp /
// MigrateDown / MigrateForce that the gateway boots with, as a
// standalone CLI surface. Critical for CI: the workflow calls
// `qorven migrate up` instead of replaying raw SQL, so we exercise the
// exact migrator shipping in production.
package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/qorvenai/qorven/internal/store"
)

var (
	migrateDir     string
	migrateBootExt bool
)

func init() {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations",
		Long: `Apply, roll back, or force-mark database migrations.

The migrator is the same one the gateway invokes at boot. Use this
command from CI and release pipelines to advance or audit the schema
without standing up the full gateway.

Examples:
  qorven migrate up
  qorven migrate down
  qorven migrate force 37

Environment:
  QORVEN_POSTGRES_DSN   Postgres DSN (required)
  QORVEN_MIGRATIONS_DIR Path to migrations/ (default: backend/migrations)
`,
	}

	migrateUp := &cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE:  runMigrateUp,
	}
	migrateDown := &cobra.Command{
		Use:   "down",
		Short: "Roll back the most recent migration",
		RunE:  runMigrateDown,
	}
	migrateForce := &cobra.Command{
		Use:   "force [version]",
		Short: "Force-mark schema at a specific version (recovery only)",
		Args:  cobra.ExactArgs(1),
		RunE:  runMigrateForce,
	}

	for _, c := range []*cobra.Command{migrateUp, migrateDown, migrateForce} {
		c.Flags().StringVar(&migrateDir, "dir", defaultMigrationsDir(),
			"Path to the migrations directory")
		c.Flags().BoolVar(&migrateBootExt, "bootstrap-extensions", true,
			"Create pgcrypto/uuid-ossp + the uuid_generate_v7 function before applying migrations")
	}

	migrateCmd.AddCommand(migrateUp, migrateDown, migrateForce)
	rootCmd.AddCommand(migrateCmd)
}

// defaultMigrationsDir picks the migrations directory with the same
// search order the gateway uses at boot.
func defaultMigrationsDir() string {
	if v := os.Getenv("QORVEN_MIGRATIONS_DIR"); v != "" {
		return v
	}
	candidates := []string{
		"migrations",
		"backend/migrations",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "migrations"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".qorven", "migrations"))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	return "migrations"
}

func openMigrationsDB() (*store.DB, error) {
	dsn := os.Getenv("QORVEN_POSTGRES_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("QORVEN_POSTGRES_DSN is required (current value: empty)")
	}
	db, err := store.New(dsn)
	if err != nil {
		return nil, fmt.Errorf("open DB: %w", err)
	}
	return db, nil
}

// bootstrapExtensions pre-creates the extensions and the
// uuid_generate_v7 function that early migrations depend on. The
// gateway does this at boot; CI/release callers must replicate it
// here because migrations run before the gateway starts.
func bootstrapExtensions(db *store.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, ext := range []string{"pgcrypto", "vector", "uuid-ossp"} {
		// Best effort — missing extensions surface through migration failures downstream.
		_, _ = db.Pool.Exec(ctx, fmt.Sprintf(`CREATE EXTENSION IF NOT EXISTS "%s"`, ext))
	}
	_, err := db.Pool.Exec(ctx, `
CREATE OR REPLACE FUNCTION uuid_generate_v7() RETURNS uuid AS $$
DECLARE unix_ts_ms bytea; uuid_bytes bytea; rand_bytes bytea;
BEGIN
  unix_ts_ms = substring(int8send(floor(extract(epoch from clock_timestamp()) * 1000)::bigint) from 3);
  BEGIN
    rand_bytes = gen_random_bytes(10);
  EXCEPTION WHEN undefined_function THEN
    rand_bytes = substring(sha256((random()::text || clock_timestamp()::text)::bytea) FROM 1 FOR 10);
  END;
  uuid_bytes = unix_ts_ms || rand_bytes;
  uuid_bytes = set_byte(uuid_bytes, 6, (b'0111' || get_byte(uuid_bytes, 6)::bit(4))::bit(8)::int);
  uuid_bytes = set_byte(uuid_bytes, 8, (b'10'   || get_byte(uuid_bytes, 8)::bit(6))::bit(8)::int);
  RETURN encode(uuid_bytes, 'hex')::uuid;
END $$ LANGUAGE plpgsql VOLATILE`)
	return err
}

func runMigrateUp(cmd *cobra.Command, _ []string) error {
	db, err := openMigrationsDB()
	if err != nil {
		return err
	}
	defer db.Pool.Close()
	if migrateBootExt {
		if err := bootstrapExtensions(db); err != nil {
			return fmt.Errorf("bootstrap extensions: %w", err)
		}
	}
	if err := db.MigrateUp(migrateDir); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	cmd.Printf("migrate up: applied from %s\n", migrateDir)
	return nil
}

func runMigrateDown(cmd *cobra.Command, _ []string) error {
	db, err := openMigrationsDB()
	if err != nil {
		return err
	}
	defer db.Pool.Close()
	if err := db.MigrateDown(migrateDir); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	cmd.Printf("migrate down: rolled back one migration\n")
	return nil
}

func runMigrateForce(cmd *cobra.Command, args []string) error {
	v, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("version must be an integer: %w", err)
	}
	db, err := openMigrationsDB()
	if err != nil {
		return err
	}
	defer db.Pool.Close()
	if err := db.MigrateForce(v); err != nil {
		return fmt.Errorf("migrate force %d: %w", v, err)
	}
	cmd.Printf("migrate force: marked schema at version %d\n", v)
	return nil
}
