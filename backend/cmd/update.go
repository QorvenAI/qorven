// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

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

	"github.com/spf13/cobra"
)

// qorven update — self-update from GitHub Releases.
//
// The release pipeline publishes one raw binary per (os, arch) pair,
// each with a .sha256 sidecar:
//
//   qorven-linux-amd64
//   qorven-linux-amd64.sha256
//   qorven-linux-arm64
//   …
//
// This command:
//  1. Resolves the target version (latest, unless --version is given).
//  2. Downloads the matching binary for runtime.GOOS/GOARCH.
//  3. Downloads the SHA-256 sidecar and verifies the digest.
//  4. Replaces the running binary atomically (rename old → .bak,
//     move new in; rollback on failure).
//
// User data, config.toml, the database, and migrations are never
// touched.

var (
	updateCheck    bool
	updateVersion  string
	// Override with QORVEN_RELEASE_REPO env var for forks / internal builds.
	defaultReleaseRepo = "qorvenai/qorven"
)

func init() {
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Update the Qorven binary to the latest release (or a specific version)",
		Long: `Self-update Qorven from its GitHub Releases page.

Only the binary is replaced — config.toml, the database, user data,
and migrations are untouched. Migrations run automatically when the
new binary starts; don't forget to restart the service.

Examples:
  qorven update                     install latest
  qorven update --check             just check, don't install
  qorven update --version v0.3.0    install a specific version

Requires write access to the current binary. On a systemd install
that binary lives at /usr/local/bin/qorven, so the command typically
needs sudo.

The repo defaulted to for downloads is ` + defaultReleaseRepo + `.
Override with QORVEN_RELEASE_REPO=<owner>/<repo> for forks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate()
		},
	}
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "check for updates without installing")
	updateCmd.Flags().StringVar(&updateVersion, "version", "", "install a specific version tag (default: latest)")
	rootCmd.AddCommand(updateCmd)
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

func runUpdate() error {
	repo := os.Getenv("QORVEN_RELEASE_REPO")
	if repo == "" {
		repo = defaultReleaseRepo
	}
	current := Version

	fmt.Printf("  Current:  %s\n", current)
	fmt.Printf("  Repo:     %s\n", repo)

	var release *releaseInfo
	var err error
	if updateVersion != "" {
		fmt.Printf("  Fetching: %s\n", updateVersion)
		release, err = fetchRelease(repo, updateVersion)
	} else {
		fmt.Print("  Checking for updates... ")
		release, err = fetchLatestRelease(repo)
	}
	if err != nil {
		return fmt.Errorf("release lookup: %w", err)
	}
	latest := release.TagName
	if updateVersion == "" {
		fmt.Println(latest)
	}

	if updateVersion == "" && (latest == current || latest == "v"+current) {
		fmt.Println("  up to date ✓")
		return nil
	}

	binAsset, shaAsset, err := pickAssets(release)
	if err != nil {
		return fmt.Errorf("no binary for %s/%s in release %s: %w",
			runtime.GOOS, runtime.GOARCH, latest, err)
	}

	if updateCheck {
		fmt.Printf("\n  %s is available. Install with: qorven update --version %s\n", latest, latest)
		return nil
	}

	// 1. Download binary + sha256 sidecar
	fmt.Printf("  Downloading %s… ", binAsset.Name)
	binPath, err := downloadAsset(binAsset)
	if err != nil {
		return fmt.Errorf("download binary: %w", err)
	}
	defer os.Remove(binPath)

	shaPath, err := downloadAsset(shaAsset)
	if err != nil {
		return fmt.Errorf("download sha256: %w", err)
	}
	defer os.Remove(shaPath)
	fmt.Println("ok")

	// 2. Verify checksum
	fmt.Print("  Verifying checksum… ")
	if err := verifyChecksum(binPath, shaPath, binAsset.Name); err != nil {
		return fmt.Errorf("checksum mismatch — refusing to install: %w", err)
	}
	fmt.Println("ok")

	// 3. Atomically replace the running binary
	currentBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current binary: %w", err)
	}
	fmt.Printf("  Installing to %s… ", currentBin)
	if err := replaceSelf(currentBin, binPath); err != nil {
		return fmt.Errorf("%w\n  (try again with sudo if this is /usr/local/bin)", err)
	}
	fmt.Println("ok")

	fmt.Printf("\n  ✓ Updated: %s → %s\n", current, latest)
	fmt.Println("  Restart the service to pick up the new binary:")
	fmt.Println("    sudo systemctl restart qorven      # systemd install")
	fmt.Println("    qorven gateway restart             # standalone install")
	return nil
}

// ───── GitHub API helpers ─────

func fetchLatestRelease(repo string) (*releaseInfo, error) {
	// /releases/latest skips pre-releases (alpha/beta/rc). Fall back to
	// listing all releases and taking the first — which is the newest by
	// published date, including pre-releases.
	r, err := githubRelease(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err == nil {
		return r, nil
	}
	return githubReleaseList(fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=1", repo))
}

func fetchRelease(repo, tag string) (*releaseInfo, error) {
	return githubRelease(fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag))
}

func githubRelease(url string) (*releaseInfo, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "qorven-self-update")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("release not found — check the version tag")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var r releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// githubReleaseList fetches a list endpoint and returns the first item.
// Used as fallback when /releases/latest returns 404 (all releases are
// pre-releases and the latest endpoint ignores them).
func githubReleaseList(url string) (*releaseInfo, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "qorven-self-update")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}
	var list []releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no releases found in %s", defaultReleaseRepo)
	}
	return &list[0], nil
}

// pickAssets finds the binary + its sha256 sidecar for this
// OS/ARCH. Naming convention (from the release Makefile):
//   qorven-<goos>-<goarch>[.exe]
//   qorven-<goos>-<goarch>[.exe].sha256
func pickAssets(r *releaseInfo) (binary, sha releaseAsset, err error) {
	binName := fmt.Sprintf("qorven-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	shaName := binName + ".sha256"
	var foundBin, foundSha bool
	for _, a := range r.Assets {
		switch a.Name {
		case binName:
			binary = a
			foundBin = true
		case shaName:
			sha = a
			foundSha = true
		}
	}
	if !foundBin {
		return binary, sha, fmt.Errorf("asset %s not in release", binName)
	}
	if !foundSha {
		return binary, sha, fmt.Errorf("asset %s not in release (refusing to install without a checksum)", shaName)
	}
	return binary, sha, nil
}

// ───── Download + verify + swap ─────

func downloadAsset(a releaseAsset) (string, error) {
	req, _ := http.NewRequest("GET", a.BrowserDownloadURL, nil)
	req.Header.Set("User-Agent", "qorven-self-update")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download %s returned %d", a.Name, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "qorven-update-*"+extForName(a.Name))
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return "", err
	}
	return tmp.Name(), nil
}

// extForName preserves the .sha256 or .exe suffix on the temp file —
// not strictly necessary, but makes debugging easier if something
// leaves a stray file behind.
func extForName(name string) string {
	switch {
	case strings.HasSuffix(name, ".sha256"):
		return ".sha256"
	case strings.HasSuffix(name, ".exe"):
		return ".exe"
	}
	return ""
}

// verifyChecksum reads the downloaded sha256 sidecar and compares its
// hex digest against the hashed binary. The sidecar shape is whatever
// `sha256sum` produces:  "<hex>  <filename>\n"
func verifyChecksum(binPath, shaPath, binName string) error {
	shaBytes, err := os.ReadFile(shaPath)
	if err != nil {
		return fmt.Errorf("read sidecar: %w", err)
	}
	wantHex := strings.Fields(strings.TrimSpace(string(shaBytes)))
	if len(wantHex) < 1 {
		return fmt.Errorf("empty sha256 sidecar")
	}
	want := strings.ToLower(wantHex[0])

	f, err := os.Open(binPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("want %s, got %s for %s", want, got, binName)
	}
	return nil
}

// replaceSelf atomically swaps the running binary with the freshly
// downloaded one. Works cross-platform via os.Rename where possible;
// falls back to copy+rename on filesystems that don't support cross-
// device rename.
func replaceSelf(current, next string) error {
	backup := current + ".bak"
	// Rename current out of the way first — on Linux this is safe
	// even while the binary is running; the old inode lives on
	// until the process exits.
	if err := os.Rename(current, backup); err != nil {
		return fmt.Errorf("backup current: %w", err)
	}
	if err := copyAndChmod(next, current); err != nil {
		// Rollback — put the old binary back.
		os.Rename(backup, current)
		return err
	}
	os.Remove(backup)
	return nil
}

func copyAndChmod(src, dst string) error {
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
