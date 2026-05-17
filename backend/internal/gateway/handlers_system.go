// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/knowledgegraph"
	"github.com/qorvenai/qorven/internal/research"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/tools"
)

var researchJobs = struct {
	sync.RWMutex
	jobs map[string]*researchJob
}{jobs: make(map[string]*researchJob)}

type researchJob struct {
	ID       string                   `json:"id"`
	Query    string                   `json:"query"`
	Mode     string                   `json:"mode"`
	Status   string                   `json:"status"`
	Report   *research.Report         `json:"report,omitempty"`
	Progress []research.ProgressEvent `json:"progress,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

type timelineMsg struct {
	SessionID  string `json:"session_id"`
	Channel    string `json:"channel"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	SenderName string `json:"sender_name,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

func (gw *Gateway) handleGetSessionMessages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if gw.sessions == nil {
		writeJSON(w, 200, map[string]any{"messages": []any{}, "total": 0})
		return
	}
	msgs, _ := gw.sessions.GetHistory(r.Context(), id)
	total := len(msgs)

	// ?limit=N&offset=M slices from the end (newest last).
	// Default: all messages. Clients send limit=50 for initial load,
	// then offset=50 to load older batches on scroll-up.
	q := r.URL.Query()
	limit, offset := 0, 0
	if v := q.Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if v := q.Get("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if limit > 0 {
		// offset counts from the end (most recent)
		end := total - offset
		if end < 0 {
			end = 0
		}
		start := end - limit
		if start < 0 {
			start = 0
		}
		msgs = msgs[start:end]
	}
	writeJSON(w, 200, map[string]any{"messages": msgs, "total": total})
}

func (gw *Gateway) handleAddSessionMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Content == "" {
		http.Error(w, `{"error":"content required"}`, 400)
		return
	}
	if gw.sessions == nil {
		http.Error(w, `{"error":"no session store"}`, http.StatusServiceUnavailable)
		return
	}
	gw.sessions.AppendMessage(r.Context(), id, session.Message{
		Role: req.Role, Content: req.Content, Timestamp: time.Now().Unix(),
	}, 0, 0)
	writeJSON(w, 200, map[string]string{"status": "saved"})
}

func (gw *Gateway) handleResearchStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		Mode  string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}
	if req.Query == "" {
		writeJSON(w, 400, map[string]string{"error": "query required"})
		return
	}
	if req.Mode == "" {
		req.Mode = "quick"
	}

	id := fmt.Sprintf("res-%d", time.Now().UnixNano())
	job := &researchJob{ID: id, Query: req.Query, Mode: req.Mode, Status: "running"}
	researchJobs.Lock()
	researchJobs.jobs[id] = job
	researchJobs.Unlock()

	go func() {
		report, err := gw.researchEngine.ResearchWithProgress(context.Background(), req.Query, research.Mode(req.Mode), func(ev research.ProgressEvent) {
			researchJobs.Lock()
			job.Progress = append(job.Progress, ev)
			researchJobs.Unlock()
		})
		researchJobs.Lock()
		if err != nil {
			job.Status = "failed"
			job.Error = err.Error()
		} else {
			job.Status = "completed"
			job.Report = report
		}
		researchJobs.Unlock()
	}()

	writeJSON(w, 202, map[string]string{"id": id, "status": "running"})
}

func (gw *Gateway) handleResearchGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	researchJobs.RLock()
	job, ok := researchJobs.jobs[id]
	researchJobs.RUnlock()
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, job)
}

func (gw *Gateway) handleOutboundPending(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"pending": []any{}})
		return
	}
	actions, err := tools.ListPending(r.Context(), gw.db.Pool)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if actions == nil {
		actions = []tools.OutboundAction{}
	}
	writeJSON(w, 200, map[string]any{"pending": actions})
}

func (gw *Gateway) handleOutboundApprove(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Notes string `json:"notes"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if err := tools.ApproveAction(r.Context(), gw.db.Pool, id, "user", body.Notes); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Execute the approved action
	var actionType string
	var payload json.RawMessage
	var agentID string
	gw.db.Pool.QueryRow(r.Context(),
		`SELECT action_type, payload, agent_id FROM outbound_queue WHERE id = $1`, id,
	).Scan(&actionType, &payload, &agentID)

	if actionType == "email_send" {
		var args map[string]any
		json.Unmarshal(payload, &args)
		result := gw.toolReg.Execute(tools.WithAgentID(r.Context(), agentID), "email_send", args)
		writeJSON(w, 200, map[string]any{"status": "approved_and_sent", "result": result.ForLLM})
		return
	}
	writeJSON(w, 200, map[string]any{"status": "approved"})
}

