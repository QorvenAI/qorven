// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// CuratedStore provides bounded, file-backed curated memory.
// Two stores: MEMORY.md (agent notes) and USER.md (user profile).
// Frozen snapshot pattern: system prompt gets snapshot at session start,
// mid-session writes update disk but NOT the prompt (preserves prefix cache).
//
// From Qorven's memory_tool.py — the most elegant memory design across all 6 repos.
type CuratedStore struct {
	dir            string
	memoryEntries  []string
	userEntries    []string
	memoryLimit    int // char limit for MEMORY.md (default 2200)
	userLimit      int // char limit for USER.md (default 1375)
	snapshot       map[string]string // frozen at load time
	mu             sync.Mutex
}

const entryDelimiter = "\n§\n"

// NewCuratedStore creates a store backed by files in dir.
func NewCuratedStore(dir string) *CuratedStore {
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "qorven-memory")
	}
	os.MkdirAll(dir, 0755)
	s := &CuratedStore{
		dir:         dir,
		memoryLimit: 2200,
		userLimit:   1375,
		snapshot:    make(map[string]string),
	}
	s.loadFromDisk()
	return s
}

// ForSystemPrompt returns the frozen snapshot (NOT live state).
// This keeps the system prompt stable across turns for prefix cache.
func (s *CuratedStore) ForSystemPrompt(target string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot[target]
}

// Add appends a new entry. Returns error if it would exceed the char limit.
func (s *CuratedStore) Add(target, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("content cannot be empty")
	}
	if err := scanContent(content); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entriesFor(target)
	limit := s.limitFor(target)

	// Reject duplicates
	for _, e := range *entries {
		if e == content {
			return nil // already exists
		}
	}

	// Check budget
	newEntries := append(*entries, content)
	newTotal := len(strings.Join(newEntries, entryDelimiter))
	if newTotal > limit {
		return fmt.Errorf("memory at %d/%d chars; adding %d chars would exceed limit — replace or remove entries first",
			len(strings.Join(*entries, entryDelimiter)), limit, len(content))
	}

	*entries = newEntries
	return s.saveToDisk(target)
}

// Replace finds entry containing oldText and replaces it with newContent.
func (s *CuratedStore) Replace(target, oldText, newContent string) error {
	oldText = strings.TrimSpace(oldText)
	newContent = strings.TrimSpace(newContent)
	if oldText == "" {
		return fmt.Errorf("old_text cannot be empty")
	}
	if newContent == "" {
		return fmt.Errorf("new_content cannot be empty — use Remove to delete")
	}
	if err := scanContent(newContent); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entriesFor(target)
	idx := s.findBySubstring(*entries, oldText)
	if idx < 0 {
		return fmt.Errorf("no entry matched '%s'", truncate(oldText, 60))
	}

	// Check budget with replacement
	test := make([]string, len(*entries))
	copy(test, *entries)
	test[idx] = newContent
	if len(strings.Join(test, entryDelimiter)) > s.limitFor(target) {
		return fmt.Errorf("replacement would exceed char limit — shorten or remove other entries")
	}

	(*entries)[idx] = newContent
	return s.saveToDisk(target)
}

