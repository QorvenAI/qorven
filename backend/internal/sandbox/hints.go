// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package sandbox

import "strings"

// Hint messages for LLM consumption
const (
	hintBinaryNotFound = "\n\n[SANDBOX] This command ran inside a Docker sandbox container. " +
		"The required tool/binary is not installed in the sandbox image. " +
		"Tell the user this failed due to sandbox environment limitations — " +
		"they can install the binary in the sandbox image or disable sandbox mode for this agent."

	hintPermissionDenied = "\n\n[SANDBOX] Permission denied inside sandbox container. " +
		"The workspace may be mounted as read-only (workspace_access: ro). " +
		"Check the agent's sandbox configuration or tell the user to change workspace_access to rw."

	hintNetworkDisabled = "\n\n[SANDBOX] Network operation failed — sandbox networking is disabled (--network none). " +
		"If this agent needs internet access, tell the user to enable network_enabled in the agent's sandbox configuration."

	hintReadOnlyFS = "\n\n[SANDBOX] Write failed — target path is outside the mounted workspace volume. " +
		"The sandbox filesystem is read-only except for the workspace mount. " +
		"Ensure all file operations use paths within the workspace directory."

	hintNoSuchFile = "\n\n[SANDBOX] File or directory not found inside sandbox container. " +
		"The sandbox has a minimal filesystem — only the workspace mount and installed packages are available. " +
		"Verify the path exists within the workspace, or install required files in the sandbox image."

	hintResourceLimit = "\n\n[SANDBOX] Sandbox resource limit reached (disk space or memory). " +
		"The container has restricted resources. Tell the user to increase sandbox limits " +
		"or clean up temporary files in the workspace."
)

// isBinaryNotFound returns true if the error indicates a missing binary.
func isBinaryNotFound(exitCode int, output string) bool {
	if exitCode == 127 {
		return true
	}
	lower := strings.ToLower(output)
	return strings.Contains(lower, "not found") &&
		(strings.Contains(lower, "command") || strings.Contains(lower, "sh:"))
}

func isPermissionDenied(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "permission denied") || strings.Contains(lower, "eacces")
}

func isNetworkDisabled(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "network is unreachable") || strings.Contains(lower, "name resolution")
}

func isReadOnlyFS(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "read-only file system") || strings.Contains(lower, "erofs")
}

func isResourceLimit(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "no space left") ||
		strings.Contains(lower, "enospc") ||
		strings.Contains(lower, "cannot allocate memory")
}

// MaybeSandboxHint returns an LLM-actionable hint suffix for sandbox exec errors.
func MaybeSandboxHint(exitCode int, output string) string {
	if isBinaryNotFound(exitCode, output) {
		return hintBinaryNotFound
	}
	if isPermissionDenied(output) {
		return hintPermissionDenied
	}
	if isNetworkDisabled(output) {
		return hintNetworkDisabled
	}
	if isReadOnlyFS(output) {
		return hintReadOnlyFS
	}
	if isResourceLimit(output) {
		return hintResourceLimit
	}
	return ""
}

// MaybeFsBridgeHint returns an LLM-actionable hint for FsBridge errors.
func MaybeFsBridgeHint(output string) string {
	if strings.Contains(output, "No such file") {
		return hintNoSuchFile
	}
	if isPermissionDenied(output) {
		return hintPermissionDenied
	}
	if isReadOnlyFS(output) {
		return hintReadOnlyFS
	}
	return ""
}
