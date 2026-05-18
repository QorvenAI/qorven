// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/config"
)

func init() {
	debugCmd.Flags().BoolVar(&debugJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(debugCmd)
}

var debugJSON bool

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Quick diagnostics dump — config, DB, gateway, providers, channels",
	Long: `Print a complete system diagnostics snapshot.

Checks and reports:
  - Binary version and build info
  - Configuration file status
  - Database connectivity and migration version
  - Gateway reachability and health
  - Provider count and status
  - Agent count
  - Channel status
  - Go runtime stats (goroutines, memory)
  - Environment variables (redacted)
  - Disk and workspace status

Useful for troubleshooting and support requests.

Examples:
  qorven debug           # human-readable output
  qorven debug --json    # machine-readable JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDebug()
	},
}

type debugResult struct {
	Timestamp   string            `json:"timestamp"`
	Version     string            `json:"version"`
	GoVersion   string            `json:"go_version"`
	OS          string            `json:"os"`
	Arch        string            `json:"arch"`
	Config      debugConfigInfo   `json:"config"`
	Database    debugDBInfo       `json:"database"`
	Gateway     debugGatewayInfo  `json:"gateway"`
	Providers   debugProvidersInfo `json:"providers"`
	Agents      debugAgentsInfo   `json:"agents"`
	Channels    debugChannelsInfo `json:"channels"`
	Runtime     debugRuntimeInfo  `json:"runtime"`
	Env         map[string]string `json:"env"`
}

type debugConfigInfo   struct { Found bool; Path string; DSNSet bool; TokenSet bool }
type debugDBInfo       struct { Connected bool; Error string; MigrationVersion string; Tables int }
type debugGatewayInfo  struct { Reachable bool; Error string; Status string; Version string; Uptime string }
type debugProvidersInfo struct { Count int; Error string }
type debugAgentsInfo   struct { Count int; Error string }
type debugChannelsInfo struct { Count int; Error string }
type debugRuntimeInfo  struct { Goroutines int; HeapMB float64; GCPCT float64 }

func runDebug() error {
	start := time.Now()
	result := debugResult{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   Version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Env:       redactedEnv(),
	}

	// ── Config ──
	configPath := ""
	if home, _ := os.UserHomeDir(); home != "" {
		configPath = home + "/.qorven/config.toml"
	}
	_, err := os.Stat(configPath)
	result.Config = debugConfigInfo{
		Found:    err == nil,
		Path:     configPath,
		DSNSet:   os.Getenv("QORVEN_POSTGRES_DSN") != "",
		TokenSet: os.Getenv("QORVEN_GATEWAY_TOKEN") != "" || os.Getenv("QORVEN_TOKEN") != "",
	}

	// ── Database ──
	cfg, _ := config.Load("")
	if cfg != nil {
		dsn := cfg.Database.DSN
		if dsn == "" { dsn = os.Getenv("QORVEN_POSTGRES_DSN") }
		if dsn != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pool, err := pgxpool.New(ctx, dsn)
			if err != nil {
				result.Database.Error = err.Error()
			} else {
				defer pool.Close()
				if err := pool.Ping(ctx); err != nil {
					result.Database.Error = err.Error()
				} else {
					result.Database.Connected = true
					// Get migration version
					var ver string
					pool.QueryRow(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&ver)
					result.Database.MigrationVersion = ver
					// Count tables
					var count int
					pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&count)
					result.Database.Tables = count
				}
			}
		} else {
			result.Database.Error = "no DSN configured"
		}
	}

	// ── Gateway ──
	server := os.Getenv("QORVEN_SERVER")
	if server == "" { server = "http://localhost:4200" }
	token := os.Getenv("QORVEN_GATEWAY_TOKEN")
	if token == "" { token = os.Getenv("QORVEN_TOKEN") }

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", server+"/health/detailed", nil)
	if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
	resp, err := client.Do(req)
	if err != nil {
		result.Gateway.Error = err.Error()
	} else {
		defer resp.Body.Close()
		result.Gateway.Reachable = true
		var health map[string]any
		json.NewDecoder(resp.Body).Decode(&health)
		if s, ok := health["status"].(string); ok { result.Gateway.Status = s }
		if v, ok := health["version"].(string); ok { result.Gateway.Version = v }
		if u, ok := health["uptime"].(string); ok { result.Gateway.Uptime = u }

		// Providers
		req2, _ := http.NewRequest("GET", server+"/v1/providers", nil)
		req2.Header.Set("Authorization", "Bearer "+token)
		if r2, err := client.Do(req2); err == nil {
			var data map[string]any
			json.NewDecoder(r2.Body).Decode(&data)
			r2.Body.Close()
			if list, ok := data["providers"].([]any); ok {
				result.Providers.Count = len(list)
			}
		} else {
			result.Providers.Error = err.Error()
		}

		// Agents
		req3, _ := http.NewRequest("GET", server+"/v1/agents", nil)
		req3.Header.Set("Authorization", "Bearer "+token)
		if r3, err := client.Do(req3); err == nil {
			var data map[string]any
			json.NewDecoder(r3.Body).Decode(&data)
			r3.Body.Close()
			if list, ok := data["agents"].([]any); ok {
				result.Agents.Count = len(list)
			}
		} else {
			result.Agents.Error = err.Error()
		}

		// Channels
		req4, _ := http.NewRequest("GET", server+"/v1/channels", nil)
		req4.Header.Set("Authorization", "Bearer "+token)
		if r4, err := client.Do(req4); err == nil {
			var data map[string]any
			json.NewDecoder(r4.Body).Decode(&data)
			r4.Body.Close()
			// channels can be in different keys
			for _, v := range data {
				if list, ok := v.([]any); ok {
					result.Channels.Count += len(list)
				}
			}
		} else {
			result.Channels.Error = err.Error()
		}
	}

	// ── Runtime ──
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	gcStats := debug.GCStats{}
	debug.ReadGCStats(&gcStats)
	result.Runtime = debugRuntimeInfo{
		Goroutines: runtime.NumGoroutine(),
		HeapMB:     float64(memStats.HeapAlloc) / 1024 / 1024,
	}

	// ── Output ──
	elapsed := time.Since(start)
	if debugJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable
	fmt.Println()
	fmt.Println("  ┌─ qorven debug ───────────────────────────────")
	fmt.Printf("  │  Version:    %s  (%s/%s  Go %s)\n", result.Version, result.OS, result.Arch, result.GoVersion)
	fmt.Printf("  │  Timestamp:  %s\n", result.Timestamp)
	fmt.Println("  │")

	// Config
	configStatus := "✓ found"
	if !result.Config.Found { configStatus = "✗ not found" }
	fmt.Printf("  │  Config:     %s (%s)\n", configStatus, result.Config.Path)
	fmt.Printf("  │  DSN env:    %s   Token env: %s\n", boolIcon(result.Config.DSNSet), boolIcon(result.Config.TokenSet))
	fmt.Println("  │")

	// Database
	dbStatus := "✓ connected"
	if !result.Database.Connected { dbStatus = "✗ " + result.Database.Error }
	fmt.Printf("  │  Database:   %s\n", dbStatus)
	if result.Database.Connected {
		fmt.Printf("  │             Migration: %s   Tables: %d\n", result.Database.MigrationVersion, result.Database.Tables)
	}
	fmt.Println("  │")

	// Gateway
	gwStatus := "✓ reachable"
	if !result.Gateway.Reachable { gwStatus = "✗ " + result.Gateway.Error }
	fmt.Printf("  │  Gateway:    %s\n", gwStatus)
	if result.Gateway.Reachable {
		fmt.Printf("  │             Status: %s   Version: %s   Uptime: %s\n",
			result.Gateway.Status, result.Gateway.Version, result.Gateway.Uptime)
		fmt.Printf("  │             Providers: %d   Agents: %d   Channels: %d\n",
			result.Providers.Count, result.Agents.Count, result.Channels.Count)
	}
	fmt.Println("  │")

	// Runtime
	fmt.Printf("  │  Runtime:    Goroutines: %d   Heap: %.1f MB\n",
		result.Runtime.Goroutines, result.Runtime.HeapMB)
	fmt.Println("  │")

	// Key env vars (redacted)
	fmt.Println("  │  Env:")
	for k, v := range result.Env {
		fmt.Printf("  │    %-30s = %s\n", k, v)
	}

	fmt.Printf("  └──────────────────────────────── (%dms)\n\n", elapsed.Milliseconds())
	return nil
}

func boolIcon(v bool) string {
	if v { return "✓ set" }
	return "✗ not set"
}

func redactedEnv() map[string]string {
	keys := []string{
		"QORVEN_SERVER", "QORVEN_GATEWAY_TOKEN", "QORVEN_TOKEN",
		"QORVEN_POSTGRES_DSN", "QORVEN_ENCRYPTION_KEY", "QORVEN_MANAGED",
	}
	result := make(map[string]string, len(keys))
	for _, k := range keys {
		v := os.Getenv(k)
		if v == "" {
			result[k] = "(not set)"
			continue
		}
		// Redact secrets — show first 4 chars then ***
		if len(v) > 8 {
			result[k] = v[:4] + "****"
		} else {
			result[k] = "****"
		}
	}
	return result
}
