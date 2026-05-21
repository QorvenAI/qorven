// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const releaseRepo = "qorvenai/qorven"

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	HTMLURL    string    `json:"html_url"`
	Assets     []ghAsset `json:"assets"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
}

// fetchLatestGHRelease returns the most recent release, including pre-releases
// (alpha/beta). The /releases/latest endpoint skips pre-releases, so we fetch
// the list and take the first non-draft entry instead.
func fetchLatestGHRelease() (*ghRelease, error) {
	repo := os.Getenv("QORVEN_RELEASE_REPO")
	if repo == "" {
		repo = releaseRepo
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=10", repo), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "qorven-gateway")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("repository is private — set GITHUB_TOKEN on the server")
	}
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no releases found in %s", repo)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API: %d", resp.StatusCode)
	}
	var releases []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&releases); err != nil {
		return nil, err
	}
	for i := range releases {
		if !releases[i].Draft {
			return &releases[i], nil
		}
	}
	return nil, fmt.Errorf("no releases found in %s", repo)
}

// releaseVersion returns the bare semver from a tag or build version string.
// Handles:
//   - "v0.1.7-alpha"                    → "0.1.7-alpha"
//   - "qorven-v0.1.7-alpha"             → "0.1.7-alpha"
//   - "v0.1.6-alpha-12-g0cf0386"        → "0.1.6-alpha"
//   - "v0.1.6-alpha-12-g0cf0386-dirty"  → "0.1.6-alpha"
func releaseVersion(tag string) string {
	tag = strings.TrimPrefix(tag, "qorven-v")
	tag = strings.TrimPrefix(tag, "v")
	// git describe format: <tag>-<N>-g<hash>[-dirty]
	// Walk from the right: skip "dirty", then when we find "g<hash>" strip
	// it and the commit-count segment immediately before it.
	parts := strings.Split(tag, "-")
	for i := len(parts) - 1; i >= 2; i-- {
		seg := parts[i]
		if seg == "dirty" {
			continue
		}
		if len(seg) > 1 && seg[0] == 'g' {
			tag = strings.Join(parts[:i-1], "-")
		}
		break
	}
	return tag
}

// handleAdminUpdateCheck — GET /v1/admin/update/check
func (gw *Gateway) handleAdminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}

	release, err := fetchLatestGHRelease()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach GitHub: " + err.Error()})
		return
	}

	current := releaseVersion(buildInfo.Version)
	latest := releaseVersion(release.TagName)
	upToDate := latest == current

	repo := os.Getenv("QORVEN_RELEASE_REPO")
	if repo == "" {
		repo = releaseRepo
	}
	changelogURL := fmt.Sprintf("https://github.com/%s/releases/tag/%s", repo, release.TagName)

	writeJSON(w, http.StatusOK, map[string]any{
		"current":       current,
		"latest":        latest,
		"up_to_date":    upToDate,
		"release_url":   release.HTMLURL,
		"changelog_url": changelogURL,
	})
}

// handleAdminUpdateInstall — POST /v1/admin/update/install
func (gw *Gateway) handleAdminUpdateInstall(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}

	release, err := fetchLatestGHRelease()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not reach GitHub: " + err.Error()})
		return
	}

	current := releaseVersion(buildInfo.Version)
	latest := releaseVersion(release.TagName)
	if latest == current {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "already up to date", "version": current})
		return
	}

	binName := fmt.Sprintf("qorven-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	shaName := binName + ".sha256"

	var binAsset, shaAsset ghAsset
	var foundBin, foundSha bool
	for _, a := range release.Assets {
		switch a.Name {
		case binName:
			binAsset = a
			foundBin = true
		case shaName:
			shaAsset = a
			foundSha = true
		}
	}
	if !foundBin || !foundSha {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": fmt.Sprintf("no binary for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, latest),
		})
		return
	}

	// Download binary
	binPath, err := downloadGHAsset(binAsset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "download failed: " + err.Error()})
		return
	}
	defer os.Remove(binPath)

	// Download checksum
	shaPath, err := downloadGHAsset(shaAsset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "checksum download failed: " + err.Error()})
		return
	}
	defer os.Remove(shaPath)

	// Verify
	if err := verifyGHChecksum(binPath, shaPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "checksum mismatch: " + err.Error()})
		return
	}

	// On Windows the binary is locked while running; stop the service so the
	// file can be overwritten. NSSM restarts it after selfExit() below.
	stopWindowsService()

	// Resolve the canonical install path — do NOT rely on os.Executable() which
	// may return a deleted inode path (e.g. qorven.bak) when the binary was
	// previously replaced by the background updater while the process was live.
	self := canonicalInstallPath()
	if err := replaceBinarySelf(self, binPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "install failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"from":        current,
		"to":          latest,
		"restart":     true,
		"auto_restart": true,
	})

	// Flush the response, then self-restart.
	// On systemd installs (Restart=always) the service manager brings the
	// new binary back up within RestartSec (3 s). On non-systemd installs
	// the process exits and the user or supervisor must restart manually.
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(500 * time.Millisecond) // give the HTTP response time to reach the client
		triggerSelfRestart()
	}()
}

// triggerSelfRestart exits cleanly so systemd's Restart=always policy brings
// the new binary up. systemctl restart is intentionally NOT called here —
// systemd silently ignores self-restart requests from sandboxed processes
// (NoNewPrivileges=yes), returning exit 0 without actually restarting.
// selfExit is defined in update_restart_unix.go / update_restart_windows.go.
func triggerSelfRestart() {
	slog.Info("update.restart", "method", "self_exit")
	selfExit()
}

// patchServiceUnit ensures the installed systemd unit has the settings
// needed for self-update to work:
//   - Restart=always   (so a clean exit triggers restart, not just failures)
//   - ReadWritePaths includes /usr/local/bin  (ProtectSystem=full makes it
//     read-only otherwise, breaking the binary swap)
//
// Safe to call repeatedly — no-ops when already correct or unit doesn't exist.
func patchServiceUnit() {
	const unitPath = "/etc/systemd/system/qorven.service"
	data, err := os.ReadFile(unitPath)
	if err != nil {
		return
	}
	updated := string(data)
	updated = strings.ReplaceAll(updated, "Restart=on-failure", "Restart=always")
	// Ensure /usr/local/bin is in ReadWritePaths so ProtectSystem=full allows
	// the binary swap. Add or extend the directive as needed.
	if !strings.Contains(updated, "/usr/local/bin") {
		if strings.Contains(updated, "ReadWritePaths=") {
			updated = strings.ReplaceAll(updated,
				"ReadWritePaths=",
				"ReadWritePaths=/usr/local/bin ",
			)
		} else if strings.Contains(updated, "ProtectSystem=") {
			updated = strings.ReplaceAll(updated,
				"ProtectSystem=",
				"ReadWritePaths=/usr/local/bin\nProtectSystem=",
			)
		}
	}
	if updated == string(data) {
		return
	}
	if err := os.WriteFile(unitPath, []byte(updated), 0644); err != nil {
		slog.Warn("update.patch_unit_failed", "err", err)
		return
	}
	if path, err := exec.LookPath("systemctl"); err == nil {
		exec.Command(path, "daemon-reload").Run()
	}
	slog.Info("update.unit_patched")
}

// backgroundUpdateChecker runs at startup and every 6 hours. If a newer
// release is available it downloads, verifies, and installs it automatically,
// then triggers a graceful restart via systemd (or self-exit for supervisors).
// On dev builds (Version == "dev" or empty) the check runs once to populate
// the version field in the status bar, but skips the install step.
func (gw *Gateway) backgroundUpdateChecker() {
	check := func() {
		release, err := fetchLatestGHRelease()
		if err != nil {
			slog.Debug("update.check_failed", "err", err)
			return
		}
		current := releaseVersion(buildInfo.Version)
		latest := releaseVersion(release.TagName)

		slog.Info("update.check", "current", current, "latest", latest)

		// On dev builds (dirty, local, or untagged) don't auto-install.
		if current == "dev" || current == "" || strings.Contains(buildInfo.Version, "dirty") || strings.Contains(buildInfo.Version, "-g") {
			slog.Info("update.dev_build_skip_install", "latest", latest)
			return
		}
		if latest == current {
			return // already up to date
		}

		slog.Info("update.installing", "from", current, "to", latest)
		binName := fmt.Sprintf("qorven-%s-%s", runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		shaName := binName + ".sha256"

		var binAsset, shaAsset ghAsset
		for _, a := range release.Assets {
			switch a.Name {
			case binName:
				binAsset = a
			case shaName:
				shaAsset = a
			}
		}
		if binAsset.BrowserDownloadURL == "" || shaAsset.BrowserDownloadURL == "" {
			slog.Warn("update.no_asset", "os", runtime.GOOS, "arch", runtime.GOARCH)
			return
		}

		binPath, err := downloadGHAsset(binAsset)
		if err != nil {
			slog.Warn("update.download_failed", "err", err)
			return
		}
		defer os.Remove(binPath)

		shaPath, err := downloadGHAsset(shaAsset)
		if err != nil {
			slog.Warn("update.sha_download_failed", "err", err)
			return
		}
		defer os.Remove(shaPath)

		if err := verifyGHChecksum(binPath, shaPath); err != nil {
			slog.Warn("update.checksum_failed", "err", err)
			return
		}

		if err := replaceBinarySelf(canonicalInstallPath(), binPath); err != nil {
			slog.Warn("update.replace_failed", "err", err)
			return
		}
		slog.Info("update.installed", "version", latest)
		time.Sleep(500 * time.Millisecond)
		triggerSelfRestart()
	}

	// Initial check on startup (delay slightly so the server is fully up).
	time.Sleep(30 * time.Second)
	check()

	// Periodic check every 6 hours.
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		check()
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func downloadGHAsset(a ghAsset) (string, error) {
	req, _ := http.NewRequest("GET", a.BrowserDownloadURL, nil)
	req.Header.Set("User-Agent", "qorven-gateway")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, a.Name)
	}
	ext := ""
	if strings.HasSuffix(a.Name, ".sha256") {
		ext = ".sha256"
	} else if strings.HasSuffix(a.Name, ".exe") {
		ext = ".exe"
	}
	tmp, err := os.CreateTemp("", "qorven-update-*"+ext)
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

func verifyGHChecksum(binPath, shaPath string) error {
	raw, err := os.ReadFile(shaPath)
	if err != nil {
		return err
	}
	fields := strings.Fields(strings.TrimSpace(string(raw)))
	if len(fields) < 1 {
		return fmt.Errorf("empty checksum file")
	}
	want := strings.ToLower(fields[0])
	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("want %s got %s", want, got)
	}
	return nil
}

// canonicalInstallPath returns the authoritative path of the Qorven binary,
// independent of how the current process was started. os.Executable() can
// return a deleted-inode path (e.g. /opt/qorven/bin/qorven.bak) when the
// background updater renamed the file while the process was live. We always
// prefer the well-known install location if it exists.
func canonicalInstallPath() string {
	for _, p := range []string{
		"/opt/qorven/bin/qorven",
		"/usr/local/bin/qorven",
		"/usr/bin/qorven",
	} {
		if fi, err := os.Lstat(p); err == nil && !fi.Mode().IsDir() {
			return p
		}
	}
	self, _ := os.Executable()
	return self
}

func replaceBinarySelf(current, next string) error {
	// The installer places the binary in /opt/qorven/bin/ and sets
	// chown qorven:qorven on both the directory and the file. The service user
	// therefore has full write access to that directory, so a plain atomic
	// rename is all we need — no sudo, no nsenter, no sandbox escaping.
	//
	// For installs that haven't migrated yet (binary still at /usr/local/bin/),
	// the startup migration in migrate_binary.go moves the binary first.
	staged := current + ".new"
	if err := copyFileTo(next, staged); err != nil {
		return fmt.Errorf("stage failed: %w", err)
	}
	if err := os.Chmod(staged, 0755); err != nil {
		os.Remove(staged)
		return fmt.Errorf("chmod failed: %w", err)
	}
	if err := os.Rename(staged, current); err != nil {
		os.Remove(staged)
		return fmt.Errorf("install failed: %w", err)
	}
	return nil
}

func copyFileTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
