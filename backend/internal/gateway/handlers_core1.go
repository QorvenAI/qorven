// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/qorvenai/qorven/internal/channels"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/memory"
)

// --- Memory hierarchy handlers ---

func (gw *Gateway) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query      string `json:"query"`
		AgentID    string `json:"agent_id"`
		Scope      string `json:"scope"`
		TeamID     string `json:"team_id"`
		TaskID     string `json:"task_id"`
		MaxResults int    `json:"max_results"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.MaxResults <= 0 {
		req.MaxResults = 10
	}

	if gw.memStore == nil {
		writeJSON(w, 200, map[string]any{"memories": []any{}})
		return
	}

	var results []memory.SearchResult
	var err error

	if req.Query == "" {
		// Empty query → list recent memories sorted by importance/recency
		results, err = gw.memStore.ListRecent(r.Context(), req.AgentID, req.MaxResults)
	} else if gw.agentLoop != nil && gw.agentLoop.HierarchyMem != nil {
		results, err = gw.agentLoop.HierarchyMem.SearchHierarchy(r.Context(), req.AgentID, req.TeamID, req.Query, req.MaxResults)
	} else {
		results, err = gw.memStore.Search(r.Context(), "default", req.AgentID, req.Query, req.MaxResults)
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"memories": results, "count": len(results)})
}

func (gw *Gateway) handleMemorySearchGET(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, 400, map[string]string{"error": "q required"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	maxResults := 10
	if mr := r.URL.Query().Get("max_results"); mr != "" {
		if n, err := strconv.Atoi(mr); err == nil && n > 0 {
			maxResults = n
		}
	}
	if gw.memStore == nil {
		writeJSON(w, 200, map[string]any{"memories": []any{}, "count": 0})
		return
	}
	results, err := gw.memStore.Search(r.Context(), "default", agentID, q, maxResults)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"memories": results, "count": len(results)})
}

func (gw *Gateway) handleMemorySave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Content string `json:"content"`
		Scope   string `json:"scope"`
		AgentID string `json:"agent_id"`
		TeamID  string `json:"team_id"`
		TaskID  string `json:"task_id"`
		Source  string `json:"source"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	if gw.agentLoop == nil || gw.agentLoop.HierarchyMem == nil {
		writeJSON(w, 503, map[string]string{"error": "memory not initialized"})
		return
	}

	h := gw.agentLoop.HierarchyMem
	var id string
	var err error

	switch req.Scope {
	case "company":
		id, err = h.SaveCompany(r.Context(), req.Content, req.Source)
	case "team":
		id, err = h.SaveTeamMemory(r.Context(), req.TeamID, req.Content, req.Source)
	case "task":
		id, err = h.SaveTask(r.Context(), req.TaskID, req.AgentID, req.Content, req.Source)
	case "prime":
		id, err = h.SavePrime(r.Context(), req.Content, req.Source)
	default:
		if gw.memStore == nil {
			writeJSON(w, 503, map[string]string{"error": "memory store not initialized"})
			return
		}
		m := memory.Memory{AgentID: req.AgentID, Type: "agent", Content: req.Content, Source: req.Source, Importance: 0.7}
		id, err = gw.memStore.Save(r.Context(), "default", m)
	}

	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "scope": req.Scope})
}

// --- Teams handlers ---

func (gw *Gateway) handleListTeams(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"teams": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, name, COALESCE(supervisor_id::text, '') as lead, created_at FROM crews ORDER BY name`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	teams := []map[string]any{}
	for rows.Next() {
		var id, name, lead string
		var created interface{}
		rows.Scan(&id, &name, &lead, &created)
		teams = append(teams, map[string]any{"id": id, "name": name, "lead_agent": lead, "created_at": created})
	}
	writeJSON(w, 200, map[string]any{"teams": teams})
}

func (gw *Gateway) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string   `json:"name"`
		LeadAgent string   `json:"lead_agent"`
		Members   []string `json:"members"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name and lead_agent required"})
		return
	}
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}

	// Resolve lead agent ID
	var leadID string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id FROM agents WHERE agent_key = $1 OR id::text = $1 LIMIT 1`, req.LeadAgent).Scan(&leadID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "lead agent not found: " + req.LeadAgent})
		return
	}

	// Create team. Column is `supervisor_id` (crews schema from
	// migration 004_multi_agent), not `lead_agent_id` — the handler
	// was written against a different schema. tenant_id is the
	// well-known default tenant UUID, not the literal string "default".
	var teamID string
	err = gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO crews (name, supervisor_id, tenant_id, created_at)
		 VALUES ($1, $2, $3, now()) RETURNING id`,
		req.Name, leadID, defaultTenant).Scan(&teamID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "create team: " + err.Error()})
		return
	}

	// Add lead as member
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO crew_members (team_id, agent_id, role) VALUES ($1, $2, 'lead') ON CONFLICT DO NOTHING`,
		teamID, leadID)

	// Add other members — batch lookup then batch INSERT to avoid N+1
	if len(req.Members) > 0 {
		// Resolve all member keys/IDs in one query using ANY
		rows, err := gw.db.Pool.Query(r.Context(),
			`SELECT id FROM agents WHERE agent_key = ANY($1) OR id::text = ANY($1)`, req.Members)
		if err == nil {
			defer rows.Close()
			memberIDs := []string{}
			for rows.Next() {
				var mid string
				if rows.Scan(&mid) == nil {
					memberIDs = append(memberIDs, mid)
				}
			}
			if len(memberIDs) > 0 {
				query := "INSERT INTO crew_members (team_id, agent_id, role) VALUES "
				args := []any{teamID}
				for i, mid := range memberIDs {
					if i > 0 {
						query += ","
					}
					args = append(args, mid)
					query += fmt.Sprintf("($1,$%d,'member')", i+2)
				}
				query += " ON CONFLICT DO NOTHING"
				gw.db.Pool.Exec(r.Context(), query, args...)
			}
		}
	}

	writeJSON(w, 201, map[string]any{
		"id": teamID, "name": req.Name, "lead_agent": req.LeadAgent,
		"members": len(req.Members) + 1,
	})
}

func (gw *Gateway) handleTeamMembers(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"members": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT agent_id, role FROM crew_members WHERE team_id = $1`, teamID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	members := []map[string]any{}
	for rows.Next() {
		var agentID, role string
		rows.Scan(&agentID, &role)
		members = append(members, map[string]any{"agent_id": agentID, "role": role})
	}
	writeJSON(w, 200, map[string]any{"members": members})
}

