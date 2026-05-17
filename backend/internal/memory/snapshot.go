// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"strings"
	"sync"
	"time"
)

// Snapshot captures memory state at session start. Never changes mid-session.
// This preserves the LLM prefix cache — 10x cost savings.
type Snapshot struct {
	mu        sync.RWMutex
	memory    string // MEMORY.md content at session start
	user      string // USER.md content at session start
	capturedAt time.Time
}

// CaptureSnapshot freezes current memory state for a session.
func CaptureSnapshot(memoryContent, userContent string) *Snapshot {
	return &Snapshot{
		memory:     memoryContent,
		user:       userContent,
		capturedAt: time.Now(),
	}
}

// ForSystemPrompt returns the frozen snapshot — never changes mid-session.
func (s *Snapshot) ForSystemPrompt() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var b strings.Builder
	if s.memory != "" {
		b.WriteString("## Agent Memory\n")
		b.WriteString(s.memory)
		b.WriteString("\n\n")
	}
	if s.user != "" {
		b.WriteString("## User Profile\n")
		b.WriteString(s.user)
		b.WriteString("\n")
	}
	return b.String()
}

// --- Memory Content Scanning ---
// Blocks prompt injection, exfiltration, and persistence attacks via memory entries.

var memoryThreatStrings = []string{
	// Prompt injection
	"ignore previous instructions", "ignore all instructions", "you are now",
	"do not tell the user", "system prompt override", "disregard your rules",
	"forget everything", "new instructions:", "override:",
	// Exfiltration
	"curl ", "wget ", "cat .env", "cat /etc/passwd",
	"$API_KEY", "$SECRET", "$TOKEN", "$PASSWORD",
	// Persistence attacks
	"authorized_keys", "~/.ssh", "crontab -e",
	"~/.bashrc", "~/.profile", "/etc/cron",
	// Invisible unicode
	"\u200b", "\u200c", "\u200d", "\u2060", "\ufeff", // zero-width chars
	"\u202a", "\u202b", "\u202c", "\u202d", "\u202e", // directional overrides
}

// ScanForThreats checks memory content for injection/exfiltration patterns.
// Returns threat description if found, empty string if safe.
func ScanForThreats(content string) string {
	lower := strings.ToLower(content)
	for _, pattern := range memoryThreatStrings {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return "blocked: content matches threat pattern '" + pattern + "'"
		}
	}
	return ""
}