func (gw *Gateway) handleOutboundReject(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Notes string `json:"notes"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	tools.RejectAction(r.Context(), gw.db.Pool, id, "user", body.Notes)
	writeJSON(w, 200, map[string]any{"status": "rejected"})
}

func (gw *Gateway) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{})
		return
	}
	var prefs json.RawMessage
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT preferences FROM user_preferences WHERE tenant_id = $1 AND user_id = 'default'`, defaultTenant,
	).Scan(&prefs)
	if err != nil {
		writeJSON(w, 200, map[string]any{})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(prefs)
}

func (gw *Gateway) handleSavePreferences(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "no database"})
		return
	}
	var body json.RawMessage
	json.NewDecoder(r.Body).Decode(&body)
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO user_preferences (tenant_id, user_id, preferences, updated_at) VALUES ($1, 'default', $2, NOW())
		 ON CONFLICT (tenant_id, user_id) DO UPDATE SET preferences = $2, updated_at = NOW()`,
		defaultTenant, body)
	// Hot-reload PII redactor on save. Without this, toggling
	// "Redact PII" in Settings would only take effect on next restart.
	// Cheap: one DB read + bitmask build, never on the chat hot path.
	if gw.agentLoop != nil {
		if kinds, on := loadPIIKinds(r.Context(), gw.db.Pool, defaultTenant); on && kinds != 0 {
			gw.agentLoop.SetPIIRedactor(agent.NewPIIRedactor(kinds))
		} else {
			gw.agentLoop.SetPIIRedactor(nil)
		}
		// Same story for prompt-injection policy — flipping from
		// "warn" to "block" should take effect on the very next
		// message, not at the next gateway restart.
		gw.agentLoop.SetPromptGuardPolicy(loadPromptGuardPolicy(r.Context(), gw.db.Pool, defaultTenant))
	}
	// SQL connections can be added/removed via the same prefs blob.
	// Rebuild the registry so the LLM sees the new state on its next
	// sql_connections call. Existing connections whose DSN didn't
	// change stay up; changed/removed ones are closed.
	if gw.sqlRegistry != nil {
		// Cheapest approach: close-then-reload. Open is lazy so we
		// don't pay the DB ping cost until the agent uses them.
		gw.sqlRegistry.Close()
		loadSQLConnections(r.Context(), gw.db, defaultTenant, gw.cfg.Auth.EncryptionKey, gw.sqlRegistry)
	}
	writeJSON(w, 200, map[string]string{"status": "saved"})
}

func (gw *Gateway) handleRunSubconscious(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "id")
	if agentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}

	sub := agent.NewSubconscious(gw.agents, gw.memStore, gw.providerReg, defaultTenant)
	result, err := sub.RunLoop(r.Context(), agentID, agent.DefaultSubconsciousConfig())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, result)
}

func (gw *Gateway) handleMessageFeedback(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "no database"})
		return
	}
	var body struct {
		SessionID string `json:"session_id"`
		AgentID   string `json:"agent_id"`
		Content   string `json:"content"`
		Rating    string `json:"rating"` // "like" or "dislike"
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Rating != "like" && body.Rating != "dislike" {
		writeJSON(w, 400, map[string]string{"error": "rating must be 'like' or 'dislike'"})
		return
	}
	gw.db.Pool.Exec(r.Context(),
		`INSERT INTO message_feedback (session_id, agent_id, message_content, rating) VALUES ($1, $2, $3, $4)`,
		body.SessionID, body.AgentID, body.Content[:min(len(body.Content), 5000)], body.Rating)
	writeJSON(w, 200, map[string]string{"status": "saved"})
}

func (gw *Gateway) handleDeleteSessionMessage(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "no database"})
		return
	}
	sessionID := chi.URLParam(r, "id")
	var body struct {
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	// Remove from session messages array (JSONB)
	gw.db.Pool.Exec(r.Context(),
		`UPDATE sessions SET messages = (
			SELECT jsonb_agg(elem) FROM jsonb_array_elements(messages) elem
			WHERE elem->>'content' != $2
		) WHERE id = $1`, sessionID, body.Content)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func sanitizeError(err error) string {
	if err == nil {
		return "unknown error"
	}
	msg := err.Error()

	// Hide PostgreSQL connection details
	if strings.Contains(msg, "SQLSTATE") {
		if strings.Contains(msg, "23505") {
			return "A record with this identifier already exists"
		}
		if strings.Contains(msg, "23503") {
			return "Referenced record not found"
		}
		if strings.Contains(msg, "42P01") {
			return "Database table not configured — run migrations"
		}
		return "Database error — please try again"
	}

	// Hide connection errors
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return "Service temporarily unavailable"
	}

	// Hide file paths
	if strings.Contains(msg, "/home/") || strings.Contains(msg, "/tmp/") {
		return "Internal error — please try again"
	}

	// Hide API key errors (but keep the provider name)
	if strings.Contains(msg, "API key") || strings.Contains(msg, "api_key") {
		return "Provider authentication failed — check your API key configuration"
	}

	// Hide connection strings with credentials
	if strings.Contains(msg, "postgres://") || strings.Contains(msg, "mysql://") || strings.Contains(msg, "redis://") {
		return "Database connection error — check configuration"
	}

	// Hide bearer tokens and JWTs
	if strings.Contains(msg, "Bearer ") || strings.Contains(msg, "eyJhbG") {
		return "Authentication error — token may be expired"
	}

	// Hide passwords in URLs
	if strings.Contains(msg, "password") || strings.Contains(msg, "secret") {
		return "Internal error — please try again"
	}

	// Truncate long errors
	if len(msg) > 200 {
		return msg[:200] + "..."
	}

	return msg
}

func (gw *Gateway) handleDetailedHealth(w http.ResponseWriter, r *http.Request) {
	ver := buildInfo.Version
	if ver == "" {
		ver = "dev"
	}
	health := map[string]any{
		"status":     "ok",
		"version":    ver,
		"commit":     buildInfo.Commit,
		"build_time": buildInfo.BuildTime,
		"uptime":     time.Since(gw.startTime).String(),
	}

	// Database
	if gw.db != nil {
		var dbOK bool
		if err := gw.db.Pool.Ping(r.Context()); err == nil {
			dbOK = true
		}
		health["database"] = map[string]any{"connected": dbOK, "pool_size": gw.db.Pool.Stat().TotalConns()}
	} else {
		health["database"] = map[string]any{"connected": false}
	}

	// Providers
	providerCount := 0
	if gw.providerReg != nil {
		providerCount = len(gw.providerReg.List())
	}
	health["providers"] = providerCount

	// Tools
	health["tools"] = gw.toolReg.Count()

	// Channels
	if gw.chanMgr != nil {
		health["channels"] = len(gw.chanMgr.List())
	}

	// Memory
	health["memory"] = map[string]any{
		"hierarchy":  gw.agentLoop != nil && gw.agentLoop.HierarchyMem != nil,
		"embeddings": gw.memStore != nil,
	}

	// MCP
	if gw.mcpClient != nil {
		health["mcp_servers"] = len(gw.mcpClient.ListServers())
	}

	writeJSON(w, 200, health)
}

func (gw *Gateway) handleStartQoros(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gw.brain.StartQoros(r.Context(), id)
	writeJSON(w, 200, map[string]string{"status": "started", "agent_id": id})
}

func (gw *Gateway) handleStopQoros(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	gw.brain.StopQoros(id)
	writeJSON(w, 200, map[string]string{"status": "stopped", "agent_id": id})
}

func (gw *Gateway) handleQorosStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	k, exists := gw.brain.Qoros[id]
	if !exists {
		writeJSON(w, 200, map[string]any{"active": false, "agent_id": id})
		return
	}
	writeJSON(w, 200, map[string]any{
		"active":   k.IsActive(),
		"agent_id": id,
	})
}

func (gw *Gateway) handleGodNodes(w http.ResponseWriter, r *http.Request) {
	if gw.kgStore == nil {
		writeJSON(w, 200, []any{})
		return
	}
	gods, err := gw.kgStore.FindGodNodes(r.Context(), defaultTenant, 10)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, gods)
}

func (gw *Gateway) handleGraphClusters(w http.ResponseWriter, r *http.Request) {
	if gw.kgStore == nil {
		writeJSON(w, 200, map[string]int{})
		return
	}
	clusters, err := gw.kgStore.ClusterByType(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, clusters)
}

func (gw *Gateway) handleGraphAnalysis(w http.ResponseWriter, r *http.Request) {
	if gw.kgStore == nil {
		writeJSON(w, 200, map[string]any{"error": "knowledge graph not configured"})
		return
	}
	entities, _ := gw.kgStore.SearchEntities(r.Context(), defaultTenant, "", 1000)
	// Get relationships for all entities
	relationships := []knowledgegraph.Relationship{}
	for _, e := range entities {
		rels, _ := gw.kgStore.GetRelationships(r.Context(), defaultTenant, e.ID)
		relationships = append(relationships, rels...)
	}
	analysis := knowledgegraph.FullAnalysis(entities, relationships)
	writeJSON(w, 200, analysis)
}

func (gw *Gateway) handleUnifiedTimeline(w http.ResponseWriter, r *http.Request) {
	if gw.sessions == nil || gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "sessions not available"})
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if limit > 500 {
		limit = 500
	}

	// Get all sessions for this agent across all channels
	allSessions, err := gw.sessions.ListForAgent(r.Context(), agentID, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// Merge messages from all sessions into one unified timeline
	timeline := []timelineMsg{}
	for _, sess := range allSessions {
		msgs, err := gw.sessions.GetHistory(r.Context(), sess.ID)
		if err != nil {
			continue
		}
		for _, m := range msgs {
			channel := sess.Channel
			if m.Channel != "" {
				channel = m.Channel
			}
			timeline = append(timeline, timelineMsg{
				SessionID:  sess.ID,
				Channel:    channel,
				Role:       m.Role,
				Content:    m.Content,
				SenderName: m.SenderName,
				Timestamp:  m.Timestamp,
			})
		}
	}

	// Sort by timestamp ascending (oldest first, like WhatsApp)
	for i := 0; i < len(timeline)-1; i++ {
		for j := i + 1; j < len(timeline); j++ {
			if timeline[j].Timestamp < timeline[i].Timestamp {
				timeline[i], timeline[j] = timeline[j], timeline[i]
			}
		}
	}

	// Return last N messages (most recent)
	if len(timeline) > limit {
		timeline = timeline[len(timeline)-limit:]
	}

	writeJSON(w, 200, map[string]any{
		"agent_id": agentID,
		"messages": timeline,
		"total":    len(timeline),
		"channels": uniqueChannels(timeline),
	})
}

func uniqueChannels(msgs []timelineMsg) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range msgs {
		if !seen[m.Channel] {
			seen[m.Channel] = true
			out = append(out, m.Channel)
		}
	}
	return out
}

func (gw *Gateway) handleGetChangelog(w http.ResponseWriter, r *http.Request) {
	content := changelogContent
	if content == "" {
		// Try to read from disk relative to binary or working directory.
		for _, p := range []string{"CHANGELOG.md", "../CHANGELOG.md"} {
			if b, err := os.ReadFile(p); err == nil {
				content = string(b)
				break
			}
		}
	}
	writeJSON(w, 200, map[string]string{"changelog": content})
}

func verifyWebhookHMAC(r *http.Request, secret, sig string) bool {
	if sig == "" {
		return false
	}
	// Remove "sha256=" prefix if present
	sig = strings.TrimPrefix(sig, "sha256=")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body)) // restore for downstream

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}
