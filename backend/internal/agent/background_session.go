// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BackgroundSession is a persistent agent session that survives across API calls.
// Each session has its own workspace, environment, and conversation state
// that persists until explicitly closed.
type BackgroundSession struct {
	ID           string            `json:"id"`
	// TenantID scopes the session so two tenants that happen to generate
	// the same sessionID (UUID collision, predictable key) get separate
	// workspaces. Sessions created via the legacy Create() (no tenant)
	// live under "" and are effectively single-tenant.
	TenantID     string            `json:"tenant_id,omitempty"`
	AgentID      string            `json:"agent_id"`
	Workspace    string            `json:"workspace"`   // persistent workspace directory
	Env          map[string]string `json:"env"`         // persistent environment variables
	CreatedAt    time.Time         `json:"created_at"`
	LastActiveAt time.Time         `json:"last_active_at"`
	MessageCount int               `json:"message_count"`
	Status       string            `json:"status"` // "active", "idle", "closed"

	// Internal state
	mu          sync.Mutex
	processes   map[string]*ManagedProcess // tracked background processes
}

// sessionKey composes the in-memory + workspace-path key. Including
// tenantID prevents cross-tenant aliasing on the `sessions` map and
// on-disk workspaces. Empty tenant falls back to a single-tenant
// layout so existing code paths keep working.
func sessionKey(tenantID, sessionID string) string {
	if tenantID == "" {
		return sessionID
	}
	return tenantID + "/" + sessionID
}

// ManagedProcess tracks a background process started by the agent.
type ManagedProcess struct {
	PID       int       `json:"pid"`
	Command   string    `json:"command"`
	StartedAt time.Time `json:"started_at"`
	Port      int       `json:"port,omitempty"` // if it's a server
}

// BackgroundSessionManager manages persistent sessions.
type BackgroundSessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*BackgroundSession
	baseDir  string // base directory for session workspaces
}

// NewBackgroundSessionManager creates a new manager.
func NewBackgroundSessionManager(baseDir string) *BackgroundSessionManager {
	if baseDir == "" {
		baseDir = filepath.Join(os.TempDir(), "qorven-sessions")
	}
	os.MkdirAll(baseDir, 0755)
	return &BackgroundSessionManager{
		sessions: make(map[string]*BackgroundSession),
		baseDir:  baseDir,
	}
}

// Create creates a persistent background session without tenant
// scoping. Kept for backward compatibility with single-tenant callers.
// New code should prefer CreateForTenant so two tenants with the same
// generated sessionID don't share a workspace.
func (m *BackgroundSessionManager) Create(ctx context.Context, agentID, sessionID string) (*BackgroundSession, error) {
	return m.CreateForTenant(ctx, "", agentID, sessionID)
}

// CreateForTenant creates a persistent session scoped to tenantID.
// Map keys and on-disk workspace paths both include the tenant so
// the worst-case "two tenants, colliding sessionID" scenario gives
// isolated state instead of a shared workspace.
func (m *BackgroundSessionManager) CreateForTenant(ctx context.Context, tenantID, agentID, sessionID string) (*BackgroundSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := sessionKey(tenantID, sessionID)
	if existing, ok := m.sessions[key]; ok {
		existing.LastActiveAt = time.Now()
		return existing, nil
	}

	// Workspace path includes tenantID. For tenantID="" this reduces
	// to the original `baseDir/sessionID` layout, preserving the
	// single-tenant on-disk structure that existing installs have.
	var workspace string
	if tenantID == "" {
		workspace = filepath.Join(m.baseDir, sessionID)
	} else {
		workspace = filepath.Join(m.baseDir, tenantID, sessionID)
	}
	if err := os.MkdirAll(workspace, 0755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	session := &BackgroundSession{
		ID:           sessionID,
		TenantID:     tenantID,
		AgentID:      agentID,
		Workspace:    workspace,
		Env:          make(map[string]string),
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		Status:       "active",
		processes:    make(map[string]*ManagedProcess),
	}

	m.sessions[key] = session
	slog.Info("background_session.created",
		"session", sessionID, "tenant", tenantID, "agent", agentID, "workspace", workspace)
	return session, nil
}

// Get retrieves an existing session.
func (m *BackgroundSessionManager) Get(sessionID string) (*BackgroundSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[sessionID]
	if ok {
		s.LastActiveAt = time.Now()
	}
	return s, ok
}

// Touch marks a session as active.
func (m *BackgroundSessionManager) Touch(sessionID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.mu.Lock()
		s.LastActiveAt = time.Now()
		s.MessageCount++
		s.mu.Unlock()
	}
}

// Close closes a session and cleans up its workspace.
func (m *BackgroundSessionManager) Close(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Kill tracked processes
	session.mu.Lock()
	for _, proc := range session.processes {
		if p, err := os.FindProcess(proc.PID); err == nil {
			p.Signal(os.Interrupt)
			slog.Info("background_session.kill_process", "session", sessionID, "pid", proc.PID, "cmd", proc.Command)
		}
	}
	session.processes = nil
	session.Status = "closed"
	session.mu.Unlock()

	delete(m.sessions, sessionID)
	slog.Info("background_session.closed", "session", sessionID)
	return nil
}

// List returns all active sessions.
func (m *BackgroundSessionManager) List() []*BackgroundSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessions := make([]*BackgroundSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// CleanupIdle closes sessions that have been idle for longer than maxIdle.
func (m *BackgroundSessionManager) CleanupIdle(maxIdle time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxIdle)
	var closed int
	for id, s := range m.sessions {
		if s.LastActiveAt.Before(cutoff) && s.Status != "closed" {
			s.Status = "closed"
			delete(m.sessions, id)
			closed++
			slog.Info("background_session.idle_cleanup", "session", id, "idle_since", s.LastActiveAt)
		}
	}
	return closed
}

// SetEnv sets an environment variable for the session.
func (s *BackgroundSession) SetEnv(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Env[key] = value
}

// GetEnv gets an environment variable from the session.
func (s *BackgroundSession) GetEnv(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Env[key]
}

// TrackProcess registers a background process with the session.
func (s *BackgroundSession) TrackProcess(pid int, command string, port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes[fmt.Sprintf("%d", pid)] = &ManagedProcess{
		PID:       pid,
		Command:   command,
		StartedAt: time.Now(),
		Port:      port,
	}
}

// ListProcesses returns all tracked processes.
func (s *BackgroundSession) ListProcesses() []ManagedProcess {
	s.mu.Lock()
	defer s.mu.Unlock()
	procs := make([]ManagedProcess, 0, len(s.processes))
	for _, p := range s.processes {
		procs = append(procs, *p)
	}
	return procs
}

// EnvSlice returns the session's env vars as a slice for exec.Cmd.Env.
func (s *BackgroundSession) EnvSlice() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	env := make([]string, 0, len(s.Env))
	for k, v := range s.Env {
		env = append(env, k+"="+v)
	}
	return env
}
