// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/providers"
)

type channelBinding struct {
	ChannelType string            `json:"channel_type"`
	DisplayName string            `json:"display_name"`
	Credentials map[string]string `json:"credentials"`
	Config      map[string]string `json:"config"`
}

func (gw *Gateway) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 200, map[string]any{"agents": []any{}})
		return
	}
	agents, err := gw.agents.List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if agents == nil {
		agents = []*agent.Agent{}
	}
	writeJSON(w, 200, map[string]any{"agents": agents})
}

func (gw *Gateway) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		agent.CreateAgentInput
		Skills   []string         `json:"skills"`
		Channels []channelBinding `json:"channels"`
		Role     string           `json:"role"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	a, err := gw.agents.Create(r.Context(), defaultTenant, body.CreateAgentInput)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Set role (prime, lead, member)
	if body.Role != "" && gw.db != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE agents SET role = $1 WHERE id = $2`, body.Role, a.ID)
	}

	// Bind channels
	boundChannels := []string{}
	if len(body.Channels) > 0 && gw.bindingStore != nil {
		for _, ch := range body.Channels {
			credJSON, _ := json.Marshal(ch.Credentials)
			cfgJSON, _ := json.Marshal(ch.Config)
			binding := channels.AgentBinding{
				AgentID:     a.ID,
				TenantID:    defaultTenant,
				ChannelType: ch.ChannelType,
				DisplayName: ch.DisplayName,
				Credentials: credJSON,
				Config:      cfgJSON,
				Enabled:     true,
			}
			if _, err := gw.bindingStore.Create(r.Context(), binding); err != nil {
				slog.Warn("channel bind failed", "agent", a.ID, "type", ch.ChannelType, "error", err)
			} else {
				boundChannels = append(boundChannels, ch.ChannelType)
			}
		}
	}

	// Auto-install skills
	installed := []string{}
	if len(body.Skills) > 0 && gw.skillStore != nil {
		for _, slug := range body.Skills {
			if gw.skillStore.Install(r.Context(), a.ID, slug) == nil {
				installed = append(installed, slug)
			}
		}
	}

	// Write soul bundle — either from system_prompt or seeded from role defaults
	if gw.bundleStore != nil {
		soulContent := body.SystemPrompt
		if soulContent == "" {
			switch body.CreateAgentInput.Role {
			case "chief", "prime":
				soulContent = fmt.Sprintf("You are %s, a Chief of Staff AI assistant. You have full access to all tools and can delegate tasks to specialist agents.", body.DisplayName)
			case "developer":
				soulContent = fmt.Sprintf("You are %s, a software development specialist. You excel at writing, reviewing, and debugging code across all languages and frameworks.", body.DisplayName)
			case "researcher":
				soulContent = fmt.Sprintf("You are %s, a research specialist. You excel at finding accurate information, synthesizing sources, and producing clear research reports.", body.DisplayName)
			case "writer":
				soulContent = fmt.Sprintf("You are %s, a creative writing specialist. You excel at producing compelling content, copy, and communications in any voice or style.", body.DisplayName)
			default:
				soulContent = fmt.Sprintf("You are %s, an AI specialist. Execute tasks efficiently and report back with clear, concise results.", body.DisplayName)
			}
			gw.bundleStore.SeedDefaults(r.Context(), a.ID, string(body.CreateAgentInput.Role))
		}
		gw.bundleStore.Upsert(r.Context(), agent.Bundle{
			AgentID:    a.ID,
			BundleType: "soul",
			Name:       "soul",
			Content:    soulContent,
			Priority:   200,
			Enabled:    true,
		})
	}

	result := map[string]any{"id": a.ID, "agent_key": a.AgentKey, "display_name": a.DisplayName}
	if body.Role != "" {
		result["role"] = body.Role
	}
	if len(boundChannels) > 0 {
		result["channels"] = boundChannels
	}
	if len(installed) > 0 {
		result["skills_installed"] = installed
	}
	writeJSON(w, 201, result)
}

// handleGenerateSoul calls the LLM to produce a rich soul prompt from a user description.
func (gw *Gateway) handleGenerateSoul(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		Description string `json:"description"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name required"})
		return
	}
	if gw.providerReg == nil {
		writeJSON(w, 503, map[string]string{"error": "no LLM provider configured"})
		return
	}
	provider := gw.providerReg.Default()
	if provider == nil {
		writeJSON(w, 503, map[string]string{"error": "no default provider available"})
		return
	}

	roleHint := req.Role
	if roleHint == "" { roleHint = "specialist" }
	descHint := req.Description
	if descHint == "" { descHint = fmt.Sprintf("a helpful AI %s", roleHint) }

	prompt := fmt.Sprintf(`Write a soul prompt for an AI agent named %q on the Qorven platform.
Role: %s
Description from creator: %s

A soul prompt is the system-level identity instruction injected before every conversation.
Rules:
- Write in second person ("You are...")
- Be specific, concrete, not generic
- Structure: Identity → Communication style → Core capabilities (3-5 bullets) → Behaviour rules (2-3 bullets)
- Under 300 words
- Plain text only, no markdown headers
- Return only the soul prompt, nothing else`, req.Name, roleHint, descHint)

	var model string
	if gw.agentLoop != nil && gw.agentLoop.SmartRouter != nil {
		model = gw.agentLoop.SmartRouter.BestModelForTier(providers.TierStandard)
	}
	resp, err := provider.Chat(r.Context(), providers.ChatRequest{
		Model:    model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"soul": strings.TrimSpace(resp.Content)})
}

func (gw *Gateway) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	a, err := gw.agents.GetForTenant(r.Context(), chi.URLParam(r, "id"), defaultTenant)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, 200, a)
}

func (gw *Gateway) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var updates map[string]any
	if json.NewDecoder(r.Body).Decode(&updates) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	// Remove fields that shouldn't be updated directly
	delete(updates, "id")
	delete(updates, "tenant_id")
	delete(updates, "created_at")

	if err := gw.agents.Update(r.Context(), id, updates); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Invalidate prompt cache for this agent
	if gw.agentLoop != nil {
		gw.agentLoop.InvalidatePromptCache(id)
	}

	writeJSON(w, 200, map[string]any{"id": id, "updated": len(updates)})
}

func (gw *Gateway) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	gw.agents.Delete(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleGetChief(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	chief, err := gw.ensureChief(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, chief)
}

func (gw *Gateway) ensureChief(ctx context.Context) (*agent.Agent, error) {
	existing, err := gw.agents.GetByKey(ctx, "chief")
	if err == nil && existing != nil {
		return existing, nil
	}
	return gw.agents.Create(ctx, defaultTenant, agent.ChiefSpec())
}
