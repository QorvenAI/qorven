// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestAtomicWriteFile_Happy: basic roundtrip — write, then read back.
func TestAtomicWriteFile_Happy(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	data := []byte("[server]\nlisten = \"127.0.0.1:4200\"\n")
	if err := AtomicWriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: got %q, want %q", got, data)
	}
}

// TestAtomicWriteFile_PreservesPermissions: secret files need 0o600;
// a naive implementation would briefly write 0o644 before chmod, or
// inherit the temp file's umask-applied mode.
func TestAtomicWriteFile_PreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission model differs on windows")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "secret")
	if err := AtomicWriteFile(path, []byte("topsecret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("perms = %v, want 0o600", fi.Mode().Perm())
	}
}

// TestAtomicWriteFile_OverwritesExisting: the whole point. A second
// write must fully replace the first's content, not append or leave
// stale bytes.
func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f")
	if err := AtomicWriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("got %q, want 'new' (second write didn't replace)", got)
	}
}

// TestAtomicWriteFile_NoTempLeak: after a successful write, no `.tmp`
// file should remain in the target directory. Verifies the temp file
// is moved in place, not copied.
func TestAtomicWriteFile_NoTempLeak(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f")
	if err := AtomicWriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "f" {
			continue
		}
		t.Errorf("unexpected file left behind: %s", e.Name())
	}
}

// TestAtomicWriteFile_CreatesMissingDir: caller shouldn't have to
// MkdirAll first. AtomicWriteFile handles it so call sites stay flat.
func TestAtomicWriteFile_CreatesMissingDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "deeper", "config.toml")
	if err := AtomicWriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
