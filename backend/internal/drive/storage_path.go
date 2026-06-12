// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package drive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultDriveRoot is the persistent base directory for all Drive files.
// Production: /var/lib/qorven/drive. Overridable via QORVEN_DRIVE_ROOT.
// It is deliberately NOT under /tmp — /tmp is cleared on reboot and would
// lose every uploaded file and surfaced workspace document.
const DefaultDriveRoot = "/var/lib/qorven/drive"

// DriveRoot returns the configured persistent Drive root.
func DriveRoot() string {
	if root := os.Getenv("QORVEN_DRIVE_ROOT"); root != "" {
		return root
	}
	return DefaultDriveRoot
}

// sanitizeSegment strips path separators and traversal from a single path
// component so a crafted tenant/agent id or filename cannot escape the root.
func sanitizeSegment(s string) string {
	s = strings.ReplaceAll(s, "..", "")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.TrimSpace(s)
	if s == "" {
		s = "_"
	}
	return s
}

// DriveFilePath returns the on-disk path for a tenant/agent/file under the root.
func DriveFilePath(tenantID, agentID, name string) string {
	return filepath.Join(DriveRoot(), sanitizeSegment(tenantID), sanitizeSegment(agentID), sanitizeSegment(name))
}

// ValidateUnderRoot returns an error if path is not within DriveRoot after
// cleaning — the single guard used by the download handler to prevent
// arbitrary-file reads.
func ValidateUnderRoot(path string) error {
	root := filepath.Clean(DriveRoot())
	clean := filepath.Clean(path)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return fmt.Errorf("path %q is outside the drive root", path)
	}
	return nil
}
