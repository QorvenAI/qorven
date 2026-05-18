// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/qorvenai/qorven/internal/heartbeat"
	"github.com/qorvenai/qorven/internal/voice"
	"github.com/qorvenai/qorven/internal/workflow"
)

func (gw *Gateway) handleGetHeartbeat(w http.ResponseWriter, r *http.Request) {
	if gw.hbStore == nil {
		writeJSON(w, 503, map[string]string{"error": "heartbeat not configured"})
		return
	}
	cfg, err := gw.hbStore.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no heartbeat config"})
		return
	}
	writeJSON(w, 200, cfg)
}

func (gw *Gateway) handleUpsertHeartbeat(w http.ResponseWriter, r *http.Request) {
	if gw.hbStore == nil {
		writeJSON(w, 503, map[string]string{"error": "heartbeat not configured"})
		return
	}
	var cfg heartbeat.Config
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cfg)
	cfg.AgentID = chi.URLParam(r, "id")
	if err := gw.hbStore.Upsert(r.Context(), defaultTenant, &cfg); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "saved"})
}

func (gw *Gateway) handleTTS(w http.ResponseWriter, r *http.Request) {
	if gw.voiceMgr == nil || !gw.voiceMgr.HasTTS() {
		writeJSON(w, 503, map[string]string{"error": "no TTS provider configured"})
		return
	}
	var req struct {
		Input string `json:"input"`
		Voice string `json:"voice"`
		Model string `json:"model"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	if req.Input == "" {
		writeJSON(w, 400, map[string]string{"error": "input required"})
		return
	}

	result, err := gw.voiceMgr.Synthesize(r.Context(), req.Input, voice.TTSOptions{
		Voice: req.Voice, Model: req.Model,
	})
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", result.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=speech.%s", result.Extension))
	w.Write(result.Audio)
}

func (gw *Gateway) handleSTT(w http.ResponseWriter, r *http.Request) {
	if gw.voiceMgr == nil || !gw.voiceMgr.HasSTT() {
		writeJSON(w, 503, map[string]string{"error": "no STT provider configured"})
		return
	}
	// Accept multipart audio file
	r.ParseMultipartForm(10 << 20) // 10MB max
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "file required"})
		return
	}
	defer file.Close()

	audio, _ := io.ReadAll(file)
	format := "webm"
	if header != nil {
		if ct := header.Header.Get("Content-Type"); ct != "" {
			if strings.Contains(ct, "wav") {
				format = "wav"
			}
			if strings.Contains(ct, "mp3") || strings.Contains(ct, "mpeg") {
				format = "mp3"
			}
			if strings.Contains(ct, "ogg") {
				format = "ogg"
			}
		}
	}

	text, err := gw.voiceMgr.Transcribe(r.Context(), audio, format)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]any{"text": text})
}

func (gw *Gateway) handleVoiceProviders(w http.ResponseWriter, r *http.Request) {
	if gw.voiceMgr == nil {
		writeJSON(w, 200, map[string]any{"tts": []any{}, "stt": []any{}})
		return
	}
	writeJSON(w, 200, gw.voiceMgr.ListProviders())
}

func (gw *Gateway) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 200, map[string]any{"workflows": []any{}})
		return
	}
	wfs, err := gw.wfStore.List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if wfs == nil {
		wfs = []workflow.Workflow{}
	}
	writeJSON(w, 200, map[string]any{"workflows": wfs})
}

func (gw *Gateway) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var wf workflow.Workflow
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&wf)
	if wf.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name required"})
		return
	}
	id, err := gw.wfStore.Create(r.Context(), defaultTenant, wf)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	wf, err := gw.wfStore.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, wf)
}

func (gw *Gateway) handleUpdateWorkflow(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var wf workflow.Workflow
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&wf)
	if err := gw.wfStore.Update(r.Context(), chi.URLParam(r, "id"), wf); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (gw *Gateway) handleDeleteWorkflow(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	gw.wfStore.Delete(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleRunWorkflow(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil || gw.wfExecutor == nil {
		writeJSON(w, 503, map[string]string{"error": "workflows not configured"})
		return
	}
	wf, err := gw.wfStore.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "workflow not found"})
		return
	}
	var input map[string]any
	json.NewDecoder(r.Body).Decode(&input)
	if input == nil {
		input = map[string]any{}
	}
	// Run async
	go func() {
		gw.wfExecutor.Run(context.Background(), wf, input)
	}()
	writeJSON(w, 202, map[string]string{"status": "started"})
}

func (gw *Gateway) handleListWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 200, map[string]any{"runs": []any{}})
		return
	}
	runs, err := gw.wfStore.ListRuns(r.Context(), chi.URLParam(r, "id"), 20)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if runs == nil {
		runs = []workflow.Run{}
	}
	writeJSON(w, 200, map[string]any{"runs": runs})
}

func (gw *Gateway) handleListChannels(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"channels": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT ci.id, ci.agent_id, ci.channel_type, ci.name, ci.enabled, ci.status, ci.last_error,
		        COALESCE(a.display_name,'') as agent_name, COALESCE(a.agent_key,'') as agent_key
		 FROM channel_instances ci LEFT JOIN agents a ON ci.agent_id = a.id
		 WHERE ci.tenant_id = $1 ORDER BY ci.created_at DESC`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, chType, name, status, lastErr, agentName, agentKey string
		var agentID *string
		var enabled bool
		rows.Scan(&id, &agentID, &chType, &name, &enabled, &status, &lastErr, &agentName, &agentKey)
		entry := map[string]any{"id": id, "channel_type": chType, "name": name, "enabled": enabled, "status": status, "last_error": lastErr, "agent_name": agentName, "agent_key": agentKey}
		if agentID != nil {
			entry["agent_id"] = *agentID
		}
		list = append(list, entry)
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"channels": list})
}

