// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"strings"
	"testing"
)

// hard_approval_test.go — Tests for approval button system.

func TestHard_DefaultApprovalOptions_HighRisk(t *testing.T) {
	opts := DefaultApprovalOptions("rm -rf /var/data")
	if opts.Risk != "high" { t.Errorf("rm -rf should be high risk, got %q", opts.Risk) }
	if opts.Command != "rm -rf /var/data" { t.Error("command not preserved") }
	if opts.Timeout != 300 { t.Errorf("timeout: %d", opts.Timeout) }
}

func TestHard_DefaultApprovalOptions_MediumRisk(t *testing.T) {
	opts := DefaultApprovalOptions("go build ./...")
	if opts.Risk != "medium" { t.Errorf("go build should be medium risk, got %q", opts.Risk) }
}

func TestHard_IsHighRisk_DangerousCommands(t *testing.T) {
	dangerous := []string{
		"rm -rf /",
		"DROP TABLE users",
		"DELETE FROM agents WHERE 1=1",
		"shutdown -h now",
		"dd if=/dev/zero of=/dev/sda",
		"chmod 777 /etc/passwd",
	}
	for _, cmd := range dangerous {
		if !isHighRisk(cmd) { t.Errorf("should be high risk: %q", cmd) }
	}
}

func TestHard_IsHighRisk_SafeCommands(t *testing.T) {
	safe := []string{
		"go build ./...",
		"git status",
		"ls -la",
		"cat file.txt",
		"echo hello",
	}
	for _, cmd := range safe {
		if isHighRisk(cmd) { t.Errorf("should NOT be high risk: %q", cmd) }
	}
}

func TestHard_ApprovalOptions_ButtonText(t *testing.T) {
	opts := DefaultApprovalOptions("test")
	if !strings.Contains(opts.ApproveText, "Approve") { t.Error("approve text missing") }
	if !strings.Contains(opts.DenyText, "Deny") { t.Error("deny text missing") }
}

func TestHard_ApprovalResult_Structure(t *testing.T) {
	result := &ApprovalResult{
		RequestID: "req_123", Approved: true, ApprovedBy: "admin",
	}
	if !result.Approved { t.Error("should be approved") }
	if result.ApprovedBy != "admin" { t.Error("approver wrong") }
}
