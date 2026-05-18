// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ─── Cartesia Sonic TTS ────────────────────────────────────────────────
//
// Native adapter for Cartesia's low-latency TTS. The WebSocket API
// gives ~90ms time-to-first-audio with Sonic-3 and supports voice
// cloning from a 5-second sample. For our one-shot Synthesize
// interface we hit the REST endpoint — later work will bolt on a
// SynthesizeStream method when the broader streaming refactor lands.
//
// API:
//   POST https://api.cartesia.ai/tts/bytes
//   Headers: X-API-Key, Cartesia-Version: 2024-06-10
//   Body: {"model_id","voice":{"mode":"id","id":"..."},"transcript","output_format":{...}}

const cartesiaAPIVersion = "2024-06-10"
const cartesiaTTSURL = "https://api.cartesia.ai/tts/bytes"

type CartesiaTTS struct {
	apiKey       string
	defaultVoice string
	defaultModel string
	client       *http.Client
}

// NewCartesiaTTS constructs a Cartesia TTS adapter. defaultVoice is a
// voice_id from the Cartesia dashboard; defaultModel is a model tag
// like "sonic-3". Both overridable per-call via TTSOptions.
func NewCartesiaTTS(apiKey, defaultVoice, defaultModel string) *CartesiaTTS {
	if defaultModel == "" { defaultModel = "sonic-3" }
	return &CartesiaTTS{
		apiKey:       apiKey,
		defaultVoice: defaultVoice,
		defaultModel: defaultModel,
		client:       &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *CartesiaTTS) Name() string { return "cartesia" }

func (p *CartesiaTTS) Synthesize(ctx context.Context, text string, opts TTSOptions) (*AudioResult, error) {
	voice := firstNonEmpty(opts.Voice, p.defaultVoice)
	model := firstNonEmpty(opts.Model, p.defaultModel)
	if voice == "" {
		return nil, fmt.Errorf("cartesia tts: voice id required (set in provider settings.voice_id)")
	}

	payload := map[string]any{
		"model_id":   model,
		"transcript": text,
		"voice": map[string]any{
			"mode": "id",
			"id":   voice,
		},
		"output_format": map[string]any{
			"container":   "mp3",
			"sample_rate": 44100,
			"encoding":    "mp3",
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", cartesiaTTSURL, bytes.NewReader(body))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Cartesia-Version", cartesiaAPIVersion)

	resp, err := p.client.Do(req)
	if err != nil { return nil, fmt.Errorf("cartesia tts: %w", err) }
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil { return nil, err }
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cartesia tts: HTTP %d — %s", resp.StatusCode, truncate(string(audio), 200))
	}
	return &AudioResult{
		Audio:     audio,
		Extension: "mp3",
		MimeType:  "audio/mpeg",
	}, nil
}

// ─── Cartesia voice discovery helper (optional) ───────────────────────
//
// Used by the Settings UI to populate a "Voice" dropdown. Not part of
// the TTSProvider interface — check-cast from the handler side.
type CartesiaVoice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
}

func (p *CartesiaTTS) ListVoices(ctx context.Context) ([]CartesiaVoice, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.cartesia.ai/voices", nil)
	if err != nil { return nil, err }
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("Cartesia-Version", cartesiaAPIVersion)

	resp, err := p.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cartesia voices: HTTP %d — %s", resp.StatusCode, truncate(string(body), 200))
	}
	out := []CartesiaVoice{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("cartesia voices: decode %w", err)
	}
	return out, nil
}

// debug-only: ensure unused imports don't break the build.
var _ = strings.TrimSpace
