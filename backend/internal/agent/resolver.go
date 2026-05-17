// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// ResolverDeps holds shared dependencies for the agent resolver.
type ResolverDeps struct {
	AgentStore    AgentDataStore
	ProviderReg   *providers.Registry
	Tools         *tools.Registry
	HasMemory     bool
	OnEvent       func(StreamEvent)

	// Security
	InjectionAction string // "log", "warn", "block", "off"
	MaxMessageChars int

	// Global defaults
	ContextWindow int
	MaxIterations int
	Workspace     string
}

// AgentDataStore interface for agent data access.
type AgentDataStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*AgentConfig, error)
	GetByKey(ctx context.Context, key string) (*AgentConfig, error)
}

// AgentConfig represents stored agent configuration for resolution.
type AgentConfig struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	AgentKey          string
	AgentType         string
	Provider          string
	Model             string
	ContextWindow     int
	MaxToolIterations int
	Workspace         string
	Status            string
}

// ResolverFunc resolves an agent by key.
type ResolverFunc func(ctx context.Context, agentKey string) (*Loop, error)

// NewManagedResolver creates a ResolverFunc that builds Loops from stored agent data.
// Note: This is a simplified resolver. Full implementation requires wiring all Loop dependencies.
func NewManagedResolver(deps ResolverDeps) ResolverFunc {
	return func(ctx context.Context, agentKey string) (*Loop, error) {
		// Support lookup by UUID
		var ag *AgentConfig
		var err error
		if id, parseErr := uuid.Parse(agentKey); parseErr == nil {
			ag, err = deps.AgentStore.GetByID(ctx, id)
		} else {
			ag, err = deps.AgentStore.GetByKey(ctx, agentKey)
		}
		if err != nil {
			return nil, fmt.Errorf("agent not found: %s", agentKey)
		}

		if ag.Status != "active" {
			return nil, fmt.Errorf("agent %s is inactive", agentKey)
		}

		// Resolve provider
		provider, ok := deps.ProviderReg.GetByName(ag.Provider)
		if !ok {
			configs := deps.ProviderReg.List()
			if len(configs) == 0 {
				return nil, fmt.Errorf("no providers configured for agent %s", agentKey)
			}
			provider, _ = deps.ProviderReg.GetByName(configs[0].Name)
			slog.Warn("agent provider not found, using fallback",
				"agent", agentKey, "wanted", ag.Provider, "using", configs[0].Name)
		}

		if provider == nil {
			return nil, fmt.Errorf("no provider available for agent %s", agentKey)
		}

		// Expand workspace path
		workspace := ag.Workspace
		if workspace == "" {
			workspace = deps.Workspace
		}
		workspace = ExpandWorkspace(workspace)
		if workspace != "" {
			if err := os.MkdirAll(workspace, 0755); err != nil {
				slog.Warn("failed to create agent workspace directory",
					"workspace", workspace, "agent", agentKey, "error", err)
			}
		}

		// Create loop with minimal config
		// Full implementation would wire all dependencies from deps
		loop := NewLoop(nil, nil, deps.ProviderReg, deps.Tools, nil, nil, ag.TenantID.String())

		slog.Info("resolved agent from store", "agent", agentKey, "model", ag.Model, "provider", ag.Provider)
		return loop, nil
	}
}

// InvalidateAgent removes an agent from the router cache.
func (r *Router) InvalidateAgent(agentKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	suffix := ":" + agentKey
	for key := range r.agents {
		if key == agentKey || strings.HasSuffix(key, suffix) {
			delete(r.agents, key)
		}
	}
	slog.Debug("invalidated agent cache", "agent", agentKey)
}

// InvalidateAll clears the entire agent cache.
func (r *Router) InvalidateAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents = make(map[string]*agentEntry)
	slog.Debug("invalidated all agent caches")
}
