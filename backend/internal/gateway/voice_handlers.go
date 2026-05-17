// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/qorvenai/qorven/internal/system"
	"github.com/qorvenai/qorven/internal/templates"
)

var installer = system.NewInstaller()

func (gw *Gateway) handleSystemSpecs(w http.ResponseWriter, r *http.Request) {
	specs := system.Detect()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(specs)
}

func (gw *Gateway) handleSystemInstall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Package string `json:"package"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Package == "" {
		http.Error(w, `{"error":"package required"}`, 400)
		return
	}

	// Check if local models are supported
	specs := system.Detect()
	if !specs.LocalOK && req.Package != "silero-vad" {
		http.Error(w, `{"error":"local models not supported on this architecture"}`, 400)
		return
	}

	if err := installer.Install(r.Context(), req.Package); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started", "package": req.Package})
}

func (gw *Gateway) handleInstallStatus(w http.ResponseWriter, r *http.Request) {
	pkg := r.URL.Query().Get("package")
	if pkg == "" {
		http.Error(w, `{"error":"package query param required"}`, 400)
		return
	}
	job := installer.Status(pkg)
	if job == nil {
		http.Error(w, `{"error":"no install job found"}`, 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// Voice config stored in gateway's config
type VoiceConfig struct {
	TTSProvider  string            `json:"tts_provider"`  // kokoro, openai, elevenlabs, edge
	STTProvider  string            `json:"stt_provider"`  // whisper-tiny/base/small/medium/large-v3, openai
	VAD          string            `json:"vad"`           // silero
	Kokoro       KokoroConfig      `json:"kokoro"`
	Whisper      WhisperConfig     `json:"whisper"`
	OpenAI       OpenAIVoiceConfig `json:"openai"`
	ElevenLabs   ElevenLabsConfig  `json:"elevenlabs"`
	Edge         EdgeConfig        `json:"edge"`
	AutoTTS      bool              `json:"auto_tts"`
	LiveTranscribe bool            `json:"live_transcribe"`
}

type KokoroConfig struct {
	URL   string `json:"url"`
	Voice string `json:"voice"`
}

type WhisperConfig struct {
	Model string `json:"model"` // tiny, base, small, medium, large-v3
	URL   string `json:"url"`
}

type OpenAIVoiceConfig struct {
	Voice string `json:"voice"` // alloy, nova, shimmer, etc.
}

type ElevenLabsConfig struct {
	APIKey  string `json:"api_key"`
	VoiceID string `json:"voice_id"`
}

type EdgeConfig struct {
	Voice string `json:"voice"`
}

var voiceConfig = VoiceConfig{
	TTSProvider: "kokoro",
	STTProvider: "whisper-base",
	VAD:         "silero",
	Kokoro:      KokoroConfig{URL: "http://localhost:8880", Voice: "af_heart"},
	Whisper:     WhisperConfig{Model: "base"},
	OpenAI:      OpenAIVoiceConfig{Voice: "alloy"},
	Edge:        EdgeConfig{Voice: "en-US-AriaNeural"},
	AutoTTS:     true,
	LiveTranscribe: true,
}

func (gw *Gateway) handleGetVoiceConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voiceConfig)
}

func (gw *Gateway) handlePutVoiceConfig(w http.ResponseWriter, r *http.Request) {
	if err := json.NewDecoder(r.Body).Decode(&voiceConfig); err != nil {
		http.Error(w, `{"error":"invalid json"}`, 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voiceConfig)
}

// --- Template / Marketplace Handlers ---

func (gw *Gateway) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(templates.Catalog())
}

func (gw *Gateway) handleInstallTemplate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TemplateID string `json:"template_id"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.TemplateID == "" {
		http.Error(w, `{"error":"template_id required"}`, 400)
		return
	}
	if gw.db == nil {
		http.Error(w, `{"error":"database not available"}`, http.StatusServiceUnavailable)
		return
	}
	inst := templates.NewInstaller(gw.db.Pool)
	result, err := inst.Install(r.Context(), "default", req.TemplateID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (gw *Gateway) handleListInstalled(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	inst := templates.NewInstaller(gw.db.Pool)
	list, _ := inst.ListInstalled(r.Context(), "default")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (gw *Gateway) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	templateID := r.PathValue("id")
	if gw.db == nil {
		http.Error(w, `{"error":"database not available"}`, http.StatusServiceUnavailable)
		return
	}
	inst := templates.NewInstaller(gw.db.Pool)
	dash, err := inst.GetDashboard(r.Context(), "default", templateID)
	if err != nil {
		http.Error(w, `{"error":"dashboard not found"}`, 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dash)
}

// handleSaveDashboard saves a custom dashboard config.
// Used by the workspace_builder tool and the visual dashboard editor.
func (gw *Gateway) handleSaveDashboard(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		http.Error(w, `{"error":"database not available"}`, http.StatusServiceUnavailable)
		return
	}
	var body struct {
		TemplateID string          `json:"template_id"`
		Name       string          `json:"name"`
		Config     json.RawMessage `json:"config"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.TemplateID == "" {
		http.Error(w, `{"error":"template_id required"}`, 400)
		return
	}
	if body.Name == "" {
		body.Name = body.TemplateID
	}

	_, err := gw.db.Pool.Exec(r.Context(),
		`INSERT INTO workspace_dashboards (tenant_id, template_id, name, config)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, template_id) DO UPDATE SET config = $4, updated_at = now()`,
		"default", body.TemplateID, body.Name, body.Config)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "saved", "template_id": body.TemplateID})
}

// handleGetDashboardByID returns a dashboard by its template ID.
// Alias for handleGetDashboard that works with the /dashboards/{id} route.
func (gw *Gateway) handleGetDashboardByID(w http.ResponseWriter, r *http.Request) {
	gw.handleGetDashboard(w, r)
}
