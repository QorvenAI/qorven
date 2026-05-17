// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tui

// Session persistence — saves/loads chat history to ~/.qorven/sessions/<sessionID>.json
// Allows messages to survive TUI restarts and provides a local message cache.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type persistedSession struct {
	SessionID  string        `json:"session_id"`
	AgentID    string        `json:"agent_id"`
	AgentName  string        `json:"agent_name"`
	ModelName  string        `json:"model_name"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Messages   []ChatMessage `json:"messages"`
}

// sessionDir returns the directory where sessions are persisted.
func sessionDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".qorven", "sessions")
}

// sessionPath returns the path for a specific session file.
func sessionPath(sessionID string) string {
	// Sanitize session ID for use as filename
	safe := strings.ReplaceAll(sessionID, "/", "_")
	safe = strings.ReplaceAll(safe, ":", "_")
	return filepath.Join(sessionDir(), safe+".json")
}

// saveSession persists the current session messages to disk.
// Called automatically when messages change and on TUI exit.
func saveSession(sessionID, agentID, agentName, modelName string, messages []ChatMessage) error {
	if sessionID == "" || len(messages) == 0 { return nil }

	dir := sessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil { return err }

	// Only save user+assistant messages, skip system messages for cleaner history
	var toSave []ChatMessage
	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			toSave = append(toSave, msg)
		}
	}
	if len(toSave) == 0 { return nil }

	data, err := json.MarshalIndent(persistedSession{
		SessionID: sessionID,
		AgentID:   agentID,
		AgentName: agentName,
		ModelName: modelName,
		UpdatedAt: time.Now(),
		Messages:  toSave,
	}, "", "  ")
	if err != nil { return err }

	return os.WriteFile(sessionPath(sessionID), data, 0600)
}

// loadSession restores messages from disk for a given session ID.
// Returns nil if no persisted session exists.
func loadSession(sessionID string) []ChatMessage {
	if sessionID == "" { return nil }
	data, err := os.ReadFile(sessionPath(sessionID))
	if err != nil { return nil }

	var sess persistedSession
	if err := json.Unmarshal(data, &sess); err != nil { return nil }
	return sess.Messages
}

// listPersistedSessions returns all locally persisted sessions (newest first).
func listPersistedSessions() []persistedSession {
	dir := sessionDir()
	entries, err := os.ReadDir(dir)
	if err != nil { return nil }

	var sessions []persistedSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") { continue }
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil { continue }
		var sess persistedSession
		if err := json.Unmarshal(data, &sess); err != nil { continue }
		sessions = append(sessions, sess)
	}
	// Sort by UpdatedAt descending
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && sessions[j].UpdatedAt.After(sessions[j-1].UpdatedAt); j-- {
			sessions[j], sessions[j-1] = sessions[j-1], sessions[j]
		}
	}
	return sessions
}

// deletePersistedSession removes a session file from disk.
func deletePersistedSession(sessionID string) {
	os.Remove(sessionPath(sessionID))
}

// exportSessionToFile writes the current session messages to an export JSON file
// in ~/.qorven/sessions/ and returns a tea.Cmd that appends a system message.
func (m *Model) exportSessionToFile() tea.Cmd {
	if len(m.messages) == 0 {
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Nothing to export — session is empty"})
		m.updateViewport()
		return nil
	}

	dir := sessionDir()
	_ = os.MkdirAll(dir, 0700)

	ts := time.Now().Format("20060102-150405")
	id := m.sessionID
	if id == "" {
		id = "local"
	}
	filename := "export-" + id + "-" + ts + ".json"
	path := filepath.Join(dir, filename)

	var toExport []ChatMessage
	for _, msg := range m.messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			toExport = append(toExport, msg)
		}
	}

	data, err := json.MarshalIndent(persistedSession{
		SessionID: id,
		AgentID:   m.agentID,
		AgentName: m.agentName,
		ModelName: m.modelName,
		UpdatedAt: time.Now(),
		Messages:  toExport,
	}, "", "  ")
	if err != nil {
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Export failed: " + err.Error()})
		m.updateViewport()
		return nil
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		m.messages = append(m.messages, ChatMessage{Role: "system", Content: "Export failed: " + err.Error()})
		m.updateViewport()
		return nil
	}

	m.messages = append(m.messages, ChatMessage{Role: "system", Content: fmt.Sprintf("✓ Exported %d messages to %s", len(toExport), path)})
	m.updateViewport()
	return nil
}

// loadSessionFromFile deserializes a JSON session file into a slice of ChatMessages.
func loadSessionFromFile(path string) ([]ChatMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess persistedSession
	if err := json.Unmarshal(data, &sess); err != nil {
		// Try bare []ChatMessage format as fallback
		var msgs []ChatMessage
		if err2 := json.Unmarshal(data, &msgs); err2 != nil {
			return nil, err
		}
		return msgs, nil
	}
	return sess.Messages, nil
}

// pruneOldSessions removes persisted sessions older than maxAge.
// Keeps at most maxSessions files.
func pruneOldSessions(maxAge time.Duration, maxSessions int) {
	dir := sessionDir()
	entries, err := os.ReadDir(dir)
	if err != nil { return }

	type entry struct {
		path    string
		modTime time.Time
	}
	var files []entry
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") { continue }
		info, err := e.Info()
		if err != nil { continue }
		path := filepath.Join(dir, e.Name())
		if time.Since(info.ModTime()) > maxAge {
			os.Remove(path)
			continue
		}
		files = append(files, entry{path, info.ModTime()})
	}

	// Sort newest first, delete oldest if over limit
	for i := 1; i < len(files); i++ {
		for j := i; j > 0 && files[j].modTime.After(files[j-1].modTime); j-- {
			files[j], files[j-1] = files[j-1], files[j]
		}
	}
	for i := maxSessions; i < len(files); i++ {
		os.Remove(files[i].path)
	}
}
