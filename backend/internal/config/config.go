// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package config

import (
	"fmt"
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

type ServerConfig struct {
	BaseURL string `toml:"base_url"`

	// Listen is the legacy single-listener address. When non-empty it
	// still works as a catch-all (serves both API and any static web
	// handler on the same port). Prefer APIListen and WebListen for new
	// deployments — the defaults below make the API localhost-only,
	// which is the safer posture for self-hosted installs.
	Listen string `toml:"listen"`

	// APIListen — where /v1/*, /auth/*, /ws, /health live. Defaults to
	// 127.0.0.1:4200 so a fresh install doesn't accidentally expose
	// the admin API to the public internet. Docker and reverse-proxy
	// deployments can override this explicitly.
	APIListen string `toml:"api_listen"`

	// WebListen — where the user-facing UI lives. Defaults to
	// 0.0.0.0:4201 so the UI is reachable on the LAN while the API
	// stays internal. Set to 127.0.0.1:4201 when running behind a
	// reverse proxy that handles TLS.
	//
	// Empty WebListen disables the web listener entirely (headless
	// mode) — useful when the Go backend is API-only and the web UI
	// runs from a separate Next.js / static host.
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
		// Defaults target the "developer on a laptop, no sudo" shape:
		//   - API on localhost only (safer posture than 0.0.0.0:80)
		//   - Web UI on 0.0.0.0:4201 (reachable on LAN, no root needed)
		//   - TLS in auto mode (self-signed for localhost)
		//   - Legacy Listen kept empty — config.toml can still override
		//     with the old single-listener knob.
		Server: ServerConfig{
			Listen:    "",
			APIListen: "127.0.0.1:4200",
			WebListen: "0.0.0.0:4201",
			TLS:       TLSConfig{Mode: "auto"},
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

	// Backward-compat normalisation. When the user sets only the
	// legacy Listen field, both API and Web share the same address
	// (the pre-split behaviour). When they set APIListen/WebListen
	// we honour those. When they set nothing, defaults() already put
	// API on localhost and Web on 0.0.0.0:4201.
	if cfg.Server.Listen != "" && cfg.Server.APIListen == "127.0.0.1:4200" && cfg.Server.WebListen == "0.0.0.0:4201" {
		// Legacy config (only `listen = "..."` set) — collapse to a
		// single listener so deployments that pinned the old behaviour
		// keep working until they opt in to the split.
		cfg.Server.APIListen = cfg.Server.Listen
		cfg.Server.WebListen = ""
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
