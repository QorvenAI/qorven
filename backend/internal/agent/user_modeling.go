// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"
)

// UserProfile represents accumulated knowledge about a user.
// Built automatically by the Learning Loop from conversation patterns.
type UserProfile struct {
	Preferences map[string]string // key → description
}

// UserModeler builds and retrieves user profiles from memory.
type UserModeler struct {
	memStore interface {
		Search(ctx context.Context, tenantID, agentID, query string, limit int) ([]map[string]string, error)
	}
}

func NewUserModeler(memStore interface {
	Search(ctx context.Context, tenantID, agentID, query string, limit int) ([]map[string]string, error)
}) *UserModeler {
	return &UserModeler{memStore: memStore}
}

// BuildProfilePrompt returns a system prompt section with user preferences.
// Called during context building to inject user knowledge.
func (um *UserModeler) BuildProfilePrompt(ctx context.Context, tenantID, agentID string) string {
	if um.memStore == nil {
		return ""
	}

	results, err := um.memStore.Search(ctx, tenantID, agentID, "user_pref:", 10)
	if err != nil || len(results) == 0 {
		return ""
	}

	var prefs []string
	for _, r := range results {
		key := r["key"]
		value := r["value"]
		key = strings.TrimPrefix(key, "user_pref:")
		prefs = append(prefs, fmt.Sprintf("- %s: %s", key, value))
	}

	if len(prefs) == 0 {
		return ""
	}

	return fmt.Sprintf(`## User Profile (learned from conversations)

%s

Use these preferences to tailor your responses. Don't mention them explicitly.
`, strings.Join(prefs, "\n"))
}
