// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	ConfigPath string `toml:"-"`

	Server    ServerConfig     `toml:"server"`
	Database  DatabaseConfig   `toml:"database"`
	Auth      AuthConfig       `toml:"auth"`
	Managed   ManagedConfig    `toml:"managed"`
	Providers []ProviderConfig `toml:"providers"`
	SearxngURL string           `toml:"searxng_url"`
	AgentDefaults AgentDefaults `toml:"agent_defaults"`
	Quota     *QuotaConfig      `toml:"quota"`
	Email     EmailConfig       `toml:"email"`
	Telegram  TelegramConfig    `toml:"telegram"`
	SelfBuild SelfBuildConfig   `toml:"self_build"`
	LLMStats            LLMStatsConfig            `toml:"llm_stats"`
	ArtificialAnalysis  ArtificialAnalysisConfig  `toml:"artificial_analysis"`
}

// ArtificialAnalysisConfig controls the optional Artificial Analysis
// enrichment pass. When APIKey is empty the pipeline is a no-op.
// API keys are available at https://artificialanalysis.ai.
type ArtificialAnalysisConfig struct {
	APIKey       string `toml:"api_key"`
	RefreshHours int    `toml:"refresh_hours"` // default 24
}

// LLMStatsConfig controls the optional LLM Stats enrichment pass.
// When APIKey is empty the whole pipeline is a no-op — callers still
// get the static catalog, just without live benchmark/pricing updates.
type LLMStatsConfig struct {
	APIKey        string `toml:"api_key"`
	RefreshHours  int    `toml:"refresh_hours"` // default 24
}

type SelfBuildConfig struct {
	Enabled      bool   `toml:"enabled"`
	IntervalStr  string `toml:"interval"`
	AgentID      string `toml:"agent_id"`
}

func (c SelfBuildConfig) Interval() time.Duration {
	if c.IntervalStr == "" { return 6 * time.Hour }
	d, err := time.ParseDuration(c.IntervalStr)
	if err != nil { return 6 * time.Hour }
	return d
}

type EmailConfig struct {
	SMTPHost string `toml:"smtp_host"`
	SMTPPort int    `toml:"smtp_port"`
	SMTPUser string `toml:"smtp_user"`
	SMTPPass string `toml:"smtp_pass"`
	From     string `toml:"from"`
	FromName string `toml:"from_name"`
}

type TelegramConfig struct {
	BotToken string `toml:"bot_token"`
}

// DefaultPort is Qorven's well-known port — unregistered in IANA, unused by
// any mainstream software. Single listener serves API + embedded UI together.
const DefaultPort = 8486

type ServerConfig struct {
	BaseURL string `toml:"base_url"`

	// Listen is the single address Qorven binds — API, WebSocket, and the
	// embedded UI all share one port. Defaults to 0.0.0.0:8486.
	// Override with QORVEN_PORT=N or set listen = "addr:port" in config.toml.
	Listen string `toml:"listen"`

	// APIListen / WebListen — kept for backward-compat with existing
	// config.toml files. When a file has these but no listen = "...",
	// Load() derives Listen from APIListen's port so old installs keep
	// working without manual edits.
	APIListen string `toml:"api_listen"`
	WebListen string `toml:"web_listen"`

	// WebDir — override the web UI location. When set, the gateway
	// serves static files from this directory instead of the bundled
	// go:embed copy. Operators use this to customise the UI without
	// rebuilding the binary:
	//   1. Clone the repo, edit web/, run `pnpm build`.
	//   2. Point `web_dir = "/var/lib/qorven/web"` here and copy
	//      web/out/ to that path.
	//   3. Restart the service; changes are picked up immediately.
	// Empty → auto-detect: try well-known dev paths, fall back to
	// the embedded UI.
	WebDir string `toml:"web_dir"`

	// Legacy TLS knobs — kept for backward-compat with existing
	// config.toml files. New installs should use the dedicated [tls]
	// section below, which supersedes these when set.
	TLSCert string `toml:"tls_cert"`
	TLSKey  string `toml:"tls_key"`
	TLSAuto bool   `toml:"tls_auto"`

	SSRFAllowedHosts []string `toml:"ssrf_allowed_hosts"`
	AllowedOrigins   []string `toml:"allowed_origins"`

	TLS TLSConfig `toml:"tls"`
}

// TLSConfig controls certificate sourcing for the web listener. Four
// modes, chosen to match the common self-hosted deployment shapes:
//
//   "auto"          (default) — self-signed ECDSA P-256 cert for
//                   localhost/private IPs; Let's Encrypt via autocert
//                   when Domain is set to a public hostname.
//
//   "reverse-proxy" — TLS terminated by Nginx/Caddy/etc. Qorven
//                     serves plain HTTP on WebListen.
//
//   "custom"        — user supplies CertFile + KeyFile.
//
//   "disabled"      — no TLS. Dev only. Loud warning on boot.
type TLSConfig struct {
	Mode     string `toml:"mode"`
	Domain   string `toml:"domain"`
	CertFile string `toml:"cert_file"`
	KeyFile  string `toml:"key_file"`
	CacheDir string `toml:"cache_dir"` // autocert storage; defaults to ~/.qorven/certs
}

