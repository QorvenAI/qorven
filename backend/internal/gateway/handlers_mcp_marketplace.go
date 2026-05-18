// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/mcp"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/training"
)

func (gw *Gateway) handleConnectMCPServer(w http.ResponseWriter, r *http.Request) {
	var cfg mcp.ServerConfig
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cfg)
	if cfg.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name required"})
		return
	}
	if cfg.Transport == "" {
		cfg.Transport = "stdio"
	}
	cfg.Enabled = true

	tools, err := gw.mcpClient.ConnectAny(r.Context(), cfg)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	// Auto-register discovered tools
	count := mcp.RegisterDiscoveredTools(gw.mcpClient, gw.toolReg)

	writeJSON(w, 200, map[string]any{
		"status": "connected", "server": cfg.Name,
		"tools_discovered": len(tools), "tools_registered": count,
	})
}

func (gw *Gateway) handleDisconnectMCPServer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	gw.mcpClient.Disconnect(name)
	writeJSON(w, 200, map[string]string{"status": "disconnected", "server": name})
}

func (gw *Gateway) handleListMCPTools(w http.ResponseWriter, r *http.Request) {
	tools := gw.mcpClient.GetAllTools()
	if tools == nil {
		tools = []mcp.DiscoveredTool{}
	}
	writeJSON(w, 200, map[string]any{"tools": tools, "count": len(tools)})
}

func (gw *Gateway) handleMarketplaceSkills(w http.ResponseWriter, r *http.Request) {
	allSkills := []map[string]any{}

	if gw.skillStore != nil {
		category := r.URL.Query().Get("category")
		search := r.URL.Query().Get("search")
		skills, _ := gw.skillStore.ListMarketplace(r.Context(), category, search)
		for _, s := range skills {
			allSkills = append(allSkills, map[string]any{
				"id": s.ID, "name": s.Name, "slug": s.Slug, "description": s.Description,
				"category": s.Category, "author": s.Author, "type": "crystallized",
			})
		}
	}

	if gw.db != nil {
		rows, err := gw.db.Pool.Query(r.Context(),
			`SELECT id, name, COALESCE(description,''), COALESCE(repo_url,''), COALESCE(license,''),
			        type, array_to_string(COALESCE(tags, ARRAY[]::TEXT[]), ','), verified, COALESCE(stars,0)
			 FROM skill_manifests ORDER BY verified DESC, name`)
		if err != nil {
			slog.Warn("skill_manifests query", "error", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var id, name, desc, repo, lic, stype, tagsStr string
				var verified bool
				var stars int
				if err := rows.Scan(&id, &name, &desc, &repo, &lic, &stype, &tagsStr, &verified, &stars); err != nil {
					slog.Warn("skill_manifests scan", "error", err)
					continue
				}
				allSkills = append(allSkills, map[string]any{
					"id": id, "name": name, "slug": id, "description": desc,
					"category": "plugin", "author": repo, "type": stype,
					"tags": strings.Split(tagsStr, ","), "verified": verified, "stars": stars,
					"repo_url": repo, "license": lic,
				})
			}
			slog.Info("skill_manifests loaded", "count", len(allSkills))
		}
	}

	writeJSON(w, 200, map[string]any{"skills": allSkills, "count": len(allSkills)})
}

func (gw *Gateway) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 404, map[string]string{"error": "skill not found"})
		return
	}
	slug := chi.URLParam(r, "slug")
	skill, err := gw.skillStore.GetBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "skill not found"})
		return
	}
	writeJSON(w, 200, skill)
}

func (gw *Gateway) handlePublishSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 503, map[string]string{"error": "skill store not available"})
		return
	}
	var d skills.SkillDetail
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if d.Slug == "" || d.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name and slug required"})
		return
	}
	id, err := gw.skillStore.Publish(r.Context(), d)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "slug": d.Slug})
}

func (gw *Gateway) handleInstallSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 503, map[string]string{"error": "skill store not available"})
		return
	}
	slug := chi.URLParam(r, "slug")
	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.AgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}
	if err := gw.skillStore.Install(r.Context(), body.AgentID, slug); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "installed", "slug": slug, "agent_id": body.AgentID})
}

