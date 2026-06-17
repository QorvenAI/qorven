// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package installer provides the non-interactive `qorven install` command.
// It runs a fixed sequence of platform steps, streaming one log line per step,
// so it works identically over ssh, cloud-init, CI, and Docker — none of which
// can host an interactive terminal UI. Platform-specific steps live in
// installer_linux.go, installer_darwin.go, and installer_windows.go.
package installer

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ── Steps ─────────────────────────────────────────────────────────────────────

type stepStatus int

const (
	stepPending stepStatus = iota
	stepRunning
	stepDone
	stepWarn
	stepFail
)

type installStep struct {
	label  string
	status stepStatus
	detail string
}

// ── Config ────────────────────────────────────────────────────────────────────

// InstallMode controls which steps the installer runs.
type InstallMode string

const (
	// InstallModeFresh runs the full step sequence (default when nothing exists).
	InstallModeFresh InstallMode = "fresh"
	// InstallModeUpgrade swaps only the binary + migrates + restarts the service.
	// Skips package installs, OS user, data dirs, and DB setup — all already in place.
	InstallModeUpgrade InstallMode = "upgrade"
	// InstallModeRepair re-runs the full idempotent sequence to fix a broken install.
	InstallModeRepair InstallMode = "repair"
)

type Config struct {
	Version          string
	DataDir          string
	SkipDocker       bool
	SkipPG           bool
	TailscaleAuthKey string // optional pre-auth key for headless Tailscale setup
	SkipTailscale    bool
	Port             int         // chosen port; 0 means use DefaultPort (8486)
	SkipNginx        bool        // true = do not install/configure nginx
	Mode             InstallMode // fresh / upgrade / repair; empty = auto-detect
}

func effectivePort(cfg Config) int {
	if cfg.Port > 0 {
		return cfg.Port
	}
	return 8486
}

// ── Plan ────────────────────────────────────────────────────────────────────

// plan resolves the effective install mode, builds the platform step list with
// upgrade/resume pre-marking applied, and returns the (possibly adjusted) config,
// the steps, the checkpoint state to persist into, and whether this run is
// resuming a prior partial install. This is the non-interactive equivalent of
// the old TUI's New() constructor — same logic, no Bubble Tea model.
func plan(cfg Config) (Config, []installStep, *installState, bool) {
	steps := platformSteps()

	// ── Mode detection ────────────────────────────────────────────────────────
	// Priority: explicit Config.Mode flag > QORVEN_INSTALL_MODE env var >
	// auto-detect (complete install present → upgrade). Repair is a full
	// idempotent sequence (safe re-run).
	effectiveMode := resolveInstallMode(cfg)
	cfg.Mode = effectiveMode

	// For upgrade mode, mark all non-upgrade steps as already-done upfront so the
	// runner only executes the binary-swap + service-restart steps.
	if effectiveMode == InstallModeUpgrade {
		upgradeIndices := platformUpgradeStepIndices()
		upgradeSet := make(map[int]bool, len(upgradeIndices))
		for _, i := range upgradeIndices {
			upgradeSet[i] = true
		}
		for i := range steps {
			if !upgradeSet[i] {
				steps[i].status = stepDone
				steps[i].detail = "skipped (upgrade)"
			}
		}
	}

	// ── Resume from checkpoint ────────────────────────────────────────────────
	// Only resume from a checkpoint that matches THIS installer version and step
	// list. Step indices are positional — a different version that reordered or
	// added/removed steps would map a stale checkpoint's indices onto the wrong
	// steps and silently skip a needed one. On any mismatch we discard the old
	// checkpoint and run the full (idempotent) sequence.
	state := loadInstallState()
	canResume := effectiveMode != InstallModeUpgrade &&
		state != nil &&
		len(state.CompletedSteps) > 0 &&
		state.Version == cfg.Version &&
		state.StepCount == len(steps)
	if canResume {
		for i := range steps {
			if stepCompletedInState(state, i) {
				steps[i].status = stepDone
				steps[i].detail = "resumed"
			}
		}
		// Propagate saved config flags so the resumed run behaves consistently.
		if state.Config.Port > 0 {
			cfg.Port = state.Config.Port
		}
		if state.Config.DataDir != "" {
			cfg.DataDir = state.Config.DataDir
		}
		cfg.SkipPG = state.Config.SkipPG
		cfg.SkipDocker = state.Config.SkipDocker
		cfg.SkipNginx = state.Config.SkipNginx
		if !cfg.SkipTailscale {
			cfg.SkipTailscale = state.Config.SkipTailscale
		}
	} else {
		// Fresh install or upgrade — create a new state record (populated as
		// steps complete).
		state = &installState{
			Version:   cfg.Version,
			StepCount: len(steps),
			Mode:      string(effectiveMode),
		}
		state.Config.Port = cfg.Port
		state.Config.DataDir = cfg.DataDir
		state.Config.SkipPG = cfg.SkipPG
		state.Config.SkipDocker = cfg.SkipDocker
		state.Config.SkipTailscale = cfg.SkipTailscale
		state.Config.SkipNginx = cfg.SkipNginx
	}

	return cfg, steps, state, canResume
}

// stepDetailByLabel returns the detail string of the first step whose label
// contains substr (case-insensitive), or "" when no matching step exists (e.g.
// the Tailscale step is absent on a platform that omits it).
func stepDetailByLabel(steps []installStep, substr string) string {
	lower := strings.ToLower(substr)
	for _, s := range steps {
		if strings.Contains(strings.ToLower(s.label), lower) {
			return s.detail
		}
	}
	return ""
}

// ── Config write + migrate + service start ──────────────────────────────────

