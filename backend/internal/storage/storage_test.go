// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestManager_NotInstalled: when rclone isn't on PATH, every op must
// return ErrRcloneNotInstalled so the agent can surface an actionable
// install message. The test fakes "not installed" by forcing an
// invalid binary path.
func TestManager_NotInstalled(t *testing.T) {
	m := &Manager{binPath: ""}

	if m.Installed() {
		t.Fatal("expected Installed() false when binPath empty")
	}

	if _, err := m.ListRemotes(context.Background()); !errors.Is(err, ErrRcloneNotInstalled) {
		t.Errorf("ListRemotes should return ErrRcloneNotInstalled, got %v", err)
	}
	if _, err := m.LSJSON(context.Background(), "r:/", false); !errors.Is(err, ErrRcloneNotInstalled) {
		t.Errorf("LSJSON should return ErrRcloneNotInstalled, got %v", err)
	}
	if _, err := m.Cat(context.Background(), "r:/x", 0); !errors.Is(err, ErrRcloneNotInstalled) {
		t.Errorf("Cat should return ErrRcloneNotInstalled, got %v", err)
	}
	if _, err := m.Copy(context.Background(), "r:/a", "/tmp/b"); !errors.Is(err, ErrRcloneNotInstalled) {
		t.Errorf("Copy should return ErrRcloneNotInstalled, got %v", err)
	}
	if _, err := m.Sync(context.Background(), "r:/a", "r:/b", "YES-DELETE-DIVERGENT"); !errors.Is(err, ErrRcloneNotInstalled) {
		t.Errorf("Sync should return ErrRcloneNotInstalled, got %v", err)
	}
	if err := m.WriteFile(context.Background(), "r:/a", []byte("x")); !errors.Is(err, ErrRcloneNotInstalled) {
		t.Errorf("WriteFile should return ErrRcloneNotInstalled, got %v", err)
	}
}

// TestManager_AllowList: the allow-list gates which remotes the agent
// can touch, regardless of what's configured in rclone.conf.
func TestManager_AllowList(t *testing.T) {
	// We don't actually need rclone installed — the requireRemote
	// check happens before the exec call, so an empty binPath is fine.
	m := &Manager{
		binPath:     "/usr/bin/rclone", // fake path so Installed()=true
		allowedList: []string{"s3", "gdrive"},
	}

	cases := []struct {
		path string
		want bool // true = should be allowed
	}{
		{"s3:bucket/key", true},
		{"gdrive:Documents", true},
		{"dropbox:foo", false},
		{"onedrive:bar", false},
		{"/local/path", false}, // no colon = rejected even if a local file
		{"noColon", false},
	}
	for _, c := range cases {
		err := m.requireRemote(c.path)
		got := err == nil
		if got != c.want {
			t.Errorf("requireRemote(%q) allowed=%v, want %v (err=%v)", c.path, got, c.want, err)
		}
	}
}

// TestManager_AllowList_Empty: empty allow-list = allow everything.
// This is the single-tenant / trusted-operator default.
func TestManager_AllowList_Empty(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone"}

	for _, p := range []string{"s3:x", "gdrive:y", "anything:z"} {
		if err := m.requireRemote(p); err != nil {
			t.Errorf("empty allow-list should permit %q, got err %v", p, err)
		}
	}
}

// TestManager_RequireRemote_MalformedPath: path without a colon or
// with a colon at position 0 is malformed and must be rejected with
// a user-legible error.
func TestManager_RequireRemote_MalformedPath(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone"}

	cases := []string{
		"",
		":nothing",
		"nothing",
		"/local/path",
	}
	for _, p := range cases {
		err := m.requireRemote(p)
		if err == nil {
			t.Errorf("expected error for malformed path %q", p)
		}
	}
}

// TestManager_Sync_ConfirmToken: sync without the magic token must
// refuse to run even with AllowWrite=true and an installed rclone.
// This is the single most important safety test in this package —
// regressions here could silently delete a user's entire bucket.
func TestManager_Sync_ConfirmToken(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone", allowWrite: true}

	_, err := m.Sync(context.Background(), "s3:a", "s3:b", "yes")
	if err == nil {
		t.Fatal("sync without magic token should fail")
	}
	if !strings.Contains(err.Error(), "YES-DELETE-DIVERGENT") {
		t.Errorf("error should cite the magic token; got %q", err)
	}

	_, err = m.Sync(context.Background(), "s3:a", "s3:b", "")
	if err == nil {
		t.Fatal("sync with empty token should fail")
	}
}

// TestManager_Sync_RequiresAllowWrite: even with the correct confirm
// token, sync must refuse when AllowWrite is false.
func TestManager_Sync_RequiresAllowWrite(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone", allowWrite: false}

	_, err := m.Sync(context.Background(), "s3:a", "s3:b", "YES-DELETE-DIVERGENT")
	if err == nil {
		t.Fatal("sync must refuse when AllowWrite=false")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error should mention writes are disabled; got %q", err)
	}
}

// TestManager_Copy_RequiresRemoteSide: a local-to-local copy has no
// reason to go through rclone — reject it.
func TestManager_Copy_RequiresRemoteSide(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone", allowWrite: true}

	_, err := m.Copy(context.Background(), "/tmp/src", "/tmp/dst")
	if err == nil {
		t.Fatal("local-to-local copy should be rejected")
	}
	if !strings.Contains(err.Error(), "remote:path") {
		t.Errorf("error should mention remote:path; got %q", err)
	}
}

// TestManager_WriteFile_SizeCap: the 100 MiB in-memory cap must be
// enforced — an agent handing us a 500 MB payload shouldn't silently
// blow up the gateway's heap.
func TestManager_WriteFile_SizeCap(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone", allowWrite: true}

	payload := make([]byte, 101<<20) // 101 MiB
	err := m.WriteFile(context.Background(), "s3:too-big", payload)
	if err == nil {
		t.Fatal("WriteFile should reject oversize payloads")
	}
	if !strings.Contains(err.Error(), "100 MiB") {
		t.Errorf("error should cite the 100 MiB cap; got %q", err)
	}
}

// TestManager_WriteFile_RequiresAllowWrite: write is disabled by default.
func TestManager_WriteFile_RequiresAllowWrite(t *testing.T) {
	m := &Manager{binPath: "/usr/bin/rclone", allowWrite: false}

	err := m.WriteFile(context.Background(), "s3:x", []byte("hello"))
	if err == nil {
		t.Fatal("WriteFile must refuse when AllowWrite=false")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error should mention disabled; got %q", err)
	}
}

// TestTruncate: quick sanity check for the helper used in error
// formatting — rclone stderr can be verbose, we don't want 10 KB
// errors in the LLM's context.
func TestTruncate(t *testing.T) {
	if got := truncate("short", 100); got != "short" {
		t.Errorf("short input modified: %q", got)
	}
	// "…" is 3 UTF-8 bytes, so truncate(200, 50) = 50 + 3 = 53 bytes.
	if got := truncate(strings.Repeat("x", 200), 50); len(got) != 53 {
		t.Errorf("expected 53 bytes (50 + 3-byte ellipsis), got %d: %q", len(got), got)
	}
	// Verify the ellipsis suffix is actually present — a regression
	// in the truncate func that silently dropped it would otherwise
	// just produce a shorter string.
	if got := truncate(strings.Repeat("x", 200), 50); !strings.HasSuffix(got, "…") {
		t.Errorf("truncated output missing ellipsis: %q", got)
	}
}