func (gw *Gateway) handleCreateChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		AgentID     string         `json:"agent_id"`
		ChannelType string         `json:"channel_type"`
		Name        string         `json:"name"`
		Config      map[string]any `json:"config"`
		DMPolicy    string         `json:"dm_policy"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.ChannelType == "" {
		writeJSON(w, 400, map[string]string{"error": "channel_type required"})
		return
	}
	configJSON, _ := json.Marshal(body.Config)
	var agentID *string
	if body.AgentID != "" {
		agentID = &body.AgentID
	}
	dmPolicy := body.DMPolicy
	if dmPolicy == "" {
		dmPolicy = "open"
	}
	var id string
	err := gw.db.Pool.QueryRow(r.Context(), `INSERT INTO channel_instances (tenant_id, agent_id, channel_type, name, config, dm_policy) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`, defaultTenant, agentID, body.ChannelType, body.Name, configJSON, dmPolicy).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleUpdateChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Name    string         `json:"name"`
		Config  map[string]any `json:"config"`
		Enabled *bool          `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Config != nil {
		configJSON, _ := json.Marshal(body.Config)
		gw.db.Pool.Exec(r.Context(), `UPDATE channel_instances SET config = $1 WHERE id = $2 AND tenant_id = $3`,
			configJSON, id, defaultTenant)
	}
	if body.Name != "" {
		gw.db.Pool.Exec(r.Context(), `UPDATE channel_instances SET name = $1 WHERE id = $2 AND tenant_id = $3`,
			body.Name, id, defaultTenant)
	}
	if body.Enabled != nil {
		gw.db.Pool.Exec(r.Context(), `UPDATE channel_instances SET enabled = $1 WHERE id = $2 AND tenant_id = $3`,
			*body.Enabled, id, defaultTenant)
	}
	writeJSON(w, 200, map[string]string{"id": id, "status": "updated"})
}

func (gw *Gateway) handleDeleteChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	gw.db.Pool.Exec(r.Context(), `DELETE FROM channel_instances WHERE id = $1`, chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleStartChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	// Stop existing instance first to prevent duplicate polling
	gw.chanMgr.Stop(r.Context(), id)
	// Load and register if not in memory
	gw.loadSingleChannel(r.Context(), id)
	gw.chanMgr.Start(r.Context(), id)
	gw.db.Pool.Exec(r.Context(), `UPDATE channel_instances SET status = 'running' WHERE id = $1`, id)
	writeJSON(w, 200, map[string]string{"status": "started"})
}

