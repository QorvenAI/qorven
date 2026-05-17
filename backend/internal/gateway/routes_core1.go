// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (gw *Gateway) RegisterCore1Routes(r chi.Router) {
	r.Get("/memory/scopes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"scopes": []string{"company", "team", "agent", "task", "session", "prime"}})
	})
	r.Post("/memory/search", gw.handleMemorySearch)
	r.Post("/memory/save", gw.handleMemorySave)
	r.Get("/sessions/{id}/export", gw.handleExportSession)
	r.Get("/teams", gw.handleListTeams)
	r.Post("/teams", gw.handleCreateTeam)
	r.Get("/teams/{id}/members", gw.handleTeamMembers)
	r.Get("/plugins", gw.handleListPlugins)
	r.Post("/plugins/install", gw.handleInstallPlugin)
	r.Delete("/plugins/{name}", gw.handleRemovePlugin)
}