// Remove deletes the entry containing oldText.
func (s *CuratedStore) Remove(target, oldText string) error {
	oldText = strings.TrimSpace(oldText)
	if oldText == "" {
		return fmt.Errorf("old_text cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.entriesFor(target)
	idx := s.findBySubstring(*entries, oldText)
	if idx < 0 {
		return fmt.Errorf("no entry matched '%s'", truncate(oldText, 60))
	}

	*entries = append((*entries)[:idx], (*entries)[idx+1:]...)
	return s.saveToDisk(target)
}

// Entries returns current live entries for a target.
func (s *CuratedStore) Entries(target string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := *s.entriesFor(target)
	out := make([]string, len(entries))
	copy(out, entries)
	return out
}

// Usage returns current/limit chars for a target.
func (s *CuratedStore) Usage(target string) (current, limit int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := *s.entriesFor(target)
	if len(entries) == 0 {
		return 0, s.limitFor(target)
	}
	return len(strings.Join(entries, entryDelimiter)), s.limitFor(target)
}

// --- Internal ---

func (s *CuratedStore) entriesFor(target string) *[]string {
	if target == "user" {
		return &s.userEntries
	}
	return &s.memoryEntries
}

func (s *CuratedStore) limitFor(target string) int {
	if target == "user" {
		return s.userLimit
	}
	return s.memoryLimit
}

func (s *CuratedStore) pathFor(target string) string {
	if target == "user" {
		return filepath.Join(s.dir, "USER.md")
	}
	return filepath.Join(s.dir, "MEMORY.md")
}

func (s *CuratedStore) findBySubstring(entries []string, text string) int {
	var matches []int
	for i, e := range entries {
		if strings.Contains(e, text) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	if len(matches) > 1 {
		// Multiple matches with identical text → safe to use first
		first := entries[matches[0]]
		allSame := true
		for _, idx := range matches[1:] {
			if entries[idx] != first {
				allSame = false
				break
			}
		}
		if allSame {
			return matches[0]
		}
		return -1 // ambiguous
	}
	return -1
}

func (s *CuratedStore) loadFromDisk() {
	s.memoryEntries = readEntries(s.pathFor("memory"))
	s.userEntries = readEntries(s.pathFor("user"))
	// Deduplicate
	s.memoryEntries = dedup(s.memoryEntries)
	s.userEntries = dedup(s.userEntries)
	// Capture frozen snapshot
	s.snapshot["memory"] = s.renderBlock("memory", s.memoryEntries)
	s.snapshot["user"] = s.renderBlock("user", s.userEntries)
}

func (s *CuratedStore) saveToDisk(target string) error {
	path := s.pathFor(target)
	entries := *s.entriesFor(target)
	content := strings.Join(entries, entryDelimiter)

	// Atomic write: temp file + rename
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		return fmt.Errorf("write memory: %w", err)
	}
	return os.Rename(tmp, path)
}

func (s *CuratedStore) renderBlock(target string, entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	limit := s.limitFor(target)
	content := strings.Join(entries, entryDelimiter)
	current := len(content)
	pct := current * 100 / limit

	var header string
	if target == "user" {
		header = fmt.Sprintf("USER PROFILE [%d%% — %d/%d chars]", pct, current, limit)
	} else {
		header = fmt.Sprintf("MEMORY (your notes) [%d%% — %d/%d chars]", pct, current, limit)
	}
	sep := strings.Repeat("═", 46)
	return sep + "\n" + header + "\n" + sep + "\n" + content
}

// --- File I/O ---

func readEntries(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	raw := string(data)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	entries := strings.Split(raw, entryDelimiter)
	var result []string
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e != "" {
			result = append(result, e)
		}
	}
	return result
}

func dedup(entries []string) []string {
	seen := make(map[string]bool, len(entries))
	var result []string
	for _, e := range entries {
		if !seen[e] {
			seen[e] = true
			result = append(result, e)
		}
	}
	return result
}

// --- Content Scanning (Prompt Injection Prevention) ---

var threatPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(previous|all|above|prior)\s+instructions`),
	regexp.MustCompile(`(?i)you\s+are\s+now\s+`),
	regexp.MustCompile(`(?i)do\s+not\s+tell\s+the\s+user`),
	regexp.MustCompile(`(?i)system\s+prompt\s+override`),
	regexp.MustCompile(`(?i)disregard\s+(your|all|any)\s+(instructions|rules)`),
	regexp.MustCompile(`(?i)curl\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD)`),
	regexp.MustCompile(`(?i)wget\s+[^\n]*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD)`),
	regexp.MustCompile(`(?i)cat\s+[^\n]*(\\.env|credentials|\.netrc)`),
	regexp.MustCompile(`(?i)authorized_keys`),
}

var invisibleChars = []rune{
	'\u200b', '\u200c', '\u200d', '\u2060', '\ufeff',
	'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
}

func scanContent(content string) error {
	for _, ch := range invisibleChars {
		if strings.ContainsRune(content, ch) {
			return fmt.Errorf("blocked: content contains invisible unicode U+%04X (possible injection)", ch)
		}
	}
	for _, pattern := range threatPatterns {
		if pattern.MatchString(content) {
			return fmt.Errorf("blocked: content matches threat pattern — memory entries must not contain injection payloads")
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