func (gw *Gateway) handleStopChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	gw.chanMgr.Stop(r.Context(), id)
	gw.db.Pool.Exec(r.Context(), `UPDATE channel_instances SET status = 'disconnected' WHERE id = $1`, id)
	writeJSON(w, 200, map[string]string{"status": "stopped"})
}

func (gw *Gateway) handleGetChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var ch struct {
		ID          string  `json:"id"`
		AgentID     *string `json:"agent_id,omitempty"`
		ChannelType string  `json:"channel_type"`
		Name        string  `json:"name"`
		Enabled     bool    `json:"enabled"`
		Status      string  `json:"status"`
		LastError   string  `json:"last_error,omitempty"`
		AgentName   string  `json:"agent_name,omitempty"`
		AgentKey    string  `json:"agent_key,omitempty"`
	}
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT ci.id, ci.agent_id, ci.channel_type, ci.name, ci.enabled, ci.status, ci.last_error,
		        COALESCE(a.display_name,'') as agent_name, COALESCE(a.agent_key,'') as agent_key
		 FROM channel_instances ci LEFT JOIN agents a ON ci.agent_id = a.id
		 WHERE ci.id = $1 AND ci.tenant_id = $2`, id, defaultTenant).
		Scan(&ch.ID, &ch.AgentID, &ch.ChannelType, &ch.Name, &ch.Enabled, &ch.Status, &ch.LastError, &ch.AgentName, &ch.AgentKey)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "channel not found"})
		return
	}
	writeJSON(w, 200, ch)
}

// handleTestChannel verifies IMAP connectivity for an email channel.
// POST /v1/channels/{id}/test
func (gw *Gateway) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not configured"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	id := chi.URLParam(r, "id")

	var channelType string
	var configJSON []byte
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT channel_type, config FROM channel_instances WHERE id = $1 AND tenant_id = $2`,
		id, defaultTenant).Scan(&channelType, &configJSON)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
		return
	}
	if channelType != "email" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "test only supported for email channels"})
		return
	}

	var cfg map[string]any
	json.Unmarshal(configJSON, &cfg)

	imapHost, _ := cfg["imap_host"].(string)
	imapPortStr, _ := cfg["imap_port"].(string)
	email, _ := cfg["email"].(string)
	password, _ := cfg["password"].(string)

	if imapHost == "" || email == "" || password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "imap_host, email, and password are required"})
		return
	}
	imapPort := 993
	if p, err := strconv.Atoi(imapPortStr); err == nil && p > 0 {
		imapPort = p
	}

	addr := net.JoinHostPort(imapHost, strconv.Itoa(imapPort))
	dialCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Dial with context timeout.
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: imapHost}}
	conn, dialErr := dialer.DialContext(dialCtx, "tcp", addr)
	if dialErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("IMAP connect failed: %v", dialErr)})
		return
	}
	c, newErr := imapclient.New(conn)
	if newErr != nil {
		conn.Close()
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("IMAP handshake failed: %v", newErr)})
		return
	}
	defer c.Logout()

	if loginErr := c.Login(email, password); loginErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("IMAP login failed: %v", loginErr)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "IMAP connection successful"})
}

func (gw *Gateway) handleToggleWorkflow(w http.ResponseWriter, r *http.Request) {
	if gw.wfStore == nil {
		writeJSON(w, 503, map[string]string{"error": "workflows not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	wf, err := gw.wfStore.Get(r.Context(), id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "workflow not found"})
		return
	}
	wf.Enabled = !wf.Enabled
	if err := gw.wfStore.Update(r.Context(), id, *wf); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "enabled": wf.Enabled})
}

func (gw *Gateway) handleToggleCronJob(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "db not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var enabled bool
	err := gw.db.Pool.QueryRow(r.Context(), `SELECT enabled FROM cron_jobs WHERE id = $1`, id).Scan(&enabled)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "cron job not found"})
		return
	}
	newEnabled := !enabled
	if _, err := gw.db.Pool.Exec(r.Context(), `UPDATE cron_jobs SET enabled = $1 WHERE id = $2`, newEnabled, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "enabled": newEnabled})
}
