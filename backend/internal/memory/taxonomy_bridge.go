// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

// ClassifyMemory determines the memory type from content using heuristics.
func ClassifyMemory(content, source string) MemoryType {
	// Check for feedback patterns
	feedbackPatterns := []string{"don't", "stop", "instead", "prefer", "always", "never", "correct", "wrong"}
	for _, p := range feedbackPatterns {
		if containsCI(content, p) { return MemFeedback }
	}
	// Check for reference patterns
	refPatterns := []string{"http://", "https://", "slack.com", "linear.app", "grafana", "jira", "notion"}
	for _, p := range refPatterns {
		if containsCI(content, p) { return MemReference }
	}
	// Check for project patterns
	projPatterns := []string{"deadline", "sprint", "release", "milestone", "freeze", "launch"}
	for _, p := range projPatterns {
		if containsCI(content, p) { return MemProject }
	}
	return MemUser
}

func containsCI(s, substr string) bool {
	return len(s) >= len(substr) && containsLower(s, substr)
}

func containsLower(s, sub string) bool {
	ls, lsub := toLower(s), toLower(sub)
	for i := 0; i <= len(ls)-len(lsub); i++ {
		if ls[i:i+len(lsub)] == lsub { return true }
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' { c += 32 }
		b[i] = c
	}
	return string(b)
}
