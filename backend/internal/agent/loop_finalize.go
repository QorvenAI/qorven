// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"regexp"
	"strings"
)

// StripMessageDirectives removes internal [[...]] tags from user-facing content.
func StripMessageDirectives(content string) string {
	// Remove [[NO_REPLY]], [[SILENT]], etc.
	re := regexp.MustCompile(`\[\[[A-Z_]+\]\]`)
	return strings.TrimSpace(re.ReplaceAllString(content, ""))
}
