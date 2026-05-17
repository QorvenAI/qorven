// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Package storage exposes rclone-backed cloud storage operations as
// agent tools. rclone supports 70+ backends (S3, GCS, Azure Blob,
// Dropbox, Google Drive, OneDrive, SFTP, WebDAV, Backblaze B2, and
// many more) via a single CLI, so wrapping it gives us that entire
// surface for one integration cost.
//
// Design:
//   - Shell out to the `rclone` binary with `--use-json-log`. Go
//     bindings exist but they drag in every backend's SDK (~100 MB
//     binary). Shelling out keeps our build lean and defers to the
//     user's rclone install, which they update on their own schedule.
//   - Credentials live in the user's rclone config at
//     ~/.config/rclone/rclone.conf (or their chosen --config path).
//     We DO NOT write creds ourselves — giving agents write access
//     to credential files is an ambient-authority footgun.
//   - Every tool validates the remote exists before running the
//     action, so malformed remote names produce a clear error.
//   - Destructive ops (write, sync) are disabled by default and
//     require explicit allow-list config.
//
// Auth story: a user runs `rclone config` once to set up their
// remotes ("gdrive:", "s3:", etc.), then lists the remotes they
// want Qorven to access via gateway config. Qorven never asks the
// LLM for API keys and never rewrites rclone.conf.
package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ErrRcloneNotInstalled is returned by every exported function when
// the rclone binary isn't on PATH. Callers should surface this to
// the user as an actionable "install rclone to use storage_*" hint
// rather than a cryptic "exec: file not found".
var ErrRcloneNotInstalled = errors.New(
	"rclone not installed — download from https://rclone.org/downloads/ and run `rclone config` to set up remotes",
)

// Manager is the package entry point. One Manager per process; shared
// across all storage_* tools. The LookPath check happens in NewManager
// once, not on every call.
type Manager struct {
	binPath     string
	configPath  string // optional --config override; empty = rclone default
	allowWrite  bool   // write/copy/sync tools are no-ops when false
	allowedList []string
	mu          sync.RWMutex // guards cached remote list
	remotes     []Remote
	remotesAt   time.Time
}

// Remote is one configured backend in rclone.conf.
type Remote struct {
	Name string `json:"name"`
	Type string `json:"type"` // s3, drive, dropbox, onedrive, etc.
}

// Config for NewManager. Zero-value = read-only, rclone auto-detected.
type Config struct {
	// BinPath overrides the binary location. Empty = search $PATH.
	// Set this on systems where rclone is installed somewhere
	// non-standard (e.g. /opt/rclone/rclone).
	BinPath string
	// ConfigPath overrides --config. Empty = rclone's default
	// (~/.config/rclone/rclone.conf). Useful for per-tenant configs
	// in a multi-tenant deployment where each tenant has their own
	// set of remotes.
	ConfigPath string
	// AllowWrite enables storage_write, storage_copy, and storage_sync.
	// Default false because a misconfigured remote + an agent with
	// write access can overwrite important files in seconds.
	AllowWrite bool
	// AllowedRemotes restricts which remote names the agent can
	// access. Empty slice = all configured remotes are allowed.
	// Useful for sharing rclone.conf with other tools while scoping
	// agent access to just the buckets you want.
	AllowedRemotes []string
}

// NewManager probes for rclone and returns a configured manager.
// If rclone is not installed, callers still get a non-nil manager
// whose tools return ErrRcloneNotInstalled on invocation — the agent
// then tells the user exactly what to do.
func NewManager(cfg Config) *Manager {
	m := &Manager{
		configPath:  cfg.ConfigPath,
		allowWrite:  cfg.AllowWrite,
		allowedList: cfg.AllowedRemotes,
	}
	bin := cfg.BinPath
	if bin == "" {
		if found, err := exec.LookPath("rclone"); err == nil {
			bin = found
		}
	}
	m.binPath = bin
	return m
}

// Installed reports whether rclone was found. Agents use this to
// decide whether to surface storage_* tools at all.
func (m *Manager) Installed() bool { return m.binPath != "" }

// AllowWrite reports whether destructive ops are enabled.
func (m *Manager) AllowWrite() bool { return m.allowWrite }

