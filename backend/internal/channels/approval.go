// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import "context"

// ApprovalChannel extends Channel with native platform approval buttons.
// Instead of typing /approve, users click platform-native buttons.
type ApprovalChannel interface {
	Channel
	// SendApprovalRequest sends a message with approve/deny buttons.
	// Returns a request ID that can be used to check the result.
	SendApprovalRequest(ctx context.Context, chatID, message string, options ApprovalOptions) (string, error)
	// GetApprovalResult checks if an approval request has been answered.
	GetApprovalResult(ctx context.Context, requestID string) (*ApprovalResult, error)
}

// ApprovalOptions configures the approval request.
type ApprovalOptions struct {
	Command     string // the command being approved
	Risk        string // "low", "medium", "high", "critical"
	Timeout     int    // seconds before auto-deny (0 = no timeout)
	ApproveText string // custom approve button text (default "✅ Approve")
	DenyText    string // custom deny button text (default "❌ Deny")
}

// ApprovalResult represents the outcome of an approval request.
type ApprovalResult struct {
	RequestID  string
	Approved   bool
	DeniedBy   string // who denied (if denied)
	ApprovedBy string // who approved (if approved)
	Reason     string // optional reason
}

// DefaultApprovalOptions returns sensible defaults.
func DefaultApprovalOptions(command string) ApprovalOptions {
	risk := "medium"
	if isHighRisk(command) { risk = "high" }
	return ApprovalOptions{
		Command: command, Risk: risk, Timeout: 300,
		ApproveText: "✅ Approve", DenyText: "❌ Deny",
	}
}

func isHighRisk(cmd string) bool {
	dangerous := []string{"rm -rf", "DROP TABLE", "DELETE FROM", "shutdown", "reboot",
		"format", "mkfs", "dd if=", "chmod 777", "> /dev/"}
	for _, d := range dangerous {
		if len(cmd) >= len(d) {
			for i := 0; i <= len(cmd)-len(d); i++ {
				if cmd[i:i+len(d)] == d { return true }
			}
		}
	}
	return false
}
