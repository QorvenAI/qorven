// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Backend is the plugin interface for memory storage.
// Users implement this to use Redis, Pinecone, Qdrant, Chroma, or any vector store.
// The default implementation is PostgreSQL (pgvector).
type Backend interface {
	// Name returns the backend identifier (e.g. "postgresql", "redis", "pinecone").
	Name() string

	// Store persists a memory with its embedding vector.
	Store(ctx context.Context, opts StoreOpts) (string, error)

	// Search finds memories by semantic similarity or text match.
	Search(ctx context.Context, opts SearchOpts) ([]SearchResult, error)

	// Delete removes a memory by ID.
	Delete(ctx context.Context, tenantID, id string) error

	// List returns memories matching a filter.
	List(ctx context.Context, opts ListOpts) ([]Memory, error)

	// Close releases backend resources.
	Close() error
}

// StoreOpts contains all parameters for storing a memory.
type StoreOpts struct {
	TenantID  string
	AgentID   string
	Scope     Scope
	TeamID    string // for team-scoped
	TaskID    string // for task-scoped
	SessionID string // for session-scoped
	Type      string
	Content   string
	Source    string
	Embedding []float32 // optional: pre-computed embedding vector
	Importance float64
	DecayExempt bool
	Metadata   map[string]string
}

// SearchOpts contains all parameters for searching memories.
type SearchOpts struct {
	TenantID   string
	AgentID    string
	Scope      Scope    // filter by scope (empty = all scopes)
	TeamID     string   // required for team scope
	TaskID     string   // required for task scope
	Query      string   // text query
	Embedding  []float32 // optional: pre-computed query embedding
	MaxResults int
	MinScore   float64 // minimum similarity threshold
}

// ListOpts contains parameters for listing memories.
type ListOpts struct {
	TenantID string
	AgentID  string
	Scope    Scope
	TeamID   string
	TaskID   string
	Type     string // filter by type prefix
	Limit    int
	Offset   int
}

// BackendRegistry manages available memory backends.
// Users register backends at startup; the active backend is selected per-tenant via config.
type BackendRegistry struct {
	mu       sync.RWMutex
	backends map[string]Backend
	active   string // name of the active backend
}

// NewBackendRegistry creates a registry with the given default backend.
func NewBackendRegistry(defaultBackend Backend) *BackendRegistry {
	r := &BackendRegistry{
		backends: make(map[string]Backend),
		active:   defaultBackend.Name(),
	}
	r.backends[defaultBackend.Name()] = defaultBackend
	return r
}

// Register adds a backend to the registry.
func (r *BackendRegistry) Register(b Backend) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.backends[b.Name()] = b
	slog.Info("memory backend registered", "name", b.Name())
}

// SetActive switches the active backend.
func (r *BackendRegistry) SetActive(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.backends[name]; !ok {
		return fmt.Errorf("memory backend %q not registered", name)
	}
	r.active = name
	slog.Info("memory backend activated", "name", name)
	return nil
}

// Active returns the currently active backend.
func (r *BackendRegistry) Active() Backend {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.backends[r.active]
}

// Get returns a specific backend by name.
func (r *BackendRegistry) Get(name string) (Backend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	return b, ok
}

// List returns all registered backend names.
func (r *BackendRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	return names
}

// CloseAll closes all registered backends.
func (r *BackendRegistry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, b := range r.backends {
		if err := b.Close(); err != nil {
			slog.Warn("memory backend close error", "name", name, "error", err)
		}
	}
}
