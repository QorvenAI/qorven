// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package config

import (
	"regexp"
	"strings"
)

const DefaultAgentID = "default"

var (
	validIDRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	invalidChars = regexp.MustCompile(`[^a-z0-9_-]+`)
	leadingDash  = regexp.MustCompile(`^-+`)
	trailingDash = regexp.MustCompile(`-+$`)
)

// NormalizeAgentID converts a user-provided name into a valid agent ID.
// Lowercase, max 64 chars, only [a-z0-9_-], invalid chars → "-".
func NormalizeAgentID(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return DefaultAgentID
	}
	lower := strings.ToLower(trimmed)
	if validIDRe.MatchString(lower) {
		return lower
	}
	result := invalidChars.ReplaceAllString(lower, "-")
	result = leadingDash.ReplaceAllString(result, "")
	result = trailingDash.ReplaceAllString(result, "")
	if len(result) > 64 {
		result = result[:64]
	}
	if result == "" {
		return DefaultAgentID
	}
	return result
}
