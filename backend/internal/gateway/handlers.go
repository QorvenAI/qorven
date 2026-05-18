// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"regexp"
)

// --- Command Interceptor: @mentions and /commands bypass LLM ---

const defaultTenant = "00000000-0000-0000-0000-000000000001"

var (
	cmdMentionRe = regexp.MustCompile(`@([a-zA-Z0-9_.-]+)\s+(.+)`)
	cmdSlashRe   = regexp.MustCompile(`^/([a-z_]+)(?:\s+(.*))?$`)
	mentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_.-]+)`)
)

// interceptCommand checks if the user message is a direct @mention or /command.
