// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/realtime"
)

// ─── Room Decisions ───────────────────────────────────────────────────────────

func (gw *Gateway) handleGetRoomDecisions(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"decisions": []any{}})
		return
	}
	roomID := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, content, decided_by, status, created_at
		 FROM room_decisions WHERE room_id = $1 AND status = 'active'
		 ORDER BY created_at DESC LIMIT 50`, roomID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"decisions": []any{}})
		return
	}
	defer rows.Close()
	decisions := []map[string]any{}
	for rows.Next() {
		var id, content, decidedBy, status string
		var createdAt interface{}
		rows.Scan(&id, &content, &decidedBy, &status, &createdAt)
		decisions = append(decisions, map[string]any{
			"id": id, "content": content, "decided_by": decidedBy,
			"status": status, "created_at": createdAt,
		})
	}
	if decisions == nil {
		decisions = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"decisions": decisions})
}

func (gw *Gateway) handlePostRoomDecision(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	roomID := chi.URLParam(r, "id")
	var body struct {
		Content   string `json:"content"`
		DecidedBy string `json:"decided_by"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Content == "" {
		writeJSON(w, 400, map[string]string{"error": "content required"})
		return
	}
	if body.DecidedBy == "" {
		body.DecidedBy = "system"
	}

	var id string
	gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO room_decisions (room_id, tenant_id, content, decided_by) VALUES ($1, $2, $3, $4) RETURNING id`,
		roomID, defaultTenant, body.Content, body.DecidedBy).Scan(&id)

	// Also post as a pinned message in the room
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO room_messages (room_id, sender_id, sender_type, content, message_type)
		 VALUES ($1, $2, 'soul', $3, 'decision')`,
		roomID, body.DecidedBy, "📌 DECISION: "+body.Content)

	writeJSON(w, 201, map[string]any{"id": id, "status": "pinned"})
}

// ─── Room Minutes ─────────────────────────────────────────────────────────────

func (gw *Gateway) handleGetRoomMinutes(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"minutes": []any{}})
		return
	}
	roomID := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, summary, decisions, action_items, blockers, msg_count, generated_at
		 FROM room_minutes WHERE room_id = $1 ORDER BY generated_at DESC LIMIT 10`, roomID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"minutes": []any{}})
		return
	}
	defer rows.Close()
	minutes := []map[string]any{}
	for rows.Next() {
		var id, summary string
		var decisions, actionItems, blockers []byte
		var msgCount int
		var generatedAt interface{}
		rows.Scan(&id, &summary, &decisions, &actionItems, &blockers, &msgCount, &generatedAt)
		minutes = append(minutes, map[string]any{
			"id": id, "summary": summary,
			"decisions":    json.RawMessage(decisions),
			"action_items": json.RawMessage(actionItems),
			"blockers":     json.RawMessage(blockers),
			"msg_count":    msgCount,
			"generated_at": generatedAt,
		})
	}
	if minutes == nil {
		minutes = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"minutes": minutes})
}

// handleGenerateRoomMinutes uses AI to summarize recent room conversation.
func (gw *Gateway) handleGenerateRoomMinutes(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.agentLoop == nil {
		writeJSON(w, 503, map[string]string{"error": "not available"})
		return
	}
	roomID := chi.URLParam(r, "id")

	// Load recent messages
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT sender_id, sender_type, content, created_at FROM room_messages
		 WHERE room_id = $1 ORDER BY created_at DESC LIMIT 100`, roomID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type msgRow struct{ SenderID, SenderType, Content string; CreatedAt time.Time }
	msgs := []msgRow{}
	for rows.Next() {
		var m msgRow
		rows.Scan(&m.SenderID, &m.SenderType, &m.Content, &m.CreatedAt)
		msgs = append(msgs, m)
	}
	if len(msgs) == 0 {
		writeJSON(w, 200, map[string]string{"status": "no messages to summarize"})
		return
	}

	// Build transcript (reverse to chronological)
	var transcript strings.Builder
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		transcript.WriteString(fmt.Sprintf("[%s] %s: %s\n",
			m.CreatedAt.Format("15:04"), m.SenderID, m.Content))
	}

	prompt := `Analyze this room conversation and extract structured meeting minutes.

Conversation:
` + transcript.String() + `

Respond with JSON only:
{
  "summary": "2-3 sentence narrative of what was discussed",
  "decisions": [{"text": "...", "decided_by": "agent_key"}],
  "action_items": [{"task": "...", "assigned_to": "agent_key", "due": "...or null"}],
  "blockers": [{"issue": "...", "raised_by": "agent_key"}]
}`

	// Use provider directly (cheaper than full agent loop)
	provider := gw.providerReg.Default()
	if provider == nil {
		writeJSON(w, 503, map[string]string{"error": "no provider"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0, "max_tokens": 1000},
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Parse and save
	content := strings.TrimSpace(resp.Content)
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		writeJSON(w, 500, map[string]string{"error": "invalid AI response"})
		return
	}
	jsonStr := content[start : end+1]

	var parsed struct {
		Summary     string          `json:"summary"`
		Decisions   json.RawMessage `json:"decisions"`
		ActionItems json.RawMessage `json:"action_items"`
		Blockers    json.RawMessage `json:"blockers"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		writeJSON(w, 500, map[string]string{"error": "parse failed: " + err.Error()})
		return
	}

	var minutesID string
	gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO room_minutes (room_id, tenant_id, summary, decisions, action_items, blockers, msg_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		roomID, defaultTenant, parsed.Summary,
		parsed.Decisions, parsed.ActionItems, parsed.Blockers, len(msgs)).Scan(&minutesID)

	// Post summary as a room message
	summaryMsg := fmt.Sprintf("📝 **Meeting Summary** (%d messages)\n\n%s", len(msgs), parsed.Summary)
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO room_messages (room_id, sender_id, sender_type, content, message_type)
		 VALUES ($1, 'system', 'system', $2, 'summary')`, roomID, summaryMsg)

	writeJSON(w, 201, map[string]any{
		"id":      minutesID,
		"summary": parsed.Summary,
		"decisions":    parsed.Decisions,
		"action_items": parsed.ActionItems,
		"blockers":     parsed.Blockers,
		"msg_count":    len(msgs),
	})
}

