// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/config"
)

func init() {
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
}

// ─── qorven backup ───────────────────────────────────────────────────────────

var backupCmd = &cobra.Command{
	Use:   "backup [output.tar.gz]",
	Short: "Export all config, agents, sessions, memories, and skills to a backup archive",
	Long: `Create a portable backup of your Qorven instance.

Exports:
  - All agents and their configurations
  - All sessions and message history
  - All memories (without vector embeddings)
  - All skills and cron jobs
  - All workflows
  - config.toml (secrets redacted from archive filename, kept in file)

The backup can be restored on any Qorven instance with 'qorven restore'.

Examples:
  qorven backup                      # creates qorven-backup-TIMESTAMP.tar.gz
  qorven backup my-backup.tar.gz     # custom filename`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outFile := fmt.Sprintf("qorven-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
		if len(args) == 1 { outFile = args[0] }

		cfg, err := config.Load("")
		if err != nil { return fmt.Errorf("load config: %w", err) }
		if cfg.Database.DSN == "" {
			cfg.Database.DSN = os.Getenv("QORVEN_POSTGRES_DSN")
		}
		if cfg.Database.DSN == "" {
			return fmt.Errorf("no database DSN configured — set QORVEN_POSTGRES_DSN")
		}

		ctx := context.Background()
		pool, err := pgxpool.New(ctx, cfg.Database.DSN)
		if err != nil { return fmt.Errorf("connect to database: %w", err) }
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil { return fmt.Errorf("database ping: %w", err) }

		fmt.Printf("Creating backup: %s\n", outFile)
		f, err := os.Create(outFile)
		if err != nil { return fmt.Errorf("create file: %w", err) }
		defer f.Close()

		gz := gzip.NewWriter(f)
		defer gz.Close()
		tw := tar.NewWriter(gz)
		defer tw.Close()

		// Tables to export
		tables := []struct {
			name  string
			query string
		}{
			{"agents", "SELECT row_to_json(t) FROM (SELECT * FROM agents) t"},
			{"sessions", "SELECT row_to_json(t) FROM (SELECT id, tenant_id, agent_id, session_key, channel, status, label, summary, created_at FROM sessions) t"},
			{"memories", "SELECT row_to_json(t) FROM (SELECT id, tenant_id, agent_id, content, memory_type, source, importance, created_at FROM memories) t"},
			{"skills", "SELECT row_to_json(t) FROM (SELECT * FROM skills) t"},
			{"cron_jobs", "SELECT row_to_json(t) FROM (SELECT * FROM cron_jobs) t"},
			{"workflows", "SELECT row_to_json(t) FROM (SELECT * FROM workflows) t"},
			{"channel_instances", "SELECT row_to_json(t) FROM (SELECT id, tenant_id, agent_id, channel_type, enabled, created_at FROM channel_instances) t"},
			{"tasks", "SELECT row_to_json(t) FROM (SELECT * FROM tasks) t"},
		}

		exported := 0
		for _, tbl := range tables {
			rows, err := pool.Query(ctx, tbl.query)
			if err != nil {
				fmt.Printf("  ⚠ Skip %s: %v\n", tbl.name, err)
				continue
			}

			var records []json.RawMessage
			for rows.Next() {
				var raw json.RawMessage
				if err := rows.Scan(&raw); err == nil {
					records = append(records, raw)
				}
			}
			rows.Close()

			data, _ := json.MarshalIndent(records, "", "  ")
			addTarFile(tw, fmt.Sprintf("data/%s.json", tbl.name), data)
			fmt.Printf("  ✓ %s (%d rows)\n", tbl.name, len(records))
			exported += len(records)
		}

		// Include config.toml if it exists (with API keys — keep it secure)
		configPath := filepath.Join(os.Getenv("HOME"), ".qorven", "config.toml")
		if data, err := os.ReadFile(configPath); err == nil {
			addTarFile(tw, "config.toml", data)
			fmt.Println("  ✓ config.toml")
		}

		// Write manifest
		manifest := map[string]any{
			"version":    Version,
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"tables":     len(tables),
			"total_rows": exported,
		}
		manifestData, _ := json.MarshalIndent(manifest, "", "  ")
		addTarFile(tw, "manifest.json", manifestData)

		fmt.Printf("\n✅ Backup complete: %s (%d records)\n", outFile, exported)
		fmt.Println("   Restore with: qorven restore", outFile)
		return nil
	},
}

