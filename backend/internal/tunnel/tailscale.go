// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tunnel

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// TailscaleProvider exposes the local public port via Tailscale Funnel.
// Requires Tailscale to be installed and the node logged in. Funnel serves on
// :443 of the node's public DNS name (machine.tailnet.ts.net).
type TailscaleProvider struct {
	port int
	cmd  *exec.Cmd
}

func NewTailscaleProvider(port int) *TailscaleProvider {
	return &TailscaleProvider{port: port}
}

func (p *TailscaleProvider) Name() string { return "tailscale" }

// Start runs `tailscale funnel {port}` (background) and returns the node's
// public funnel URL derived from `tailscale status --json`. localURL is
// ignored beyond its port (Funnel maps the node's :443 to the local port).
func (p *TailscaleProvider) Start(ctx context.Context, localURL string) (string, error) {
	if _, err := exec.LookPath("tailscale"); err != nil {
		return "", fmt.Errorf("tailscale not installed: %w", err)
	}

	// Resolve the node's public DNS name for the returned URL.
	dns, err := tailscaleFunnelDNSName(ctx)
	if err != nil {
		return "", err
	}

	// `tailscale funnel <port>` runs in the foreground holding the funnel open;
	// run it detached and keep the handle so Stop() can tear it down.
	cmd := exec.Command("tailscale", "funnel", fmt.Sprintf("%d", p.port))
	setProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start tailscale funnel: %w", err)
	}
	p.cmd = cmd

	return "https://" + dns, nil
}

func (p *TailscaleProvider) Stop() error {
	if p.cmd != nil && p.cmd.Process != nil {
		killGroup(p.cmd)
		_ = p.cmd.Wait()
		p.cmd = nil
	}
	// Best-effort: reset funnel config for this port.
	_ = exec.Command("tailscale", "funnel", "--bg=false", fmt.Sprintf("%d", p.port), "off").Run()
	return nil
}

// tailscaleFunnelDNSName returns the node's public DNS name (…​.ts.net) from
// `tailscale status --json` → Self.DNSName (trailing dot trimmed).
func tailscaleFunnelDNSName(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("tailscale status: %w", err)
	}
	name := extractTailscaleDNSName(string(out))
	if name == "" {
		return "", fmt.Errorf("could not determine tailscale DNS name (is the node logged in?)")
	}
	return name, nil
}

// extractTailscaleDNSName pulls Self.DNSName out of `tailscale status --json`
// without a full struct — small and unit-testable.
func extractTailscaleDNSName(jsonOut string) string {
	const key = `"DNSName":`
	idx := strings.Index(jsonOut, `"Self":`)
	if idx < 0 {
		return ""
	}
	rest := jsonOut[idx:]
	k := strings.Index(rest, key)
	if k < 0 {
		return ""
	}
	rest = rest[k+len(key):]
	start := strings.Index(rest, `"`)
	if start < 0 {
		return ""
	}
	rest = rest[start+1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return strings.TrimSuffix(rest[:end], ".")
}
