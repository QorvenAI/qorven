// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/config"
)

var doctorFix bool

var doctorCommand = &cobra.Command{
	Use:   "doctor",
	Short: "Check system environment and configuration health",
	Long: `Full health check of Qorven installation.

Checks: version, config, database, providers, agents, memory,
tools, channels, workspace, external tools, gateway.

Examples:
  qorven doctor          # check everything
  qorven doctor --fix    # auto-repair common issues`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor()
	},
}

func init() {
	doctorCommand.Flags().BoolVar(&doctorFix, "fix", false, "Auto-repair common issues")
	rootCmd.AddCommand(doctorCommand)
}

func runDoctor() error {
	issues := 0

	// ── Header ──
	fmt.Println()
	fmt.Println("  qorven doctor")
	fmt.Printf("  Version:    %s", Version)
	if Commit != "none" { fmt.Printf(" (commit: %s)", Commit) }
	if BuildTime != "unknown" { fmt.Printf(" built: %s", BuildTime) }
	fmt.Println()
	fmt.Printf("  OS:         %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Go:         %s\n", runtime.Version())

	// ── Config ──
	fmt.Println()
	cfg, err := config.Load(os.Getenv("QORVEN_CONFIG"))
	if err != nil {
		doctorFail("Config", fmt.Sprintf("%v", err))
		doctorHint("Run: qorven init")
		issues++
		fmt.Println("\n  Doctor check complete (config failed).")
		return nil
	}
	if cfg.ConfigPath != "" {
		doctorOK("Config", cfg.ConfigPath)
	} else {
		doctorWarn("Config", "using defaults (no config file found)")
		doctorHint("Run: qorven init")
	}

	// Check .env
	qHome := resolveQorvenHome()
	envPath := filepath.Join(qHome, ".env")
	if _, err := os.Stat(envPath); err == nil {
		doctorOK("Secrets", envPath)
	} else {
		doctorWarn("Secrets", ".env not found")
		if doctorFix {
			os.MkdirAll(qHome, 0755)
			os.WriteFile(envPath, []byte("# Qorven secrets\n"), 0600)
			doctorFixed("Created " + envPath)
		}
	}

	// ── Database ──
	fmt.Println()
	fmt.Println("  Database:")
	dsn := cfg.Database.DSN
	if dsn == "" {
		doctorFail("  Connection", "NOT CONFIGURED")
		doctorHint("Set QORVEN_POSTGRES_DSN or run: qorven init")
		issues++
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		start := time.Now()
		pool, connErr := pgxpool.New(ctx, dsn)
		if connErr != nil {
			doctorFail("  Connection", fmt.Sprintf("FAILED (%v)", connErr))
			issues++
		} else {
			defer pool.Close()
			if pingErr := pool.Ping(ctx); pingErr != nil {
				doctorFail("  Connection", fmt.Sprintf("PING FAILED (%v)", pingErr))
				issues++
			} else {
				latency := time.Since(start)
				var pgVer string
				pool.QueryRow(ctx, "SELECT version()").Scan(&pgVer)
				if idx := strings.Index(pgVer, ","); idx > 0 { pgVer = pgVer[:idx] }
				doctorOK("  Connection", fmt.Sprintf("%s (%dms)", pgVer, latency.Milliseconds()))

				// Schema
				var version int
				var dirty bool
				err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version, &dirty)
				if err != nil {
					doctorWarn("  Schema", "no migrations table")
					if doctorFix {
						doctorHint("Run: qorven migrate up")
					}
				} else if dirty {
					doctorFail("  Schema", fmt.Sprintf("v%d (DIRTY)", version))
					doctorHint("Run: qorven migrate force %d", version-1)
					issues++
				} else {
					doctorOK("  Schema", fmt.Sprintf("v%d", version))
				}

				// pgvector
				var hasVector bool
				pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')").Scan(&hasVector)
				if hasVector {
					doctorOK("  pgvector", "installed")
				} else {
					doctorWarn("  pgvector", "not installed (vector search disabled)")
				}

				// Tables
				var tableCount int
				pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&tableCount)
				doctorOK("  Tables", fmt.Sprintf("%d", tableCount))

				// ── Providers (from DB) ──
				fmt.Println()
				fmt.Println("  Providers:")
				rows, _ := pool.Query(ctx, "SELECT name, COALESCE(display_name,name), enabled, COALESCE(api_key,'') FROM llm_providers ORDER BY name")
				if rows != nil {
					provCount := 0
					for rows.Next() {
						var name, display string
						var enabled bool
						var key string
						rows.Scan(&name, &display, &enabled, &key)
						provCount++
						status := "enabled"
						if !enabled { status = "disabled" }
						if key != "" {
							masked := key[:4] + "..." + key[max(4, len(key)-4):]
							doctorOK("  "+display, fmt.Sprintf("%s (%s)", masked, status))
						} else {
							doctorWarn("  "+display, fmt.Sprintf("no API key (%s)", status))
						}
					}
					rows.Close()
					if provCount == 0 {
						doctorWarn("  (none)", "no providers in database")
					}
				}

				// Config providers
				for _, p := range cfg.Providers {
					if p.APIKey != "" {
						masked := p.APIKey[:4] + "..." + p.APIKey[max(4, len(p.APIKey)-4):]
						doctorOK("  "+p.Name+" (config)", masked)
					}
				}

				// ── Agents ──
				fmt.Println()
				var agentCount int
				pool.QueryRow(ctx, "SELECT count(*) FROM agents").Scan(&agentCount)
				if agentCount > 0 {
					doctorOK("Agents", fmt.Sprintf("%d", agentCount))
					rows, _ := pool.Query(ctx, "SELECT agent_key, model FROM agents ORDER BY created_at LIMIT 5")
					if rows != nil {
						for rows.Next() {
							var key, model string
							rows.Scan(&key, &model)
							fmt.Printf("    %-16s %s\n", key, model)
						}
						rows.Close()
					}
				} else {
					doctorWarn("Agents", "none — run: qorven agent create")
				}

				// ── Memory ──
				fmt.Println()
				var memCount int
				pool.QueryRow(ctx, "SELECT count(*) FROM memories").Scan(&memCount)
				doctorOK("Memory", fmt.Sprintf("%d memories", memCount))

				// ── Channels ──
				fmt.Println()
				fmt.Println("  Channels:")
				chRows, _ := pool.Query(ctx, "SELECT name, channel_type, enabled FROM channel_instances ORDER BY channel_type")
				if chRows != nil {
					chCount := 0
					for chRows.Next() {
						var name, chType string
						var enabled bool
						chRows.Scan(&name, &chType, &enabled)
						chCount++
						status := "enabled"
						if !enabled { status = "disabled" }
						doctorOK("  "+chType+"/"+name, status)
					}
					chRows.Close()
					if chCount == 0 { fmt.Println("    (none configured)") }
				}
			}
		}
	}

	// ── Workspace ──
	fmt.Println()
	wsPath := filepath.Join(qHome, "workspaces")
	if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
		free := diskFree(wsPath)
		doctorOK("Workspace", fmt.Sprintf("%s (%s free)", wsPath, free))
	} else {
		doctorWarn("Workspace", wsPath+" (not found)")
		if doctorFix {
			os.MkdirAll(wsPath, 0755)
			doctorFixed("Created " + wsPath)
		}
	}

	// ── External Tools ──
	fmt.Println()
	fmt.Println("  External Tools:")
	for _, tool := range []string{"docker", "curl", "git", "python3", "node"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			fmt.Printf("    %-12s not found\n", tool)
		} else {
			doctorOK("  "+tool, path)
		}
	}

	// ── Gateway ──
	fmt.Println()
	gw := "http://" + cfg.Server.Listen
	resp, err := http.Get(gw + "/health")
	if err == nil && resp.StatusCode == 200 {
		resp.Body.Close()
		doctorOK("Gateway", gw+" (running)")
	} else {
		doctorWarn("Gateway", gw+" (not running)")
		doctorHint("Start with: qorven start")
	}

	// ── Summary ──
	fmt.Println()
	if issues == 0 {
		fmt.Println("  ✓ All checks passed")
	} else {
		fmt.Printf("  ✗ %d issue(s) found\n", issues)
		if !doctorFix {
			fmt.Println("  Run: qorven doctor --fix")
		}
	}
	fmt.Println()
	return nil
}

func doctorOK(label, detail string) {
	fmt.Printf("  %-14s ✓ %s\n", label+":", detail)
}

func doctorWarn(label, detail string) {
	fmt.Printf("  %-14s ⚠ %s\n", label+":", detail)
}

func doctorFail(label, detail string) {
	fmt.Printf("  %-14s ✗ %s\n", label+":", detail)
}

func doctorHint(format string, args ...any) {
	fmt.Printf("    → "+format+"\n", args...)
}

func doctorFixed(msg string) {
	fmt.Printf("    ✓ Fixed: %s\n", msg)
}


func max(a, b int) int { if a > b { return a }; return b }