func (gw *Gateway) handleUninstallSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 200, map[string]string{"status": "uninstalled"})
		return
	}
	slug := chi.URLParam(r, "slug")
	var body struct {
		AgentID string `json:"agent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.AgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}
	gw.skillStore.Uninstall(r.Context(), body.AgentID, slug)
	writeJSON(w, 200, map[string]string{"status": "uninstalled", "slug": slug})
}

func (gw *Gateway) handleAgentSkills(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 200, map[string]any{"skills": []any{}, "count": 0})
		return
	}
	agentID := chi.URLParam(r, "id")
	installed, err := gw.skillStore.AgentSkills(r.Context(), agentID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"skills": installed, "count": len(installed)})
}

func (gw *Gateway) handleRateSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 200, map[string]string{"status": "rated"})
		return
	}
	slug := chi.URLParam(r, "slug")
	var body struct {
		Rating int    `json:"rating"`
		Review string `json:"review"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Rating < 1 || body.Rating > 5 {
		writeJSON(w, 400, map[string]string{"error": "rating must be 1-5"})
		return
	}
	gw.skillStore.Rate(r.Context(), slug, body.Rating, body.Review)
	writeJSON(w, 200, map[string]string{"status": "rated"})
}

func (gw *Gateway) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	if gw.notifStore == nil {
		writeJSON(w, 200, map[string]any{"notifications": []any{}, "unread": 0})
		return
	}
	notifs, unread, err := gw.notifStore.List(r.Context(), 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"notifications": notifs, "unread": unread})
}

func (gw *Gateway) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	if gw.notifStore == nil {
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	gw.notifStore.MarkRead(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (gw *Gateway) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	if gw.notifStore == nil {
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	gw.notifStore.MarkAllRead(r.Context())
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (gw *Gateway) writeNotification(agentID, agentKey, agentName, nType, title, highlight, source, sourceID string) {
	if gw.notifStore == nil {
		return
	}
	id, _ := gw.notifStore.Create(context.Background(), agentID, agentKey, agentName, nType, title, highlight, source, sourceID)
	// WebSocket is just a doorbell — tells UI to fetch from DB
	gw.rtHub.Broadcast(realtime.Event{Type: "notification", Data: map[string]string{
		"id": id, "agent_key": agentKey, "agent_name": agentName, "highlight": highlight, "source": source,
	}})
}

func (gw *Gateway) handleTrainingExport(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	format := r.URL.Query().Get("format") // "jsonl" (default), "preferences", "corrections"

	exp := training.NewExporter(gw.db.Pool)

	switch format {
	case "preferences":
		pairs, err := exp.ExportPreferences(r.Context(), agentID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pairs)
	case "corrections":
		data, err := exp.ExportCorrections(r.Context(), agentID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/jsonl")
		w.Write(data)
	default:
		data, err := exp.ExportJSONL(r.Context(), agentID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/jsonl")
		w.Write(data)
	}
}

func (gw *Gateway) handleRemoveRoomMember(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	roomID := chi.URLParam(r, "id")
	agentID := chi.URLParam(r, "agent_id")
	gw.db.Pool.Exec(r.Context(), `DELETE FROM room_members WHERE room_id = $1 AND agent_id = $2`, roomID, agentID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleListCrystallizedSkills(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, name, slug, description, scope, mode, reuse_count, success_rate, created_at
		 FROM crystallized_skills WHERE agent_id = $1 OR scope IN ('shared','marketplace')
		 ORDER BY reuse_count DESC`, agentID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	skills := []map[string]any{}
	for rows.Next() {
		var id, name, slug, desc, scope, mode string
		var reuse int
		var rate float64
		var created time.Time
		rows.Scan(&id, &name, &slug, &desc, &scope, &mode, &reuse, &rate, &created)
		skills = append(skills, map[string]any{
			"id": id, "name": name, "slug": slug, "description": desc,
			"scope": scope, "mode": mode, "reuse_count": reuse,
			"success_rate": rate, "created_at": created,
			"promote_ready": reuse >= 3 && scope == "private",
		})
	}
	json.NewEncoder(w).Encode(skills)
}

func (gw *Gateway) handlePromoteSkill(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Scope string `json:"scope"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Scope != "shared" && body.Scope != "marketplace" {
		http.Error(w, `{"error":"scope must be 'shared' or 'marketplace'"}`, 400)
		return
	}
	gw.db.Pool.Exec(r.Context(), `UPDATE crystallized_skills SET scope = $1 WHERE id = $2`, body.Scope, id)
	json.NewEncoder(w).Encode(map[string]string{"status": "promoted", "scope": body.Scope})
}

func (gw *Gateway) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	if gw.connReg == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	json.NewEncoder(w).Encode(gw.connReg.List())
}

func (gw *Gateway) handleTestConnector(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Credentials map[string]string `json:"credentials"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	conn, ok := gw.connReg.Get(id)
	if !ok {
		http.Error(w, `{"error":"connector not found"}`, 404)
		return
	}
	err := conn.TestConnection(r.Context(), body.Credentials)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (gw *Gateway) handleExecuteConnector(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Action      string            `json:"action"`
		Credentials map[string]string `json:"credentials"`
		Params      map[string]any    `json:"params"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	result, err := gw.connReg.Execute(r.Context(), id, body.Action, body.Credentials, body.Params)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"result": result})
}