// writeConfigAndMigrate writes config.toml + .env, runs the database migrations,
// and (on success) restarts the service. baseURL is the address the UI will be
// reached at (auto-detected, no interactive confirmation). A migration failure
// is fatal: the service is NOT started on a broken/dirty schema because that
// produces confusing runtime errors that are far harder to diagnose than a
// clear install-time failure.
func writeConfigAndMigrate(cfg Config, baseURL string) error {
	etcDir := platformConfigDir()
	os.MkdirAll(etcDir, 0755)

	if baseURL == "" {
		baseURL = "localhost"
	}
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "http://" + baseURL
	}

	port := effectivePort(cfg)
	configPath := filepath.Join(etcDir, "config.toml")

	// If config.toml already exists and is non-trivial, leave it untouched — a
	// user may have customised listen port, base_url, or TLS settings, and
	// overwriting would clobber them. The installer only writes config.toml on a
	// fresh install; on a re-run the .env (secrets + DSN) is all that may change.
	if fi, statErr := os.Stat(configPath); statErr == nil && fi.Size() > 32 {
		// Config already exists — skip writing it; just update secrets below.
	} else {
		cfgContent := fmt.Sprintf(`# Qorven Configuration — generated by qorven install
[server]
listen = "0.0.0.0:%d"
base_url = "%s"

[server.tls]
mode = "disabled"

[database]
# DSN is in .env
`, port, baseURL)
		if err := os.WriteFile(configPath, []byte(cfgContent), 0644); err != nil {
			return fmt.Errorf("write config.toml: %w", err)
		}
	}

	envPath := filepath.Join(etcDir, ".env")

	// Preserve ALL existing secrets and, when --skip-postgres is set, the DSN.
	// Regenerating QORVEN_ENCRYPTION_KEY renders all stored API keys unreadable —
	// it is NEVER regenerated once it exists.
	existingKey := ""
	existingToken := ""
	existingDSN := ""
	if existing, readErr := os.ReadFile(envPath); readErr == nil {
		for _, line := range strings.Split(string(existing), "\n") {
			switch {
			case strings.HasPrefix(line, "QORVEN_ENCRYPTION_KEY="):
				existingKey = strings.TrimPrefix(line, "QORVEN_ENCRYPTION_KEY=")
			case strings.HasPrefix(line, "QORVEN_GATEWAY_TOKEN="):
				existingToken = strings.TrimPrefix(line, "QORVEN_GATEWAY_TOKEN=")
			case strings.HasPrefix(line, "QORVEN_POSTGRES_DSN="):
				existingDSN = strings.TrimPrefix(line, "QORVEN_POSTGRES_DSN=")
			}
		}
	}

	// Generate secrets only when absent (first install).
	if existingKey == "" {
		existingKey = randHex(32)
	}
	if existingToken == "" {
		existingToken = randHex(16)
	}

	// DSN: when --skip-postgres is set the user points at a custom / remote PG.
	// If a DSN is already in .env, honour it unconditionally — never overwrite a
	// user-supplied DSN with a localhost probe. Only probe (and write) when no
	// DSN exists yet.
	var dsn string
	switch {
	case cfg.SkipPG && existingDSN != "":
		dsn = existingDSN
	case existingDSN != "":
		dsn = existingDSN
	default:
		dsn = probeSocketDSN()
	}

	env := strings.Join([]string{
		"# Qorven secrets — keep private",
		"QORVEN_POSTGRES_DSN=" + dsn,
		"QORVEN_GATEWAY_TOKEN=" + existingToken,
		"QORVEN_ENCRYPTION_KEY=" + existingKey,
		"",
	}, "\n")
	if err := os.WriteFile(envPath, []byte(env), 0600); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}

	// Run migrations before starting the service. Retry up to 3 times to let the
	// socket settle. A final failure is fatal (see function doc).
	var migrateErr error
	for i := 0; i < 3; i++ {
		migrateErr = platformMigrate(configPath, dsn)
		if migrateErr == nil {
			break
		}
		if i < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	if migrateErr != nil {
		return fmt.Errorf(
			"database migration failed — the service was NOT started to avoid a broken schema.\n\n"+
				"To diagnose and retry:\n"+
				"  sudo QORVEN_CONFIG=%s QORVEN_POSTGRES_DSN=%s qorven migrate up\n\n"+
				"If the schema is dirty from a previous failed run:\n"+
				"  sudo qorven migrate force <N>  (use the version shown in the error above)\n\n"+
				"Underlying error: %w",
			configPath, redactDSNPassword(dsn), migrateErr)
	}

	platformRestartService(configPath)
	return nil
}

// redactDSNPassword hides any embedded password in a Postgres DSN before it is
// shown to the user (e.g. in an error message). Handles both the URL form
// (postgres://user:pass@host/db) and the keyword form (password=secret).
func redactDSNPassword(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		if _, hasPw := u.User.Password(); hasPw {
			u.User = url.UserPassword(u.User.Username(), "****")
			dsn = u.String()
		}
	}
	reKw := regexp.MustCompile(`(?i)password=('[^']*'|"[^"]*"|[^\s&]+)`)
	return reKw.ReplaceAllString(dsn, "password=****")
}

// ── Health check ──────────────────────────────────────────────────────────────

// waitForHealth polls the local /health endpoint until it returns 200 or the
// timeout elapses. Returns (true, "") when healthy, (false, reason) otherwise.
func waitForHealth(cfg Config, timeout time.Duration) (bool, string) {
	port := effectivePort(cfg)
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return true, ""
			}
		}
		time.Sleep(1 * time.Second)
	}
	return false, fmt.Sprintf("service did not respond within %s", timeout)
}