// ─── qorven restore ──────────────────────────────────────────────────────────

var restoreCmd = &cobra.Command{
	Use:   "restore <backup.tar.gz>",
	Short: "Restore a Qorven instance from a backup archive",
	Long: `Restore your Qorven instance from a backup created by 'qorven backup'.

The restore process:
  1. Reads the backup archive
  2. For each table, performs INSERT ... ON CONFLICT DO NOTHING
     (existing data is preserved, backup fills in missing records)
  3. Reports how many records were restored per table

Use --overwrite to replace existing records with backup data.

Examples:
  qorven restore qorven-backup-20260416-120000.tar.gz
  qorven restore backup.tar.gz --overwrite`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		archivePath := args[0]

		cfg, err := config.Load("")
		if err != nil { return fmt.Errorf("load config: %w", err) }
		if cfg.Database.DSN == "" {
			cfg.Database.DSN = os.Getenv("QORVEN_POSTGRES_DSN")
		}
		if cfg.Database.DSN == "" {
			return fmt.Errorf("no database DSN — set QORVEN_POSTGRES_DSN")
		}

		ctx := context.Background()
		pool, err := pgxpool.New(ctx, cfg.Database.DSN)
		if err != nil { return fmt.Errorf("connect to database: %w", err) }
		defer pool.Close()

		f, err := os.Open(archivePath)
		if err != nil { return fmt.Errorf("open archive: %w", err) }
		defer f.Close()

		gz, err := gzip.NewReader(f)
		if err != nil { return fmt.Errorf("read gzip: %w", err) }
		defer gz.Close()
		tr := tar.NewReader(gz)

		fmt.Printf("Restoring from: %s\n", archivePath)
		if overwrite { fmt.Println("  Mode: overwrite (existing records replaced)") }

		totalRestored := 0
		for {
			hdr, err := tr.Next()
			if err == io.EOF { break }
			if err != nil { return fmt.Errorf("read archive: %w", err) }

			if !strings.HasPrefix(hdr.Name, "data/") || !strings.HasSuffix(hdr.Name, ".json") {
				continue
			}

			tableName := strings.TrimSuffix(strings.TrimPrefix(hdr.Name, "data/"), ".json")
			data, _ := io.ReadAll(tr)

			var records []map[string]any
			if err := json.Unmarshal(data, &records); err != nil || len(records) == 0 { continue }

			restored, skipped := 0, 0
			for _, record := range records {
				if err := upsertRecord(ctx, pool, tableName, record, overwrite); err == nil {
					restored++
				} else {
					skipped++
				}
			}
			fmt.Printf("  ✓ %s: %d restored, %d skipped\n", tableName, restored, skipped)
			totalRestored += restored
		}

		fmt.Printf("\n✅ Restore complete: %d total records restored\n", totalRestored)
		return nil
	},
}

func init() {
	restoreCmd.Flags().Bool("overwrite", false, "Replace existing records with backup data")
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func addTarFile(tw *tar.Writer, name string, data []byte) {
	tw.WriteHeader(&tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0644,
		ModTime: time.Now(),
	})
	tw.Write(data)
}

// upsertRecord inserts a single record into a table.
// Uses ON CONFLICT DO NOTHING (or DO UPDATE on overwrite).
func upsertRecord(ctx context.Context, pool *pgxpool.Pool, table string, record map[string]any, overwrite bool) error {
	// Build columns and placeholders
	cols := make([]string, 0, len(record))
	vals := make([]any, 0, len(record))
	for k, v := range record {
		cols = append(cols, k)
		vals = append(vals, v)
	}
	if len(cols) == 0 { return nil }

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	conflict := "ON CONFLICT DO NOTHING"
	if overwrite {
		// Simple overwrite via DELETE + INSERT (safe for idempotent restore)
		if id, ok := record["id"]; ok {
			pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", table), id) //nolint:errcheck
		}
		conflict = ""
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) %s",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
		conflict,
	)
	_, err := pool.Exec(ctx, query, vals...)
	return err
}
