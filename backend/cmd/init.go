// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/spf13/cobra"

	"github.com/qorvenai/qorven/cmd/wizard"
	"github.com/qorvenai/qorven/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Setup wizard — configure database, provider, and create first agent",
	Long: `Interactive setup wizard for first-time Qorven configuration.

Steps:
  1. Database connection (PostgreSQL)
  2. Run migrations
  3. Choose LLM provider + API key
  4. Create default agent
  5. Generate auth token
  6. Save config

Examples:
  qorven init                                    # Interactive wizard
  qorven init --non-interactive --db-dsn "postgres://..." --provider deepseek --api-key "sk-..."`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(cmd)
	},
}

var (
	initDBDSN          string
	initProvider       string
	initAPIKey         string
	initAPIBase        string
	initAuthToken      string
	initWebListen      string
	initAPIListen      string
	initNonInteractive bool
)

func init() {
	initCmd.Flags().StringVar(&initDBDSN, "db-dsn", "", "PostgreSQL DSN")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "LLM provider (deepseek/openai/gemini/anthropic)")
	initCmd.Flags().StringVar(&initAPIKey, "api-key", "", "Provider API key")
	initCmd.Flags().StringVar(&initAPIBase, "api-base", "", "Custom API base URL")
	initCmd.Flags().StringVar(&initAuthToken, "auth-token", "", "Gateway auth token (auto-generated if empty)")
	initCmd.Flags().StringVar(&initWebListen, "port", "0.0.0.0:443", "Web UI listen address (host:port or just port)")
	initCmd.Flags().StringVar(&initAPIListen, "api-port", "127.0.0.1:4200", "Internal API listen address (loopback:port or just port)")
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false, "Non-interactive mode (requires --db-dsn and --api-key)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command) error {
	qorvenHome := resolveQorvenHome()
	os.MkdirAll(qorvenHome, 0755)
	os.MkdirAll(filepath.Join(qorvenHome, "logs"), 0755)
	os.MkdirAll(filepath.Join(qorvenHome, "workspaces"), 0755)

	// ── Collect DB + provider — TUI for interactive, flags for non-interactive ──

	dsn := initDBDSN
	if dsn == "" {
		dsn = os.Getenv("QORVEN_POSTGRES_DSN")
	}
	// Pre-flight: try existing config / auto-detect before asking the user.
	if dsn == "" {
		if existing, err := loadExistingConfig(); err == nil && existing != "" {
			dsn = existing
			fmt.Printf("  Using DSN from config: %s\n", maskDSN(dsn))
		}
	}
	if dsn == "" {
		if probed := probeWorkingDSN(); probed != "" {
			dsn = probed
			fmt.Printf("  Auto-detected working DSN: %s\n", maskDSN(dsn))
		}
	}

	provider := initProvider
	apiKey := initAPIKey
	apiBase := initAPIBase

	if !initNonInteractive && (dsn == "" || provider == "") {
		// Launch the TUI to collect what we're still missing.
		result, err := wizard.RunInit()
		if err != nil {
			return fmt.Errorf("init wizard error: %w", err)
		}
		if result.Cancelled {
			return fmt.Errorf("setup cancelled")
		}
		if dsn == "" {
			// Build DSN from TUI result
			r := result
			if r.DBPassword != "" {
				dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
					r.DBUser, r.DBPassword, r.DBHost, r.DBPort, r.DBName, r.DBSSLMode)
			} else {
				dsn = fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=%s",
					r.DBUser, r.DBHost, r.DBPort, r.DBName, r.DBSSLMode)
			}
		}
		if provider == "" {
			provider = result.Provider
			apiKey = result.APIKey
			apiBase = result.APIBase
		}
	}

	// Last-resort DSN if still empty.
	if dsn == "" {
		user := os.Getenv("USER")
		if user == "" {
			user = "qorven"
		}
		dsn = fmt.Sprintf("postgres:///qorven?host=/var/run/postgresql&user=%s&sslmode=disable", user)
		fmt.Printf("  Using socket default: %s\n", maskDSN(dsn))
	}
	if provider == "" {
		provider = "deepseek"
		apiBase = "https://api.deepseek.com/v1"
	}
	if apiBase == "" {
		switch provider {
		case "deepseek":
			apiBase = "https://api.deepseek.com/v1"
		case "openai":
			apiBase = "https://api.openai.com/v1"
		case "gemini":
			apiBase = "https://generativelanguage.googleapis.com/v1beta/openai"
		case "anthropic":
			apiBase = "https://api.anthropic.com/v1"
		}
	}

	fmt.Println()
	fmt.Println("  ⚡ Qorven Setup")
	fmt.Println()

	// ── Test connection ──
	fmt.Print("  Testing connection... ")
	start := time.Now()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("cannot connect: %v\n\n  Is PostgreSQL running? Try: systemctl start postgresql", err)
	}
	if err := db.Ping(); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("cannot ping: %v\n\n  Check your DSN and try again", err)
	}
	latency := time.Since(start)

	var pgVersion string
	db.QueryRow("SELECT version()").Scan(&pgVersion)
	pgShort := pgVersion
	if idx := strings.Index(pgVersion, ","); idx > 0 {
		pgShort = pgVersion[:idx]
	}
	fmt.Printf("OK (%s, %dms)\n", pgShort, latency.Milliseconds())

	// ── Migrations ──
	fmt.Print("  Running migrations... ")
	migrationsDir := findMigrationsDir()
	if migrationsDir != "" {
		applied := 0
		entries, _ := os.ReadDir(migrationsDir)
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".up.sql") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(migrationsDir, e.Name()))
			if err != nil {
				continue
			}
			if _, err := db.Exec(string(data)); err == nil {
				applied++
			}
		}
		fmt.Printf("OK (%d files applied)\n", applied)
	} else {
		fmt.Println("SKIPPED (no migrations directory found)")
	}

	// ── Extensions ──
	fmt.Print("  Extensions... ")
	db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm")

	var hasVector bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')").Scan(&hasVector)
	if hasVector {
		fmt.Println("pgvector ✓, pg_trgm ✓")
	} else {
		fmt.Println("pg_trgm ✓ (pgvector not available — install for vector search)")
	}
	db.Close()

	if apiKey != "" && len(apiKey) >= 8 {
		fmt.Printf("  Provider: %s (%s...%s)\n", provider, apiKey[:4], apiKey[len(apiKey)-4:])
	} else {
		fmt.Printf("  Provider: %s (no key — add later with: qorven config set)\n", provider)
	}

	// ── Step 6: Auth token ──
	authToken := initAuthToken
	if authToken == "" || authToken == "auto" {
		authToken = generateToken(16)
	}
	encryptionKey := generateToken(32)

	// ── Step 7: Save config ──
	fmt.Println()
	fmt.Println("  ── Step 3: Saving ──")

	cfgPath := filepath.Join(qorvenHome, "config.toml")
	envPath := filepath.Join(qorvenHome, ".env")

	// Normalise port flags — accept bare "8443" or full "0.0.0.0:8443"
	webListen := normaliseListenAddr(initWebListen, "0.0.0.0")
	apiListen := normaliseListenAddr(initAPIListen, "127.0.0.1")

	// config.toml — no secrets
	cfg := config.Config{
		Server: config.ServerConfig{
			APIListen: apiListen,
			WebListen: webListen,
		},
		Database: config.DatabaseConfig{}, // DSN goes in .env
		Providers: []config.ProviderConfig{{
			Name: provider, Type: providerType(provider), APIBase: apiBase,
		}},
	}
	cfgData := buildConfigTOML(cfg, provider, apiBase)
	if err := config.AtomicWriteFile(cfgPath, []byte(cfgData), 0644); err != nil {
		fmt.Printf("  WARN: could not write config atomically: %v (falling back to direct write)\n", err)
		os.WriteFile(cfgPath, []byte(cfgData), 0644)
	}
	fmt.Printf("  Config:  %s\n", cfgPath)

	// .env — secrets only
	writeEnvFile(envPath, dsn, authToken, encryptionKey)
	if apiKey != "" {
		f, _ := os.OpenFile(envPath, os.O_APPEND|os.O_WRONLY, 0600)
		envKey := strings.ToUpper(provider) + "_API_KEY"
		if provider == "deepseek" { envKey = "DEEPSEEK_API_KEY" }
		fmt.Fprintf(f, "export %s=%s\n", envKey, apiKey)
		f.Close()
	}
	fmt.Printf("  Secrets: %s\n", envPath)

	// ── Step 8: Create default agent ──
	fmt.Print("  Creating default agent... ")
	model := defaultModel(provider)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db2, _ := sql.Open("pgx", dsn)
	if db2 != nil {
		_, err := db2.ExecContext(ctx,
			`INSERT INTO agents (id, tenant_id, agent_key, display_name, model, system_prompt)
			 VALUES (gen_random_uuid(), '00000000-0000-0000-0000-000000000001', 'prime', 'Qorven Prime', $1, 'You are Qorven Prime, a helpful AI assistant.')
			 ON CONFLICT DO NOTHING`, model)
		if err != nil {
			fmt.Printf("skipped (%v)\n", err)
		} else {
			fmt.Printf("OK (agent: prime, model: %s)\n", model)
		}
		db2.Close()
	}

	// ── Summary ──
	fmt.Println()
	fmt.Println("  ┌─────────────────────────────────────────────┐")
	fmt.Println("  │           ✓ Setup Complete!                  │")
	fmt.Println("  └─────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("  Auth token:", authToken)
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("    source", envPath)
	fmt.Println("    qorven start          # start the gateway")
	fmt.Println("    qorven doctor         # verify everything works")
	fmt.Println("    qorven agent chat     # start chatting")
	fmt.Println()

	return nil
}

