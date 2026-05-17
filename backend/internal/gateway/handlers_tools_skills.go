// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/tools"
)

func (gw *Gateway) handleListBuiltinTools(w http.ResponseWriter, r *http.Request) {
	type toolInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := []toolInfo{}
	for _, name := range gw.toolReg.List() {
		if t, ok := gw.toolReg.Get(name); ok {
			out = append(out, toolInfo{Name: t.Name(), Description: t.Description()})
		}
	}
	writeJSON(w, 200, map[string]any{"tools": out, "count": len(out)})
}

func (gw *Gateway) handleListCustomTools(w http.ResponseWriter, r *http.Request) {
	if gw.customTools == nil {
		writeJSON(w, 200, map[string]any{"tools": []any{}})
		return
	}
	list, err := gw.customTools.List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []tools.CustomToolDef{}
	}
	writeJSON(w, 200, map[string]any{"tools": list})
}

func (gw *Gateway) handleCreateCustomTool(w http.ResponseWriter, r *http.Request) {
	if gw.customTools == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var def tools.CustomToolDef
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&def); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if def.Name == "" || def.Command == "" {
		writeJSON(w, 400, map[string]string{"error": "name and command required"})
		return
	}
	id, err := gw.customTools.Create(r.Context(), defaultTenant, def)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// Register in live registry
	def.ID = id
	def.Enabled = true
	gw.toolReg.Register(tools.NewDynamicTool(def, "/tmp/qorven-workspace"))
	slog.Info("custom tool created", "name", def.Name)
	writeJSON(w, 201, map[string]string{"id": id, "name": def.Name})
}

func (gw *Gateway) handleDeleteCustomTool(w http.ResponseWriter, r *http.Request) {
	if gw.customTools == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	gw.customTools.Delete(r.Context(), defaultTenant, id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleListSkills(w http.ResponseWriter, r *http.Request) {
	// Combine filesystem skills + DB skills
	allSkills := []map[string]any{}

	if gw.skillLoader != nil {
		for _, s := range gw.skillLoader.ListSkills() {
			allSkills = append(allSkills, map[string]any{
				"name": s.Name, "slug": s.Slug, "description": s.Description,
				"source": s.Source, "path": s.Path,
			})
		}
	}
	if allSkills == nil {
		allSkills = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"skills": allSkills, "count": len(allSkills)})
}

func (gw *Gateway) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	gw.skillStore.Delete(r.Context(), defaultTenant, id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	servers := gw.mcpClient.ListServers()
	if servers == nil {
		servers = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"servers": servers})
}

func (gw *Gateway) handleGetMCPServer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	for _, s := range gw.mcpClient.ListServers() {
		if s["name"] == name {
			writeJSON(w, 200, s)
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "server not found"})
}

func (gw *Gateway) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	for _, s := range gw.mcpClient.ListServers() {
		if s["name"] == name {
			connected, _ := s["connected"].(bool)
			if connected {
				writeJSON(w, 200, map[string]string{"status": "ok"})
			} else {
				writeJSON(w, 503, map[string]string{"status": "disconnected"})
			}
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "server not found"})
}

func (gw *Gateway) handleGetMCPServerTools(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	matched := []map[string]any{}
	for _, t := range gw.mcpClient.GetAllTools() {
		if t.ServerName == name {
			matched = append(matched, map[string]any{"name": t.Name, "description": t.Description})
		}
	}
	if matched == nil {
		matched = []map[string]any{}
	}
	writeJSON(w, 200, matched)
}

// handlePatchSkill handles PATCH /v1/skills/:id — admin-only pin/unpin.
func (gw *Gateway) handlePatchSkill(w http.ResponseWriter, r *http.Request) {
	if gw.skillStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Pinned *bool `json:"pinned"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if body.Pinned == nil {
		writeJSON(w, 400, map[string]string{"error": "pinned field required"})
		return
	}
	if err := gw.skillStore.SetPinned(r.Context(), defaultTenant, id, *body.Pinned); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("skill.pin_changed", "id", id, "pinned", *body.Pinned)
	writeJSON(w, 200, map[string]any{"id": id, "pinned": *body.Pinned})
}
