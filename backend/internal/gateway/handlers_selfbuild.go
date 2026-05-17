// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/audit"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/templates"
)

func (gw *Gateway) handleSelfBuild(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description string `json:"description"`
		TemplateID  string `json:"template_id,omitempty"` // optional: force a specific template
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Description == "" {
		writeJSON(w, 400, map[string]string{"error": "description required"})
		return
	}
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}

	inst := templates.NewInstaller(gw.db.Pool)

	// Step 1: Find the best matching template
	templateID := body.TemplateID
	if templateID == "" {
		templateID = matchTemplate(body.Description)
	}

	var installed *templates.InstalledWorkspace
	var matchedTemplate *templates.WorkspaceTemplate

	if templateID != "" {
		// Install the matched template directly
		result, err := inst.Install(r.Context(), defaultTenant, templateID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "template install failed: " + err.Error()})
			return
		}
		installed = result
		for _, t := range templates.Catalog() {
			if t.ID == templateID {
				copy := t
				matchedTemplate = &copy
				break
			}
		}
	} else if gw.agentLoop != nil {
		// Step 2: No matching template — use AI to generate a custom workspace
		chief, _ := gw.agents.GetByKey(r.Context(), "chief")
		chiefID := ""
		if chief != nil {
			chiefID = chief.ID
		}

		aiPrompt := `You are a workspace architect. Build an AI workspace based on: "` + body.Description + `"

Available agent roles: leader, specialist, researcher, developer, writer, analyst, support
Available dashboard block types: stat-row, pipeline, data-table, chart, contacts, feed, kanban, calendar, timeline
Available layouts: grid-2col, grid-3col, sidebar-right

Respond with ONLY valid JSON (no markdown, no explanation):
{"template_id":"custom","name":"<workspace name>","agents":[{"key":"<key>","name":"<name>","role":"<role>","reports_to":"<parent-key or empty>","system_prompt":"<50-100 word prompt>","tools":["web_search"]}],"dashboard":{"layout":"grid-2col","blocks":[{"type":"stat-row"},{"type":"feed","title":"Activity"}]},"connectors":[],"summary":"<one sentence>"}`

		aiResult, err := gw.agentLoop.Chat(r.Context(), chiefID, aiPrompt)
		if err == nil && aiResult != "" {
			// Parse AI response as a template and install it
			var customTmpl templates.WorkspaceTemplate
			// Extract JSON from AI response
			start := strings.Index(aiResult, "{")
			end := strings.LastIndex(aiResult, "}")
			if start >= 0 && end > start {
				jsonStr := aiResult[start : end+1]
				if json.Unmarshal([]byte(jsonStr), &customTmpl) == nil && len(customTmpl.Agents) > 0 {
					customTmpl.ID = "custom-" + fmt.Sprintf("%d", time.Now().Unix())
					result, err := inst.InstallCustom(r.Context(), defaultTenant, &customTmpl)
					if err == nil {
						installed = result
						matchedTemplate = &customTmpl
					}
				}
			}
		}

		// Final fallback: install the general-purpose template
		if installed == nil {
			result, _ := inst.Install(r.Context(), defaultTenant, "research")
			installed = result
		}
	} else {
		// No AI available — install research template as default
		result, err := inst.Install(r.Context(), defaultTenant, "research")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		installed = result
	}

	// Audit log
	if gw.auditStore != nil {
		gw.auditStore.Log(r.Context(), defaultTenant, audit.ActorUser, "api", "", "self-build", "workspace", "",
			map[string]string{"description": body.Description[:min(len(body.Description), 200)], "template": templateID}, r.RemoteAddr)
	}

	// Build response
	resp := map[string]any{
		"status":        "installed",
		"template_id":   installed.TemplateID,
		"name":          installed.Name,
		"agents":        installed.AgentIDs,
		"agent_count":   installed.AgentCount,
		"dashboard_url": "/dashboard/" + installed.TemplateID,
	}
	if matchedTemplate != nil {
		resp["description"] = matchedTemplate.Description
		resp["icon"] = matchedTemplate.Icon
		resp["connectors"] = matchedTemplate.Connectors
	}
	writeJSON(w, 200, resp)
}

func matchTemplate(description string) string {
	lower := strings.ToLower(description)
	scores := map[string]int{}
	keywords := map[string][]string{
		"crm":       {"crm", "sales", "leads", "pipeline", "prospects", "deals", "customers", "outreach", "revenue"},
		"social":    {"social", "twitter", "linkedin", "instagram", "content", "posts", "marketing", "brand", "audience"},
		"research":  {"research", "analysis", "data", "report", "study", "information", "sources", "academic", "knowledge"},
		"trading":   {"trading", "crypto", "stocks", "portfolio", "market", "finance", "investment", "bitcoin", "prices"},
		"invoicing": {"invoice", "billing", "payment", "accounting", "finance", "receipts", "expenses", "budget"},
		"devops":    {"devops", "engineering", "ci/cd", "deployment", "monitoring", "infrastructure", "servers", "code"},
	}
	for tmplID, kws := range keywords {
		for _, kw := range kws {
			if strings.Contains(lower, kw) {
				scores[tmplID]++
			}
		}
	}
	best, bestScore := "", 0
	for id, score := range scores {
		if score > bestScore {
			best, bestScore = id, score
		}
	}
	if bestScore >= 1 {
		return best
	}
	return "" // no confident match
}

func (gw *Gateway) handleExportTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	catalog := templates.Catalog()
	for _, t := range catalog {
		if t.ID == id {
			t.Version = "1.0.0"
			t.Author = "Qorven"
			writeJSON(w, 200, t)
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "template not found"})
}

func (gw *Gateway) handleImportTemplate(w http.ResponseWriter, r *http.Request) {
	var t templates.WorkspaceTemplate
	if json.NewDecoder(r.Body).Decode(&t) != nil || t.ID == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid template JSON"})
		return
	}
	// Validate
	if len(t.Agents) == 0 {
		writeJSON(w, 400, map[string]string{"error": "template must have at least one agent"})
		return
	}
	if gw.auditStore != nil {
		gw.auditStore.Log(r.Context(), defaultTenant, audit.ActorUser, "api", "", "import", "template", t.ID, map[string]string{"name": t.Name, "version": t.Version}, r.RemoteAddr)
	}
	writeJSON(w, 200, map[string]any{"status": "imported", "template": t})
}

func (gw *Gateway) handleProbeModels(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIBase string `json:"api_base"`
		APIKey  string `json:"api_key"`
		Type    string `json:"provider_type"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.APIBase == "" {
		http.Error(w, `{"error":"api_base required"}`, 400)
		return
	}
	models, err := providers.FetchModelsLive(r.Context(), req.Type, req.APIBase, req.APIKey)
	if err != nil {
		writeJSON(w, 200, map[string]any{"models": []any{}, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"models": models})
}