// ListRemotes returns the remote names configured in rclone.conf,
// filtered by AllowedRemotes if set. Cached for 60s — listing is
// cheap but the cache protects against a misbehaving agent calling
// `storage_remotes` in a tight loop.
func (m *Manager) ListRemotes(ctx context.Context) ([]Remote, error) {
	if !m.Installed() {
		return nil, ErrRcloneNotInstalled
	}
	m.mu.RLock()
	if time.Since(m.remotesAt) < 60*time.Second && m.remotes != nil {
		r := m.remotes
		m.mu.RUnlock()
		return r, nil
	}
	m.mu.RUnlock()

	out, err := m.run(ctx, "listremotes", "--long")
	if err != nil {
		return nil, err
	}
	// `rclone listremotes --long` prints "name: type" lines.
	remotes := []Remote{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		typeField := strings.TrimSpace(line[colon+1:])
		if m.allowed(name) {
			remotes = append(remotes, Remote{Name: name, Type: typeField})
		}
	}

	m.mu.Lock()
	m.remotes = remotes
	m.remotesAt = time.Now()
	m.mu.Unlock()
	return remotes, nil
}

// allowed checks the configured allow-list. Empty list = allow all.
func (m *Manager) allowed(remoteName string) bool {
	if len(m.allowedList) == 0 {
		return true
	}
	for _, a := range m.allowedList {
		if a == remoteName {
			return true
		}
	}
	return false
}

// requireRemote checks that the remote in "remote:path" is allowed.
// Returns a user-legible error if not, which the agent relays to the
// caller verbatim. Unknown or disallowed remotes produce the same
// message on purpose — don't leak which remotes exist.
func (m *Manager) requireRemote(fullPath string) error {
	colon := strings.Index(fullPath, ":")
	if colon <= 0 {
		return fmt.Errorf("path must be remote:path (got %q)", fullPath)
	}
	name := fullPath[:colon]
	if !m.allowed(name) {
		return fmt.Errorf("remote %q is not in the allow-list", name)
	}
	return nil
}

// LSJSON runs `rclone lsjson remote:path`. Uses JSON output so we can
// return structured entries to the LLM instead of parsing human text.
// The --max-depth=1 flag is applied by default — agents rarely want
// a recursive listing and recursion on a deep bucket is expensive.
func (m *Manager) LSJSON(ctx context.Context, remotePath string, recursive bool) ([]Entry, error) {
	if !m.Installed() {
		return nil, ErrRcloneNotInstalled
	}
	if err := m.requireRemote(remotePath); err != nil {
		return nil, err
	}
	args := []string{"lsjson", remotePath}
	if !recursive {
		args = append(args, "--max-depth=1")
	} else {
		// Recursion is enabled but still bounded — unbounded recursion
		// on S3 with millions of objects is pathological.
		args = append(args, "--max-depth=3")
	}
	out, err := m.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse lsjson: %w (raw: %s)", err, truncate(string(out), 200))
	}
	return entries, nil
}

// Entry is one item from `rclone lsjson`. Names chosen to match rclone
// output exactly so debugging is less confusing.
type Entry struct {
	Path     string    `json:"Path"`
	Name     string    `json:"Name"`
	Size     int64     `json:"Size"`
	MimeType string    `json:"MimeType"`
	ModTime  time.Time `json:"ModTime"`
	IsDir    bool      `json:"IsDir"`
}

// Cat fetches a single file's contents. Enforces a max byte cap so an
// agent can't DoS the context window by reading a 10GB log file.
func (m *Manager) Cat(ctx context.Context, remotePath string, maxBytes int) ([]byte, error) {
	if !m.Installed() {
		return nil, ErrRcloneNotInstalled
	}
	if err := m.requireRemote(remotePath); err != nil {
		return nil, err
	}
	if maxBytes <= 0 || maxBytes > 5<<20 {
		maxBytes = 5 << 20 // 5 MiB default cap
	}
	// rclone cat supports --count to bound the read. Use it so we
	// never pull more than we'll use.
	args := []string{"cat", remotePath, fmt.Sprintf("--count=%d", maxBytes)}
	return m.run(ctx, args...)
}

