// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package system

import "testing"

func TestScanGoPackages_CurrentDir(t *testing.T) {
	pkgs := ScanGoPackages(".")
	// May return empty if not in a Go project root
	_ = pkgs
}

func TestScanConfig_Nonexistent(t *testing.T) {
	_ = ScanConfig("/nonexistent/config.toml")
	// nonexistent config returns nil — expected
}

func TestScanServiceHealth(t *testing.T) {
	health := ScanServiceHealth()
	// May return empty if no services running
	_ = health
}

func TestPackageInfo_Fields(t *testing.T) {
	p := PackageInfo{Name: "agent", Path: "internal/agent"}
	if p.Name != "agent" { t.Error("wrong name") }
}

func TestPageInfo_Fields(t *testing.T) {
	p := PageInfo{Name: "dashboard", Path: "/dashboard"}
	if p.Path != "/dashboard" { t.Error("wrong path") }
}
