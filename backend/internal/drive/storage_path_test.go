// Copyright 2026 Qorven AI. All rights reserved.
package drive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDriveRoot_EnvOverride(t *testing.T) {
	t.Setenv("QORVEN_DRIVE_ROOT", "/data/custom-drive")
	if got := DriveRoot(); got != "/data/custom-drive" {
		t.Fatalf("DriveRoot = %q, want /data/custom-drive", got)
	}
}

func TestDriveRoot_DefaultNotTmp(t *testing.T) {
	t.Setenv("QORVEN_DRIVE_ROOT", "")
	got := DriveRoot()
	if strings.HasPrefix(got, os.TempDir()) || strings.HasPrefix(got, "/tmp") {
		t.Fatalf("default DriveRoot must not be under /tmp (ephemeral); got %q", got)
	}
}

func TestDriveFilePath_UnderRoot(t *testing.T) {
	t.Setenv("QORVEN_DRIVE_ROOT", "/data/d")
	p := DriveFilePath("tenantA", "agentB", "report.pdf")
	want := filepath.Join("/data/d", "tenantA", "agentB", "report.pdf")
	if p != want {
		t.Fatalf("DriveFilePath = %q, want %q", p, want)
	}
	if err := ValidateUnderRoot(p); err != nil {
		t.Fatalf("ValidateUnderRoot(%q) = %v, want nil", p, err)
	}
}

func TestValidateUnderRoot_RejectsTraversal(t *testing.T) {
	t.Setenv("QORVEN_DRIVE_ROOT", "/data/d")
	if err := ValidateUnderRoot("/data/d/../../etc/passwd"); err == nil {
		t.Fatal("ValidateUnderRoot must reject path-traversal escaping the root")
	}
	if err := ValidateUnderRoot("/etc/passwd"); err == nil {
		t.Fatal("ValidateUnderRoot must reject a path outside the root")
	}
}

func TestDriveFilePath_SanitizesName(t *testing.T) {
	t.Setenv("QORVEN_DRIVE_ROOT", "/data/d")
	p := DriveFilePath("t", "a", "../../escape.sh")
	if err := ValidateUnderRoot(p); err != nil {
		t.Fatalf("a sanitized path must stay under root; got %q err %v", p, err)
	}
}
