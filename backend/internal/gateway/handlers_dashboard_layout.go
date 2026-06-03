// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// dashboardLayout mirrors the user_dashboard_layouts row.
type dashboardLayout struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Layout    json.RawMessage `json:"layout"`  // LayoutItem[] from react-grid-layout
	Widgets   json.RawMessage `json:"widgets"` // {[id]: WidgetConfig}
	IsDefault bool            `json:"is_default"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// widgetConfig mirrors the frontend WidgetConfig type.
// The backend treats it as opaque JSON — validation is done on the frontend.
type widgetConfig = json.RawMessage

// ── Handlers ─────────────────────────────────────────────────────────────────

// handleGetDashboardLayout returns the user's default dashboard layout.
// GET /v1/dashboard/layout
func (gw *Gateway) handleGetDashboardLayout(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if userID == "" {
		userID = defaultTenant // fallback for single-user mode
	}

	var dl dashboardLayout
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, name, layout, widgets, is_default, updated_at
		 FROM user_dashboard_layouts
		 WHERE user_id = $1 AND is_default = true
		 ORDER BY updated_at DESC LIMIT 1`,
		userID,
	).Scan(&dl.ID, &dl.Name, &dl.Layout, &dl.Widgets, &dl.IsDefault, &dl.UpdatedAt)

	if err != nil {
		// No layout saved yet — return empty defaults
		writeJSON(w, http.StatusOK, map[string]any{
			"id":         "",
			"name":       "My Dashboard",
			"layout":     json.RawMessage("[]"),
			"widgets":    json.RawMessage("{}"),
			"is_default": true,
		})
		return
	}
	writeJSON(w, http.StatusOK, dl)
}

// handleSaveDashboardLayout upserts the user's default dashboard layout.
// PUT /v1/dashboard/layout
func (gw *Gateway) handleSaveDashboardLayout(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if userID == "" {
		userID = defaultTenant
	}

	var body struct {
		Name    string          `json:"name"`
		Layout  json.RawMessage `json:"layout"`
		Widgets json.RawMessage `json:"widgets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Name == "" {
		body.Name = "My Dashboard"
	}
	if body.Layout == nil {
		body.Layout = json.RawMessage("[]")
	}
	if body.Widgets == nil {
		body.Widgets = json.RawMessage("{}")
	}

	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO user_dashboard_layouts (user_id, tenant_id, name, layout, widgets, is_default, updated_at)
		 VALUES ($1, $2, $3, $4, $5, true, now())
		 ON CONFLICT (user_id, name) DO UPDATE
		   SET layout = $4, widgets = $5, updated_at = now()
		 RETURNING id`,
		userID, defaultTenant, body.Name, body.Layout, body.Widgets,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "status": "saved"})
}

// handleListDashboards returns all dashboard layouts for the user.
// GET /v1/dashboard/layouts
func (gw *Gateway) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if userID == "" {
		userID = defaultTenant
	}

	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, name, is_default, updated_at
		 FROM user_dashboard_layouts WHERE user_id = $1
		 ORDER BY is_default DESC, updated_at DESC`,
		userID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	type listItem struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		IsDefault bool      `json:"is_default"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	result := []listItem{}
	for rows.Next() {
		var item listItem
		if err := rows.Scan(&item.ID, &item.Name, &item.IsDefault, &item.UpdatedAt); err != nil {
			continue
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCreateDashboard creates a new named dashboard for the user.
// POST /v1/dashboard/layouts
func (gw *Gateway) handleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if userID == "" {
		userID = defaultTenant
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}

	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO user_dashboard_layouts (user_id, tenant_id, name, layout, widgets, is_default)
		 VALUES ($1, $2, $3, '[]', '{}', false)
		 RETURNING id`,
		userID, defaultTenant, strings.TrimSpace(body.Name),
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "name": strings.TrimSpace(body.Name)})
}

// handleSetDefaultDashboard sets a dashboard as the default for the user.
// PUT /v1/dashboard/layouts/:id/default
func (gw *Gateway) handleSetDefaultDashboard(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	u := userFromContext(r.Context())
	userID := ""
	if u != nil {
		userID = u.ID
	}
	if userID == "" {
		userID = defaultTenant
	}

	dashID := r.PathValue("id")
	if dashID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	// Clear all existing defaults for this user, then set the new one
	_, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE user_dashboard_layouts SET is_default = (id = $1) WHERE user_id = $2`,
		dashID, userID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGenerateWidget uses the agent loop to generate a widget config from a natural language prompt.
// POST /v1/dashboard/generate-widget
// Body: {"prompt": "show me agent error rate by hour as a bar chart"}
// Returns: WidgetConfig JSON
func (gw *Gateway) handleGenerateWidget(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt required"})
		return
	}

	// Available data sources — injected into the system prompt
	availableSources := `agent_status_live, spend_total_today, agent_runs_per_hour, session_count_today,
pending_approvals, spend_by_provider_30d, channel_message_volume, task_completion_rate, error_rate_by_agent`

	systemPrompt := fmt.Sprintf(`You are a dashboard widget designer for the Qorven AI agent platform.
Generate a single dashboard widget configuration as JSON matching this TypeScript type:

type WidgetConfig = {
  id: string;          // generate a random uuid-style string
  title: string;       // clear, concise widget title
  type: "metric" | "line" | "area" | "bar" | "donut" | "activity" | "agents" | "tasks" | "heatmap" | "progress";
  dataSource: string;  // MUST be one of the available sources listed below
  grid: { w: number; h: number }; // w: 2-12, h: 2-8 in grid units
  config?: {
    xKey?: string;
    yKey?: string;
    color?: string;          // hex color
    prefix?: string;         // e.g. "$" for spend
    suffix?: string;         // e.g. "%%" for rates
    aggregation?: "sum" | "avg" | "count" | "last";
    timeRange?: "1h" | "24h" | "7d" | "30d";
    showTrend?: boolean;
  };
};

Available data sources: %s

Rules:
- Return ONLY valid JSON matching the type above, nothing else
- Pick the most appropriate chart type for the requested data
- Set grid size appropriately: metrics=w3h2, charts=w6h4, lists=w4h5
- Always set a clear descriptive title`, availableSources)

	if gw.agentLoop == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "agent loop not available"})
		return
	}

	// Use the agent loop for a single non-streaming call
	req := agent.RunRequest{
		UserMessage:       body.Prompt,
		ExtraSystemPrompt: systemPrompt,
		NoTools:           true,
		Stream:            false,
	}
	result, err := gw.agentLoop.Run(r.Context(), req, func(_ agent.StreamEvent) {})
	if err != nil || result == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate widget"})
		return
	}
	content := result.Content

	// Try to find JSON object in the response
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start == -1 || end == -1 || end <= start {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "model did not return valid widget JSON"})
		return
	}
	jsonStr := content[start : end+1]

	// Validate it's parseable JSON
	var widgetCfg map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &widgetCfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "model returned invalid JSON"})
		return
	}

	// Ensure required fields exist
	if widgetCfg["type"] == nil {
		widgetCfg["type"] = "metric"
	}
	if widgetCfg["id"] == nil {
		widgetCfg["id"] = fmt.Sprintf("widget-%d", time.Now().UnixNano())
	}
	if widgetCfg["grid"] == nil {
		widgetCfg["grid"] = map[string]any{"w": 4, "h": 3}
	}

	writeJSON(w, http.StatusOK, widgetCfg)
}