func resolveQorvenHome() string {
	if h := os.Getenv("QORVEN_HOME"); h != "" { return h }
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".qorven")
}

func findMigrationsDir() string {
	candidates := []string{
		"migrations",
		filepath.Join(resolveQorvenHome(), "migrations"),
		"/usr/local/share/qorven/migrations",
	}
	for _, d := range candidates {
		if info, err := os.Stat(d); err == nil && info.IsDir() { return d }
	}
	return ""
}

// loadExistingConfig reads the DSN from /etc/qorven/config.toml (system
// install) or ~/.qorven/config.toml (user install). Returns "" if not found
// or if the config has no DSN set.
func loadExistingConfig() (string, error) {
	candidates := []string{
		"/etc/qorven/config.toml",
		filepath.Join(resolveQorvenHome(), "config.toml"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "dsn") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					dsn := strings.TrimSpace(parts[1])
					dsn = strings.Trim(dsn, `"'`)
					if dsn != "" {
						return dsn, nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no DSN in existing config")
}

// probeWorkingDSN tries the most common DSN patterns and returns the
// first one that successfully connects. This prevents the "wrong default"
// problem where init asks for a password but the system uses socket auth.
func probeWorkingDSN() string {
	user := os.Getenv("USER")
	if user == "" {
		user = "qorven"
	}
	candidates := []string{
		// Socket auth — most common after install.sh, no password needed
		fmt.Sprintf("postgres:///qorven?host=/var/run/postgresql&user=%s&sslmode=disable", user),
		"postgres:///qorven?host=/var/run/postgresql&user=qorven&sslmode=disable",
		// TCP no password (some configs allow local TCP without auth)
		fmt.Sprintf("postgres://%s@127.0.0.1:5432/qorven?sslmode=disable", user),
		"postgres://qorven@127.0.0.1:5432/qorven?sslmode=disable",
	}
	for _, dsn := range candidates {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = db.PingContext(ctx)
		cancel()
		db.Close()
		if err == nil {
			return dsn
		}
	}
	return ""
}

func maskDSN(dsn string) string {
	if idx := strings.Index(dsn, "@"); idx > 0 {
		prefix := dsn[:strings.Index(dsn, "://")+3]
		return prefix + "***@" + dsn[idx+1:]
	}
	return dsn
}

func providerType(name string) string {
	switch name {
	case "openai", "deepseek": return "openai"
	case "anthropic": return "anthropic"
	case "gemini": return "gemini"
	default: return "openai"
	}
}

func defaultModel(provider string) string {
	switch provider {
	case "deepseek": return "deepseek-chat"
	case "openai": return "gpt-4o-mini"
	case "gemini": return "gemini-2.0-flash"
	case "anthropic": return "claude-sonnet-4-20250514"
	default: return "gpt-4o-mini"
	}
}

func buildConfigTOML(cfg config.Config, provider, apiBase string) string {
	var b strings.Builder
	b.WriteString("# Qorven Configuration — generated by qorven init\n\n")
	b.WriteString("[server]\n")
	b.WriteString(fmt.Sprintf("api_listen = \"%s\"\n", cfg.Server.APIListen))
	b.WriteString(fmt.Sprintf("web_listen = \"%s\"\n\n", cfg.Server.WebListen))
	b.WriteString("# Database DSN is in .env (secrets stay separate from config)\n")
	b.WriteString("[database]\n\n")
	b.WriteString("[[providers]]\n")
	b.WriteString(fmt.Sprintf("name = \"%s\"\n", provider))
	b.WriteString(fmt.Sprintf("type = \"%s\"\n", providerType(provider)))
	if apiBase != "" {
		b.WriteString(fmt.Sprintf("api_base = \"%s\"\n", apiBase))
	}
	b.WriteString("# api_key is in .env\n")
	return b.String()
}

// normaliseListenAddr accepts either a bare port ("8443") or a full
// addr ("0.0.0.0:8443"). Bare ports get the given defaultHost prepended.
func normaliseListenAddr(s, defaultHost string) string {
	if s == "" {
		return ""
	}
	if _, _, err := strings.Cut(s, ":"); !err {
		// No colon — treat as bare port number
		return defaultHost + ":" + s
	}
	return s
}
