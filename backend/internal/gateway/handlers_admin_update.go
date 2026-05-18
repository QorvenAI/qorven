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
	"net/http"
	"os"
	"runtime"
	"strings"
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

// releaseVersion strips the qorven-v or v prefix to get a bare semver string.
func releaseVersion(tag string) string {
	tag = strings.TrimPrefix(tag, "qorven-v")
	tag = strings.TrimPrefix(tag, "v")
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

	current := buildInfo.Version
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

	current := buildInfo.Version
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

	// Replace binary
	self, err := os.Executable()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cannot locate running binary: " + err.Error()})
		return
	}
	if err := replaceBinarySelf(self, binPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "install failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"from":    current,
		"to":      latest,
		"restart": true,
	})
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

func replaceBinarySelf(current, next string) error {
	backup := current + ".bak"
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	if err := func() error {
		in, err := os.Open(next)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(current, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}(); err != nil {
		os.Rename(backup, current) // rollback
		return err
	}
	os.Remove(backup)
	return nil
}
