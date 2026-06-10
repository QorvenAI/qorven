// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

// quickTunnelStartTimeout bounds how long Start waits for cloudflared to print
// the assigned *.trycloudflare.com URL before giving up.
const quickTunnelStartTimeout = 30 * time.Second

// quickTunnelRE matches the assigned URL cloudflared prints on stderr.
var quickTunnelRE = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// extractQuickTunnelURL returns the trycloudflare.com URL found in line, or "".
func extractQuickTunnelURL(line string) string {
	return quickTunnelRE.FindString(line)
}

// CloudflareProvider runs a Cloudflare quick tunnel (no account required) via
// the cloudflared CLI, exposing a local URL at a random *.trycloudflare.com.
type CloudflareProvider struct {
	binPath string

	mu  sync.Mutex
	cmd *exec.Cmd
}

// NewCloudflareProvider returns a provider that runs the cloudflared at binPath.
func NewCloudflareProvider(binPath string) *CloudflareProvider {
	return &CloudflareProvider{binPath: binPath}
}

// Name implements Provider.
func (p *CloudflareProvider) Name() string { return "cloudflare" }

// Start launches `cloudflared tunnel --no-autoupdate --url localURL`, scans
// stderr for the assigned trycloudflare.com URL, and returns it. The process is
// left running and can be terminated with Stop. If no URL appears within
// quickTunnelStartTimeout (or ctx is cancelled), the process is killed and an
// error is returned.
func (p *CloudflareProvider) Start(ctx context.Context, localURL string) (string, error) {
	cmd := exec.CommandContext(ctx, p.binPath,
		"tunnel", "--no-autoupdate", "--url", localURL)
	setProcGroup(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("tunnel: cloudflared stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("tunnel: start cloudflared: %w", err)
	}

	p.mu.Lock()
	p.cmd = cmd
	p.mu.Unlock()

	// Scan stderr for the assigned URL on a goroutine so we can apply a timeout.
	urlCh := make(chan string, 1)
	scanner := bufio.NewScanner(stderr)
	go func() {
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			if !found {
				if u := extractQuickTunnelURL(line); u != "" {
					found = true
					urlCh <- u
				}
			}
			// Keep draining after the URL is found so cloudflared doesn't block
			// on a full stderr pipe.
		}
		if !found {
			// Signal that stderr closed without ever yielding a URL.
			close(urlCh)
		}
	}()

	timer := time.NewTimer(quickTunnelStartTimeout)
	defer timer.Stop()

	select {
	case u, ok := <-urlCh:
		if !ok || u == "" {
			p.killProcess()
			return "", fmt.Errorf("tunnel: cloudflared exited before reporting a URL")
		}
		return u, nil
	case <-timer.C:
		p.killProcess()
		return "", fmt.Errorf("tunnel: cloudflared did not report a URL within %s", quickTunnelStartTimeout)
	case <-ctx.Done():
		p.killProcess()
		return "", ctx.Err()
	}
}

// Stop kills the cloudflared process group and waits for it to exit.
func (p *CloudflareProvider) Stop() error {
	p.mu.Lock()
	cmd := p.cmd
	p.cmd = nil
	p.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}
	killGroup(cmd)
	// Wait reaps the process; ignore the inevitable "killed" error.
	_ = cmd.Wait()
	return nil
}

// killProcess kills the running cloudflared (if any) and reaps it.
func (p *CloudflareProvider) killProcess() {
	p.mu.Lock()
	cmd := p.cmd
	p.cmd = nil
	p.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}
	killGroup(cmd)
	go func() {
		_ = cmd.Wait()
	}()
}