// ─── Typing Indicators ────────────────────────────────────────────────────────

func (gw *Gateway) handleRoomTyping(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.WriteHeader(204)
		return
	}
	roomID := chi.URLParam(r, "id")
	var body struct {
		AgentID string `json:"agent_id"`
		Typing  bool   `json:"typing"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.AgentID == "" {
		w.WriteHeader(204)
		return
	}

	if body.Typing {
		gw.db.Pool.Exec(r.Context(),
			`INSERT INTO room_typing (room_id, agent_id) VALUES ($1, $2)
			 ON CONFLICT (room_id, agent_id) DO UPDATE SET started_at = NOW()`,
			roomID, body.AgentID)
	} else {
		gw.db.Pool.Exec(r.Context(),
			`DELETE FROM room_typing WHERE room_id = $1 AND agent_id = $2`,
			roomID, body.AgentID)
	}

	// Broadcast typing event via WebSocket
	if gw.rtHub != nil {
		eventType := "room_typing_start"
		if !body.Typing {
			eventType = "room_typing_stop"
		}
		gw.rtHub.Broadcast(realtime.Event{
			Type: eventType,
			Data: map[string]string{"room_id": roomID, "agent_id": body.AgentID},
		})
	}
	w.WriteHeader(204)
}

// ─── Room Tasks ───────────────────────────────────────────────────────────────

func (gw *Gateway) handleGetRoomTasks(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"tasks": []any{}})
		return
	}
	roomID := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, title, description, assigned_by, assigned_to, status, due_at, created_at
		 FROM room_tasks WHERE room_id = $1 ORDER BY created_at DESC`, roomID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"tasks": []any{}})
		return
	}
	defer rows.Close()
	tasks := []map[string]any{}
	for rows.Next() {
		var id, title, assignedBy, assignedTo, status string
		var description *string
		var dueAt, createdAt interface{}
		rows.Scan(&id, &title, &description, &assignedBy, &assignedTo, &status, &dueAt, &createdAt)
		t := map[string]any{
			"id": id, "title": title, "assigned_by": assignedBy,
			"assigned_to": assignedTo, "status": status,
			"due_at": dueAt, "created_at": createdAt,
		}
		if description != nil {
			t["description"] = *description
		}
		tasks = append(tasks, t)
	}
	if tasks == nil {
		tasks = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"tasks": tasks})
}

