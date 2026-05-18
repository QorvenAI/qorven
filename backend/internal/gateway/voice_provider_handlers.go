// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/voice"
)

// ─── Voice provider catalog + CRUD (DB-driven plug-and-play) ──────────
//
// Mirror of the LLM provider admin surface at /v1/providers but for
// voice. The Settings → Voice page and the setup wizard voice step
// both render forms straight off GET /v1/voice/catalog and drive
// provider rows through POST/PUT/DELETE /v1/voice/providers.

// handleVoiceCatalog returns the embedded driver catalog, optionally
// filtered by ?kind=tts|stt|realtime and ?hosting=cloud|local.
func (gw *Gateway) handleVoiceCatalog(w http.ResponseWriter, r *http.Request) {
	c, err := voice.LoadCatalog()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	entries := c.Filter(r.URL.Query().Get("kind"), r.URL.Query().Get("hosting"))
	writeJSON(w, 200, map[string]any{"drivers": entries, "count": len(entries)})
}

// handleVoiceProvidersList returns every configured provider row for
// the default tenant. API keys are stripped before serialising —
// the UI never sees a raw key.
func (gw *Gateway) handleVoiceProvidersList(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		// Fall back to the legacy manager view so the Settings page
		// still renders something useful on DB-less deployments.
		writeJSON(w, 200, map[string]any{
			"providers": []voice.ProviderRow{},
			"manager":   gw.voiceMgr.ListProviders(),
		})
		return
	}
	rows, err := gw.voiceStore.List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for i := range rows {
		rows[i].APIKey = "" // never expose the decrypted key
	}
	writeJSON(w, 200, map[string]any{
		"providers": rows,
		"manager":   gw.voiceMgr.ListProviders(),
	})
}

// handleVoiceProvidersCreate persists a new provider row, then builds
// the concrete adapter and registers it on the Manager so the user
// can use it immediately without a restart.
func (gw *Gateway) handleVoiceProvidersCreate(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		writeJSON(w, 503, map[string]string{"error": "voice store not configured"})
		return
	}
	var req struct {
		voice.ProviderRow
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	req.ProviderRow.APIKey = req.APIKey
	row := req.ProviderRow
	created, err := gw.voiceStore.Create(r.Context(), defaultTenant, row)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// Re-read so APIKey is decrypted and ready for BuildProvider.
	full, err := gw.voiceStore.Get(r.Context(), defaultTenant, created.ID)
	if err == nil {
		if tts, stt, berr := voice.BuildProvider(full); berr == nil {
			if tts != nil { gw.voiceMgr.RegisterTTS(tts) }
			if stt != nil { gw.voiceMgr.RegisterSTT(stt) }
			if full.IsDefault {
				switch full.Kind {
				case "tts": if tts != nil { gw.voiceMgr.SetPrimaryTTS(tts.Name()) }
				case "stt": if stt != nil { gw.voiceMgr.SetPrimarySTT(stt.Name()) }
				}
			}
		}
	}
	created.APIKey = ""
	writeJSON(w, 201, created)
}

// handleVoiceProvidersUpdate modifies a row. If the caller flips
// is_default, the store demotes other rows of the same kind.
func (gw *Gateway) handleVoiceProvidersUpdate(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		writeJSON(w, 503, map[string]string{"error": "voice store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		voice.ProviderRow
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	req.ProviderRow.APIKey = req.APIKey
	row := req.ProviderRow
	if err := gw.voiceStore.Update(r.Context(), defaultTenant, id, row); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	// Re-register so the Manager picks up the new config without a
	// restart. Registering the same Name() overwrites the previous
	// instance in the map.
	if full, err := gw.voiceStore.Get(r.Context(), defaultTenant, id); err == nil {
		if tts, stt, berr := voice.BuildProvider(full); berr == nil {
			if tts != nil { gw.voiceMgr.RegisterTTS(tts) }
			if stt != nil { gw.voiceMgr.RegisterSTT(stt) }
		}
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// handleVoiceProvidersDelete removes a row. The Manager keeps the
// stale adapter in-memory until a restart — for the common case of
// "user removed a broken provider" that's fine.
func (gw *Gateway) handleVoiceProvidersDelete(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		writeJSON(w, 503, map[string]string{"error": "voice store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.voiceStore.Delete(r.Context(), defaultTenant, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleVoiceProvidersSetDefault flips one row's is_default=true
// within its kind (tts/stt/realtime) and updates the Manager's
// primary TTS/STT accordingly.
func (gw *Gateway) handleVoiceProvidersSetDefault(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		writeJSON(w, 503, map[string]string{"error": "voice store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.voiceStore.SetDefault(r.Context(), defaultTenant, id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if full, err := gw.voiceStore.Get(r.Context(), defaultTenant, id); err == nil {
		if tts, stt, berr := voice.BuildProvider(full); berr == nil {
			switch full.Kind {
			case "tts": if tts != nil { gw.voiceMgr.SetPrimaryTTS(tts.Name()) }
			case "stt": if stt != nil { gw.voiceMgr.SetPrimarySTT(stt.Name()) }
			}
		}
	}
	writeJSON(w, 200, map[string]string{"status": "default_set"})
}

// handleVoiceProvidersTest runs a tiny round-trip against the named
// provider: TTS synthesises "Qorven voice test" and returns 200 if
// audio comes back; STT sends a 500ms silent WAV and returns 200 on
// any successful API response (empty transcript counts). Useful for
// the "Test" button in Settings without the user supplying an audio
// sample.
func (gw *Gateway) handleVoiceProvidersTest(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		writeJSON(w, 503, map[string]string{"error": "voice store not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	row, err := gw.voiceStore.Get(r.Context(), defaultTenant, id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "provider not found"})
		return
	}
	tts, stt, err := voice.BuildProvider(row)
	if err != nil {
		writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
		return
	}
	switch row.Kind {
	case "tts":
		if tts == nil {
			writeJSON(w, 200, map[string]any{"success": false, "error": "TTS driver not built"})
			return
		}
		result, err := tts.Synthesize(r.Context(), "Qorven voice test.", voice.TTSOptions{})
		if err != nil {
			writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{
			"success":   true,
			"bytes":     len(result.Audio),
			"mime":      result.MimeType,
			"extension": result.Extension,
		})
	case "stt":
		if stt == nil {
			writeJSON(w, 200, map[string]any{"success": false, "error": "STT driver not built"})
			return
		}
		text, err := stt.Transcribe(r.Context(), voiceSilentWav(), "wav")
		if err != nil {
			writeJSON(w, 200, map[string]any{"success": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"success": true, "transcript": text})
	case "realtime":
		// Realtime drivers don't have a one-shot test path; we only
		// verify the row exists and BuildProvider accepted it.
		writeJSON(w, 200, map[string]any{"success": true, "note": "realtime — connect via /ws/voice to test live"})
	}
}

// voiceSilentWav returns a 60-byte wav with a tiny bit of silence.
// Valid enough to make STT APIs answer "0 chars transcribed" instead
// of "invalid audio" — exactly what the test button wants.
func voiceSilentWav() []byte {
	return []byte{
		'R', 'I', 'F', 'F',
		0x2c, 0x00, 0x00, 0x00,
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		0x10, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
		0x40, 0x1f, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00,
		0x02, 0x00, 0x10, 0x00,
		'd', 'a', 't', 'a',
		0x10, 0x00, 0x00, 0x00,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
}
