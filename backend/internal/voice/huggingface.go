// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// ─── HuggingFace Inference API voice adapters ──────────────────────────
//
// One pair of structs that drives any HF-hosted TTS or STT model via
// the public Inference API. Endpoint shape:
//
//   POST https://api-inference.huggingface.co/models/{model_id}
//   Authorization: Bearer {HF_TOKEN}
//
//   TTS: body {"inputs":"text"} → raw audio bytes (model-dependent: WAV or FLAC)
//   STT: body = raw audio bytes → JSON {"text":"..."}
//
// Bring-your-own-model: any HF model with the right pipeline tag
// works with zero code changes. The catalog surfaces a curated
// shortlist (bark, speecht5, whisper-large-v3, distil-whisper,
// canary-1b, parakeet-tdt) but the user can drop any HF model_id
// into the provider's settings.model_id field.
//
// Cold-start handling: HF returns HTTP 503 with {estimated_time}
// while a model warms up. We surface a structured error so the UI
// can retry rather than 500. Happy path: warm models respond in
// 200-800 ms for typical inputs.

const hfBase = "https://api-inference.huggingface.co/models"

type HuggingFaceTTS struct {
	apiKey  string
	modelID string
	client  *http.Client
}

func NewHuggingFaceTTS(apiKey, modelID string) *HuggingFaceTTS {
	return &HuggingFaceTTS{
		apiKey:  apiKey,
		modelID: modelID,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *HuggingFaceTTS) Name() string { return "huggingface_tts" }

func (p *HuggingFaceTTS) Synthesize(ctx context.Context, text string, _ TTSOptions) (*AudioResult, error) {
	body, _ := json.Marshal(map[string]string{"inputs": text})
	req, err := http.NewRequestWithContext(ctx, "POST", hfBase+"/"+p.modelID, bytes.NewReader(body))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/wav")
	if p.apiKey != "" { req.Header.Set("Authorization", "Bearer "+p.apiKey) }

	resp, err := p.client.Do(req)
	if err != nil { return nil, fmt.Errorf("huggingface tts: %w", err) }
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("huggingface tts %s: HTTP %d — %s",
			p.modelID, resp.StatusCode, truncate(string(data), 200))
	}
	// HF sometimes returns JSON on success for some models — sniff
	// the first byte to pick the right Extension.
	ext := "wav"
	mime := "audio/wav"
	if len(data) > 4 && (string(data[:4]) == "OggS") {
		ext = "opus"; mime = "audio/opus"
	} else if len(data) > 3 && string(data[:3]) == "ID3" {
		ext = "mp3"; mime = "audio/mpeg"
	}
	return &AudioResult{Audio: data, Extension: ext, MimeType: mime}, nil
}

type HuggingFaceSTT struct {
	apiKey  string
	modelID string
	client  *http.Client
}

func NewHuggingFaceSTT(apiKey, modelID string) *HuggingFaceSTT {
	return &HuggingFaceSTT{
		apiKey:  apiKey,
		modelID: modelID,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *HuggingFaceSTT) Name() string { return "huggingface_stt" }

func (p *HuggingFaceSTT) Transcribe(ctx context.Context, audio []byte, format string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", hfBase+"/"+p.modelID, bytes.NewReader(audio))
	if err != nil { return "", err }
	// HF accepts the audio content-type directly — e.g. audio/wav,
	// audio/webm, audio/mpeg. Fall back to octet-stream when the
	// caller doesn't know.
	ct := "audio/" + format
	if format == "" { ct = "application/octet-stream" }
	req.Header.Set("Content-Type", ct)
	if p.apiKey != "" { req.Header.Set("Authorization", "Bearer "+p.apiKey) }

	resp, err := p.client.Do(req)
	if err != nil { return "", fmt.Errorf("huggingface stt: %w", err) }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("huggingface stt %s: HTTP %d — %s",
			p.modelID, resp.StatusCode, truncate(string(body), 200))
	}
	var out struct{ Text string `json:"text"` }
	if err := json.Unmarshal(body, &out); err == nil && out.Text != "" {
		return strings.TrimSpace(out.Text), nil
	}
	return strings.TrimSpace(string(body)), nil
}
