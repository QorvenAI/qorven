// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import (
	"fmt"
	"strings"
	"time"
)

// labelWidth is the column the status marker is aligned to in the step log.
const labelWidth = 40

// Run executes the install non-interactively, streaming one log line per step.
// It returns (true, nil) on success. There is no TTY, no prompt, and no full
// screen UI — the output is plain lines, so it works over ssh, cloud-init, CI,
// and Docker, all of which the old interactive installer could not support.
func Run(cfg Config) (bool, error) {
	cfg, steps, state, resuming := plan(cfg)

	logHeader(cfg, resuming)

	for i := range steps {
		// Steps pre-marked done by upgrade/resume are reported and skipped.
		if steps[i].status == stepDone {
			logStep(i, len(steps), steps[i].label, "skip", steps[i].detail)
			continue
		}

		detail, warn, err := executeStep(i, cfg)
		steps[i].detail = detail

		switch {
		case err != nil:
			steps[i].status = stepFail
			logStep(i, len(steps), steps[i].label, "FAIL", "")
			fmt.Printf("\n%s\n", err.Error())
			return false, fmt.Errorf("install failed at step %q: %w", steps[i].label, err)
		case warn:
			steps[i].status = stepWarn
			logStep(i, len(steps), steps[i].label, "warn", detail)
			// Warn steps are NOT checkpointed — they are re-attempted on a re-run
			// so a previously-skipped optional install can recover.
		default:
			steps[i].status = stepDone
			logStep(i, len(steps), steps[i].label, "ok", detail)
			markStepComplete(state, i)
		}
	}

	// Resolve the URL the UI will be reached at — auto-detected, never prompted.
	baseURL := resolveBaseURL(steps)

	if err := writeConfigAndMigrate(cfg, baseURL); err != nil {
		fmt.Printf("\n%s\n", err.Error())
		return false, err
	}

	logDone(cfg, steps, baseURL)
	return true, nil
}

// resolveBaseURL picks the address to advertise in config + the summary. It
// honours a Tailscale "connected:<ip>" result first, then a detected public or
// LAN IP, then falls back to localhost. A Tailscale "url:<auth-url>" result
// means the node still needs browser authorization — we cannot block on that in
// a non-interactive run, so we fall back to a detected IP and surface the auth
// URL in the final summary instead.
func resolveBaseURL(steps []installStep) string {
	tsDetail := stepDetailByLabel(steps, "tailscale")
	if strings.HasPrefix(tsDetail, "connected:") {
		return strings.TrimPrefix(tsDetail, "connected:")
	}

	ips := detectIPs()
	if ips.publicURL != "" {
		return ips.publicURL
	}
	if len(ips.lanIPs) > 0 {
		return ips.lanIPs[0]
	}
	return "localhost"
}

// ── Plain log output (no terminal styling) ──────────────────────────────────

func logHeader(cfg Config, resuming bool) {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	fmt.Printf("\n==> Qorven installer %s  (mode: %s)\n", version, cfg.Mode)
	if resuming {
		fmt.Println("    Resuming a previous partial install — completed steps are skipped.")
	}
	fmt.Println()
}

// logStep prints one aligned step line, e.g.
//
//	[ 4/12] Install PostgreSQL ............... ok    16.3 — pgvector enabled
func logStep(idx, total int, label, status, detail string) {
	dots := labelWidth - len(label)
	if dots < 1 {
		dots = 1
	}
	line := fmt.Sprintf("  [%2d/%d] %s %s %-4s", idx+1, total, label, strings.Repeat(".", dots), status)
	if detail != "" {
		line += "  " + detail
	}
	fmt.Println(line)
}

func logDone(cfg Config, steps []installStep, baseURL string) {
	url := baseURL
	if !strings.HasPrefix(url, "http") {
		url = "http://" + url
	}

	fmt.Println()
	fmt.Println("==> Install complete.")
	fmt.Println()

	// Health probe — best-effort, so the summary reflects reality.
	if ok, reason := waitForHealth(cfg, 12*time.Second); ok {
		fmt.Printf("  Service:  healthy\n")
	} else {
		fmt.Printf("  Service:  not responding yet (%s)\n", reason)
		fmt.Printf("            check logs, then: sudo qorven migrate up && sudo systemctl restart qorven\n")
	}

	fmt.Printf("  Open:     %s\n", url)

	// If Tailscale produced a browser-auth URL, the node isn't on the tailnet yet.
	if tsDetail := stepDetailByLabel(steps, "tailscale"); strings.HasPrefix(tsDetail, "url:") {
		authURL := strings.TrimPrefix(tsDetail, "url:")
		fmt.Println()
		fmt.Printf("  Tailscale: authorize this machine to finish joining your tailnet:\n    %s\n", authURL)
	}

	fmt.Println()
	fmt.Println("  Manage the service:")
	fmt.Println(platformServiceCommands())
	fmt.Println()
}