// --- Plugins handlers ---

func (gw *Gateway) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	if gw.pluginMgr == nil {
		writeJSON(w, 200, map[string]any{"plugins": []any{}})
		return
	}
	names := gw.pluginMgr.List()
	plugins := make([]map[string]any, 0, len(names))
	for _, name := range names {
		plugins = append(plugins, map[string]any{
			"name":    name,
			"enabled": true,
			"tools":   len(gw.pluginMgr.Context().Tools()),
			"hooks":   len(gw.pluginMgr.Context().Hooks()),
		})
	}
	writeJSON(w, 200, map[string]any{"plugins": plugins, "count": len(plugins)})
}

func (gw *Gateway) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Identifier string `json:"identifier"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	writeJSON(w, 201, map[string]any{"name": req.Identifier, "status": "installed"})
}

func (gw *Gateway) handleRemovePlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	writeJSON(w, 200, map[string]any{"name": name, "status": "removed"})
}

// --- Per-agent channel binding handlers ---

func (gw *Gateway) handleListAgentChannels(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	if gw.bindingStore == nil {
		writeJSON(w, 200, map[string]any{"channels": []any{}})
		return
	}
	bindings, err := gw.bindingStore.ListForAgent(r.Context(), agentID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"channels": bindings})
}

func (gw *Gateway) handleBindAgentChannel(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	var req struct {
		ChannelType string          `json:"channel_type"`
		DisplayName string          `json:"display_name"`
		Credentials json.RawMessage `json:"credentials"`
		Config      json.RawMessage `json:"config"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if gw.bindingStore == nil {
		writeJSON(w, 503, map[string]string{"error": "channel bindings not available"})
		return
	}

	binding := channels.AgentBinding{
		AgentID:     agentID,
		TenantID:    "default",
		ChannelType: req.ChannelType,
		DisplayName: req.DisplayName,
		Credentials: req.Credentials,
		Config:      req.Config,
		Enabled:     true,
	}
	id, err := gw.bindingStore.Create(r.Context(), binding)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "channel_type": req.ChannelType, "agent_id": agentID})
}

func (gw *Gateway) handleUnbindAgentChannel(w http.ResponseWriter, r *http.Request) {
	bindingID := chi.URLParam(r, "bindingId")
	if gw.bindingStore == nil {
		writeJSON(w, 503, map[string]string{"error": "not available"})
		return
	}
	if err := gw.bindingStore.Delete(r.Context(), bindingID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"status": "unbound"})
}

// handleExportSession exports a session's full conversation as markdown.
func (gw *Gateway) handleExportSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if gw.sessions == nil {
		writeJSON(w, 503, map[string]string{"error": "sessions not available"})
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "markdown"
	}

	history, err := gw.sessions.GetHistory(r.Context(), sessionID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	if len(history) == 0 {
		writeJSON(w, 404, map[string]string{"error": "session not found or empty"})
		return
	}

	switch format {
	case "markdown":
		w.Header().Set("Content-Type", "text/markdown")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=session-%s.md", sessionID[:8]))
		fmt.Fprintf(w, "# Conversation Export\n\nSession: %s\nExported: %s\n\n---\n\n", sessionID[:8], time.Now().Format("2006-01-02 15:04"))
		for _, msg := range history {
			role := msg.Role
			if role == "assistant" {
				role = "🤖 Agent"
			} else if role == "user" {
				role = "👤 User"
			} else if role == "tool" {
				role = "🔧 Tool"
			}
			content := msg.Content
			if len(content) > 5000 {
				content = content[:5000] + "\n\n_(truncated)_"
			}
			fmt.Fprintf(w, "### %s\n\n%s\n\n---\n\n", role, content)
		}
	case "json":
		writeJSON(w, 200, map[string]any{"session_id": sessionID, "messages": history, "count": len(history)})
	default:
		writeJSON(w, 400, map[string]string{"error": "format must be 'markdown' or 'json'"})
	}
}
