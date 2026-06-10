// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tunnel

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/qorvenai/qorven/internal/config"
)

// cloudflaredReleaseBase is the GitHub "latest" download prefix for cloudflared.
//
// SECURITY NOTE: the binary is fetched over HTTPS from the official Cloudflare
// repo (not attacker-controllable) but there is NO checksum/signature
// verification and "latest" is a moving target. The fetched binary is made
// executable and run as a subprocess. Accepted limitation for the quick-tunnel
// stage; a future hardening step should pin a specific version + verify its
// SHA-256 before exec. Do not widen the source URL to anything user-supplied.
const cloudflaredReleaseBase = "https://github.com/cloudflare/cloudflared/releases/latest/download/"

// cloudflaredAssetName maps a GOOS/GOARCH pair to the cloudflared release asset
// filename, e.g. ("linux", "arm64") → "cloudflared-linux-arm64". Windows assets
// carry a .exe suffix. Unsupported architectures return an error.
func cloudflaredAssetName(goos, goarch string) (string, error) {
	var arch string
	switch goarch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("tunnel: unsupported architecture %q for cloudflared", goarch)
	}

	switch goos {
	case "linux", "darwin":
		return fmt.Sprintf("cloudflared-%s-%s", goos, arch), nil
	case "windows":
		return fmt.Sprintf("cloudflared-%s-%s.exe", goos, arch), nil
	default:
		return "", fmt.Errorf("tunnel: unsupported OS %q for cloudflared", goos)
	}
}

// cloudflaredBinPath returns the on-disk location for a downloaded cloudflared.
func cloudflaredBinPath() string {
	name := "cloudflared"
	if runtime.GOOS == "windows" {
		name = "cloudflared.exe"
	}
	return config.Sub("bin", name)
}

// EnsureCloudflared returns the path to a usable cloudflared binary.
//
// Resolution order:
//  1. a previously-downloaded copy at config.Sub("bin", "cloudflared")
//  2. a system-installed cloudflared on PATH
//  3. download the correct build for runtime.GOOS/GOARCH into the data dir,
//     chmod +x, and return it.
func EnsureCloudflared(ctx context.Context) (string, error) {
	// (1) previously-downloaded copy.
	binPath := cloudflaredBinPath()
	if fi, err := os.Stat(binPath); err == nil && !fi.IsDir() {
		return binPath, nil
	}

	// (2) system-installed cloudflared on PATH.
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}

	// (3) download the correct build.
	asset, err := cloudflaredAssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	url := cloudflaredReleaseBase + asset

	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		return "", fmt.Errorf("tunnel: create bin dir: %w", err)
	}

	slog.Info("tunnel.cloudflared.downloading", "url", url, "dest", binPath)
	if err := downloadFile(ctx, url, binPath); err != nil {
		return "", err
	}
	slog.Info("tunnel.cloudflared.downloaded", "path", binPath)
	return binPath, nil
}

// downloadFile fetches url into a temp file alongside dest, makes it executable,
// then atomically renames it into place.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("tunnel: build request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("tunnel: download cloudflared: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tunnel: download cloudflared: unexpected status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".cloudflared-*")
	if err != nil {
		return fmt.Errorf("tunnel: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Clean up the temp file on any error path.
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("tunnel: write cloudflared: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("tunnel: close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("tunnel: chmod cloudflared: %w", err)
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("tunnel: install cloudflared: %w", err)
	}
	return nil
}