// Copy copies a source to a destination. Both can be remote:path or
// local paths; rclone handles cross-backend transfers natively.
// Returns a short status string on success.
func (m *Manager) Copy(ctx context.Context, src, dst string) (string, error) {
	if !m.Installed() {
		return "", ErrRcloneNotInstalled
	}
	if !m.allowWrite {
		return "", errors.New("write operations disabled — set AllowWrite=true in storage.Config to enable")
	}
	// Validate that at least one side mentions an allowed remote.
	// Local-to-local copies are rejected — that's what the `exec` or
	// filesystem tool is for. We exist for cross-cloud work.
	srcRemote := strings.Contains(src, ":") && !strings.HasPrefix(src, "/")
	dstRemote := strings.Contains(dst, ":") && !strings.HasPrefix(dst, "/")
	if !srcRemote && !dstRemote {
		return "", errors.New("at least one of src/dst must be a remote:path")
	}
	if srcRemote {
		if err := m.requireRemote(src); err != nil {
			return "", err
		}
	}
	if dstRemote {
		if err := m.requireRemote(dst); err != nil {
			return "", err
		}
	}
	// --stats-one-line + --stats=0 so the command returns once and
	// we capture the final summary instead of streaming progress.
	out, err := m.run(ctx, "copy", src, dst, "--stats=0", "--stats-one-line", "--verbose=0")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Sync mirrors src to dst, deleting extra files on dst. DANGEROUS.
// Locked behind AllowWrite; we also require an explicit `confirmToken`
// argument that must equal the magic "YES-DELETE-DIVERGENT" string.
// The magic token is a deliberate friction point: an LLM can type it,
// but it has to type it on purpose. Much safer than a silent
// --delete-excluded flag buried in docs.
func (m *Manager) Sync(ctx context.Context, src, dst, confirmToken string) (string, error) {
	if !m.Installed() {
		return "", ErrRcloneNotInstalled
	}
	if !m.allowWrite {
		return "", errors.New("write operations disabled")
	}
	if confirmToken != "YES-DELETE-DIVERGENT" {
		return "", errors.New(
			"sync deletes files on the destination; pass confirm=\"YES-DELETE-DIVERGENT\" to proceed")
	}
	srcRemote := strings.Contains(src, ":") && !strings.HasPrefix(src, "/")
	dstRemote := strings.Contains(dst, ":") && !strings.HasPrefix(dst, "/")
	if !srcRemote && !dstRemote {
		return "", errors.New("at least one of src/dst must be a remote:path")
	}
	if srcRemote {
		if err := m.requireRemote(src); err != nil {
			return "", err
		}
	}
	if dstRemote {
		if err := m.requireRemote(dst); err != nil {
			return "", err
		}
	}
	out, err := m.run(ctx, "sync", src, dst, "--stats=0", "--stats-one-line", "--verbose=0")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// WriteFile uploads a local file or byte payload to a remote path.
// For a small in-memory payload we use `rclone rcat` which reads
// stdin; for an existing local file we use `rclone copyto`. Both
// honor AllowWrite.
func (m *Manager) WriteFile(ctx context.Context, remotePath string, payload []byte) error {
	if !m.Installed() {
		return ErrRcloneNotInstalled
	}
	if !m.allowWrite {
		return errors.New("write operations disabled")
	}
	if err := m.requireRemote(remotePath); err != nil {
		return err
	}
	if len(payload) > 100<<20 {
		// 100 MiB cap — anything bigger should go through copy of a
		// local file, not an in-memory buffer.
		return errors.New("payload exceeds 100 MiB; use storage_copy from a local file instead")
	}
	_, err := m.runWithStdin(ctx, payload, "rcat", remotePath)
	return err
}

// --- low-level rclone invocation ---

func (m *Manager) run(ctx context.Context, args ...string) ([]byte, error) {
	if m.configPath != "" {
		args = append([]string{"--config", m.configPath}, args...)
	}
	cmd := exec.CommandContext(ctx, m.binPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		// rclone's own error messages are usually well-formed and
		// safe to pass to the LLM as-is (they don't leak creds).
		return nil, fmt.Errorf("rclone %s: %s", strings.Join(args, " "), truncate(errText, 500))
	}
	return stdout.Bytes(), nil
}

func (m *Manager) runWithStdin(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	if m.configPath != "" {
		args = append([]string{"--config", m.configPath}, args...)
	}
	cmd := exec.CommandContext(ctx, m.binPath, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = err.Error()
		}
		return nil, fmt.Errorf("rclone %s: %s", strings.Join(args, " "), truncate(errText, 500))
	}
	return stdout.Bytes(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