type DatabaseConfig struct {
	DSN string `toml:"dsn"`
}

type AuthConfig struct {
	Token         string `toml:"token"`
	EncryptionKey string `toml:"encryption_key"`
	OwnerEmail    string `toml:"owner_email"`
}

type ManagedConfig struct {
	Enabled    bool   `toml:"enabled"`
	CreditsAPI string `toml:"credits_api"`
	BillingAPI string `toml:"billing_api"`
}

type ProviderConfig struct {
	Name    string `toml:"name"`
	Type    string `toml:"type"`
	APIBase string `toml:"api_base"`
	APIKey  string `toml:"api_key"`
	Model   string `toml:"model"`
	Region  string `toml:"region"`
	Enabled bool   `toml:"enabled"`
	IsDefault bool `toml:"is_default"`
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Listen: fmt.Sprintf("0.0.0.0:%d", DefaultPort),
			TLS:    TLSConfig{Mode: "auto"},
		},
		Database: DatabaseConfig{},
		Auth:     AuthConfig{},
		Managed:  ManagedConfig{},
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	// Find config file — check multiple locations
	if path == "" {
		path = os.Getenv("QORVEN_CONFIG")
	}
	if path == "" {
		home, _ := os.UserHomeDir()
		candidates := []string{
			"config.toml",
			"/etc/qorven/config.toml",
			filepath.Join(home, ".qorven", "config.toml"),
			"qorven.toml",
		}
		for _, name := range candidates {
			if _, err := os.Stat(name); err == nil {
				path = name
				break
			}
		}
	}

	// Load TOML if found
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if err := toml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		abs, _ := filepath.Abs(path)
		cfg.ConfigPath = abs
	}

	// Environment overrides (always win)
	if v := os.Getenv("QORVEN_POSTGRES_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("QORVEN_TOKEN"); v != "" {
		cfg.Auth.Token = v
	}
	if v := os.Getenv("QORVEN_GATEWAY_TOKEN"); v != "" {
		cfg.Auth.Token = v
	}
	if v := os.Getenv("QORVEN_ENCRYPTION_KEY"); v != "" {
		cfg.Auth.EncryptionKey = v
	}
	if v := os.Getenv("QORVEN_HOST"); v != "" {
		host := v
		port := "80"
		if p := os.Getenv("QORVEN_PORT"); p != "" {
			port = p
		}
		cfg.Server.Listen = host + ":" + port
	} else if v := os.Getenv("QORVEN_PORT"); v != "" {
		cfg.Server.Listen = "0.0.0.0:" + v
	}

	// Backward-compat: old configs have api_listen/web_listen but no listen.
	// Derive Listen from APIListen's port so existing installs upgrade cleanly.
	if cfg.Server.Listen == "" && cfg.Server.APIListen != "" {
		_, port, err := net.SplitHostPort(cfg.Server.APIListen)
		if err == nil {
			cfg.Server.Listen = "0.0.0.0:" + port
		}
	}
	// If still empty, use default port.
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = fmt.Sprintf("0.0.0.0:%d", DefaultPort)
	}
	if os.Getenv("QORVEN_MANAGED") == "true" {
		cfg.Managed.Enabled = true
	}

	return cfg, nil
}

// AgentDefaults holds global defaults applied to all agents.
type AgentDefaults struct {
	Model  string            `toml:"model"`
	Params map[string]any    `toml:"params"` // global default params (e.g. cacheRetention)
	Models map[string]ModelOverride `toml:"models"` // per-model overrides
}

type ModelOverride struct {
	Params map[string]any `toml:"params"`
}

// MergeParams merges: defaults.params → defaults.models[model].params → agent.params
func (d *AgentDefaults) MergeParams(model string, agentParams map[string]any) map[string]any {
	merged := make(map[string]any)
	for k, v := range d.Params { merged[k] = v }
	if mo, ok := d.Models[model]; ok {
		for k, v := range mo.Params { merged[k] = v }
	}
	for k, v := range agentParams { merged[k] = v }
	return merged
}

// QuotaConfig controls per-user/group request rate limiting.
type QuotaConfig struct {
	Enabled   bool                    `toml:"enabled"`
	Default   QuotaWindow             `toml:"default"`
	Groups    map[string]QuotaWindow  `toml:"groups"`
	Channels  map[string]QuotaWindow  `toml:"channels"`
	Providers map[string]QuotaWindow  `toml:"providers"`
}

// QuotaWindow defines request limits per time window.
type QuotaWindow struct {
	Hour int `toml:"hour" json:"hour"`
	Day  int `toml:"day" json:"day"`
	Week int `toml:"week" json:"week"`
}

// IsZero returns true if all limits are zero (unlimited).
func (w QuotaWindow) IsZero() bool { return w.Hour == 0 && w.Day == 0 && w.Week == 0 }
