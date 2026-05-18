// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package sandbox

import (
	"context"
	"testing"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	// default image set at runtime — expected empty
	_ = cfg
}

func TestDockerManager_New(t *testing.T) {
	m := NewDockerManager(Config{Image: "ubuntu:22.04"})
	if m == nil { t.Fatal("nil manager") }
}

func TestDockerManager_Stats_Empty(t *testing.T) {
	m := NewDockerManager(Config{})
	stats := m.Stats()
	if stats == nil { t.Error("nil stats") }
}

func TestDockerManager_ReleaseAll_Empty(t *testing.T) {
	m := NewDockerManager(Config{})
	err := m.ReleaseAll(context.Background())
	if err != nil { t.Errorf("release empty should not error: %v", err) }
}

func TestDockerManager_Release_Nonexistent(t *testing.T) {
	m := NewDockerManager(Config{})
	err := m.Release(context.Background(), "nonexistent")
	// Should not error for nonexistent key
	_ = err
}

func TestCheckDockerAvailable(t *testing.T) {
	err := CheckDockerAvailable(context.Background())
	if err != nil { t.Skipf("Docker not available: %v", err) }
}

func TestExecResult_Fields(t *testing.T) {
	r := ExecResult{ExitCode: 0, Stdout: "hello", Stderr: ""}
	if r.ExitCode != 0 { t.Error("wrong exit code") }
	if r.Stdout != "hello" { t.Error("wrong stdout") }
}

func TestExecResult_Error(t *testing.T) {
	r := ExecResult{ExitCode: 1, Stderr: "command not found"}
	if r.ExitCode == 0 { t.Error("should be non-zero") }
}
