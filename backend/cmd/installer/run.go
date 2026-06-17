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

	// The Tailscale step is deferred until AFTER every other step runs, so the
	// connection question appears once the install work is done — not up front.
	// Identify it by label so this stays correct regardless of step ordering.
	tsIdx := stepIndexByLabel(steps, "tailscale")

	for i := range steps {
		if i == tsIdx {
			continue // handled after the loop, post-connection-prompt
		}
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

	// Now that the install work is done, ask how the server should be reached.
	// On an upgrade — or with no terminal — chooseConnection returns without
	// asking (fully unattended). Then run the deferred Tailscale step if chosen.
	var conn connChoice
	if cfg.Mode != InstallModeUpgrade {
		conn = chooseConnection(cfg)
	} else {
		conn = connChoice{useTailscale: !cfg.SkipTailscale}
	}
	cfg.SkipTailscale = !conn.useTailscale

	if tsIdx >= 0 && steps[tsIdx].status != stepDone {
		detail, warn, _ := executeStep(tsIdx, cfg)
		steps[tsIdx].detail = detail
		if warn {
			steps[tsIdx].status = stepWarn
		} else {
			steps[tsIdx].status = stepDone
		}
		logStep(tsIdx, len(steps), steps[tsIdx].label, statusWord(steps[tsIdx].status), detail)
	}

	// Resolve the URL the UI will be reached at, honouring the user's choice.
	baseURL := resolveBaseURL(steps, conn, tsIdx)

	if err := writeConfigAndMigrate(cfg, baseURL); err != nil {
		fmt.Printf("\n%s\n", err.Error())
		return false, err
	}

	logDone(cfg, steps, baseURL)
	return true, nil
}

// resolveBaseURL picks the address to advertise in config + the summary, in
// priority order:
//  1. A user-chosen override (a detected IP they picked, or a custom URL).
//  2. A Tailscale result: "connected:<ip>" → use it; "url:<auth-url>" → wait for
//     the user to authorize in a browser, then use the assigned 100.x address.
//     On a successful wait the step's detail is updated to "connected:<ip>" so
//     the summary does not still show a stale "authorize this machine" notice.
//  3. A detected public or LAN IP.
//  4. localhost.
func resolveBaseURL(steps []installStep, conn connChoice, tsIdx int) string {
	if conn.overrideURL != "" {
		return conn.overrideURL
	}

	tsDetail := stepDetailByLabel(steps, "tailscale")
	switch {
	case strings.HasPrefix(tsDetail, "connected:"):
		return strings.TrimPrefix(tsDetail, "connected:")
	case strings.HasPrefix(tsDetail, "url:"):
		authURL := strings.TrimPrefix(tsDetail, "url:")
		if ip := awaitTailscaleAuth(authURL, 3*time.Minute); ip != "" {
			// Mark the step connected so logDone won't repeat the auth notice.
			if tsIdx >= 0 {
				steps[tsIdx].detail = "connected:" + ip
			}
			return ip
		}
		// Not authorized in time — fall through to a detected address so the
		// service still has a usable base_url; the summary repeats the auth URL.
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

// stepIndexByLabel returns the index of the first step whose label contains
// substr (case-insensitive), or -1 when none match (e.g. a platform with no
// Tailscale step).
func stepIndexByLabel(steps []installStep, substr string) int {
	lower := strings.ToLower(substr)
	for i := range steps {
		if strings.Contains(strings.ToLower(steps[i].label), lower) {
			return i
		}
	}
	return -1
}

// statusWord maps a stepStatus to the short word shown in the step log.
func statusWord(s stepStatus) string {
	switch s {
	case stepWarn:
		return "warn"
	case stepFail:
		return "FAIL"
	default:
		return "ok"
	}
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