func (gw *Gateway) handleCreateRoomTask(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	roomID := chi.URLParam(r, "id")
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		AssignedBy  string `json:"assigned_by"`
		AssignedTo  string `json:"assigned_to"`
		DueAt       string `json:"due_at"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	if body.AssignedBy == "" {
		body.AssignedBy = "user"
	}
	if body.AssignedTo == "" {
		body.AssignedTo = "unassigned"
	}

	// Validate due_at: must be a parseable timestamp; discard freeform strings.
	var parsedDue *time.Time
	if body.DueAt != "" {
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(layout, body.DueAt); err == nil {
				parsedDue = &t
				break
			}
		}
	}

	var id string
	var q string
	var args []any
	if parsedDue != nil {
		q = `INSERT INTO room_tasks (room_id, tenant_id, title, description, assigned_by, assigned_to, due_at)
		      VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7) RETURNING id`
		args = []any{roomID, defaultTenant, body.Title, body.Description, body.AssignedBy, body.AssignedTo, *parsedDue}
	} else {
		q = `INSERT INTO room_tasks (room_id, tenant_id, title, description, assigned_by, assigned_to)
		      VALUES ($1, $2, $3, NULLIF($4,''), $5, $6) RETURNING id`
		args = []any{roomID, defaultTenant, body.Title, body.Description, body.AssignedBy, body.AssignedTo}
	}
	if err := gw.db.Pool.QueryRow(r.Context(), q, args...).Scan(&id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Also post as a task message in the room
	taskMsg := fmt.Sprintf("📋 TASK → @%s: %s", body.AssignedTo, body.Title)
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO room_messages (room_id, sender_id, sender_type, content, message_type)
		 VALUES ($1, $2, 'user', $3, 'task')`,
		roomID, body.AssignedBy, taskMsg)

	writeJSON(w, 201, map[string]any{"id": id, "status": "created"})
}

func (gw *Gateway) handleUpdateRoomTask(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	taskID := chi.URLParam(r, "task_id")
	var body struct {
		Status string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Status == "" {
		writeJSON(w, 400, map[string]string{"error": "status required"})
		return
	}
	gw.db.Pool.Exec(r.Context(),
		`UPDATE room_tasks SET status = $1, updated_at = NOW() WHERE id = $2`,
		body.Status, taskID)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// ─── Room Org Chart ───────────────────────────────────────────────────────────

func (gw *Gateway) handleGetRoomOrg(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"members": []any{}})
		return
	}
	roomID := chi.URLParam(r, "id")

	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT a.id, a.agent_key, a.display_name, a.role, a.avatar,
		        rm.role as room_role, rm.speaking_order, rm.can_decide, rm.last_active_at
		 FROM room_members rm
		 JOIN agents a ON rm.agent_id = a.id
		 WHERE rm.room_id = $1
		 ORDER BY rm.speaking_order ASC, a.display_name ASC`, roomID)
	if err != nil {
		writeJSON(w, 200, map[string]any{"members": []any{}})
		return
	}
	defer rows.Close()

	members := []map[string]any{}
	for rows.Next() {
		var id, agentKey, displayName, avatar, roomRole string
		var role, agentRole *string
		var speakingOrder int
		var canDecide bool
		var lastActiveAt interface{}
		rows.Scan(&id, &agentKey, &displayName, &agentRole, &avatar,
			&roomRole, &speakingOrder, &canDecide, &lastActiveAt)
		if role != nil {
			agentRoleStr := *role
			_ = agentRoleStr
		}

		agentRoleStr := ""
		if agentRole != nil {
			agentRoleStr = *agentRole
		}

		members = append(members, map[string]any{
			"id":             id,
			"agent_key":      agentKey,
			"display_name":   displayName,
			"agent_role":     agentRoleStr,
			"avatar":         avatar,
			"room_role":      roomRole,
			"speaking_order": speakingOrder,
			"can_decide":     canDecide,
			"last_active_at": lastActiveAt,
		})
	}
	if members == nil {
		members = []map[string]any{}
	}

	writeJSON(w, 200, map[string]any{
		"room_id": roomID,
		"members": members,
	})
}

// Unused import prevention
var _ = agent.ExtractHighlight
var _ = time.Second
